package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
	"github.com/spf13/cobra"
)

func (c *cli) operatorLoginCmd() *cobra.Command {
	var apiURL string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate to the Morsel instance via GitHub Device Flow",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if apiURL == "" {
				if c.profile != nil {
					apiURL = c.profile.APIURL
				}
			}
			if apiURL == "" {
				return fmt.Errorf("no API URL configured — provide --api-url to connect to a Morsel instance")
			}
			prof, err := c.handler.OperatorLogin(cmd.Context(), apiURL)
			if err != nil {
				return err
			}
			if err := c.handler.SaveProfile(cmd.Context(), c.profileName, prof); err != nil {
				return err
			}
			ctxlog.From(cmd.Context()).Info("authenticated", "api_url", apiURL)
			fmt.Println("Authenticated successfully.")
			return nil
		},
	}
	cmd.Flags().StringVar(&apiURL, "api-url", "", "Morsel API URL (default: from profile)")
	return cmd
}

// ── Device Flow implementation ────────────────────────────────────────────────

type githubConfigResp struct {
	ClientID string `json:"client_id"`
}

type deviceCodeResp struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type deviceTokenResp struct {
	AccessToken string `json:"access_token"`
	Error       string `json:"error"`
	Interval    int    `json:"interval"`
}

func fetchMorselGitHubClientID(ctx context.Context, apiURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL+"/github/config", nil)
	if err != nil {
		return "", fmt.Errorf("build config request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch github config: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			ctxlog.From(ctx).Warn("close github config response body", "err", closeErr)
		}
	}()
	var cfg githubConfigResp
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return "", fmt.Errorf("decode github config: %w", err)
	}
	return cfg.ClientID, nil
}

func startGitHubDeviceFlow(ctx context.Context, clientID string) (deviceCodeResp, error) {
	body := url.Values{"client_id": {clientID}, "scope": {"read:user"}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://github.com/login/device/code", strings.NewReader(body))
	if err != nil {
		return deviceCodeResp{}, fmt.Errorf("build device code request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return deviceCodeResp{}, fmt.Errorf("start device flow: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			ctxlog.From(ctx).Warn("close device code response body", "err", closeErr)
		}
	}()

	var dr deviceCodeResp
	if err := json.NewDecoder(resp.Body).Decode(&dr); err != nil {
		return deviceCodeResp{}, fmt.Errorf("decode device code response: %w", err)
	}
	if dr.DeviceCode == "" {
		return deviceCodeResp{}, fmt.Errorf("github did not return a device code")
	}
	if dr.Interval == 0 {
		dr.Interval = 5
	}
	return dr, nil
}

func pollGitHubDeviceFlow(ctx context.Context, clientID string, dr deviceCodeResp) (string, error) {
	body := url.Values{
		"client_id":   {clientID},
		"device_code": {dr.DeviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}
	interval := time.Duration(dr.Interval) * time.Second
	deadline := time.Now().Add(time.Duration(dr.ExpiresIn) * time.Second)

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(interval):
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("device flow timed out — run 'morsel operator login' to try again")
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			"https://github.com/login/oauth/access_token", strings.NewReader(body.Encode()))
		if err != nil {
			return "", fmt.Errorf("build token poll request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("poll device flow: %w", err)
		}
		raw, readErr := io.ReadAll(resp.Body)
		if closeErr := resp.Body.Close(); closeErr != nil {
			ctxlog.From(ctx).Warn("close device poll response body", "err", closeErr)
		}
		if readErr != nil {
			return "", fmt.Errorf("read device poll response: %w", readErr)
		}

		var tr deviceTokenResp
		if err := json.Unmarshal(raw, &tr); err != nil {
			return "", fmt.Errorf("decode token response: %w", err)
		}

		switch tr.Error {
		case "":
			if tr.AccessToken != "" {
				return tr.AccessToken, nil
			}
		case "authorization_pending":
			// keep polling
		case "slow_down":
			if tr.Interval > 0 {
				interval = time.Duration(tr.Interval) * time.Second
			} else {
				interval += 5 * time.Second
			}
		case "expired_token":
			return "", fmt.Errorf("device flow code expired — run 'morsel operator login' to try again")
		case "access_denied":
			return "", fmt.Errorf("authorization denied by user")
		default:
			return "", fmt.Errorf("device flow error: %s", tr.Error)
		}
	}
}

type morselAuthResp struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
}

func exchangeGitHubTokenWithMorsel(ctx context.Context, apiURL, ghToken string) (*Profile, error) {
	body, err := json.Marshal(struct {
		Token string `json:"token"`
	}{Token: ghToken})
	if err != nil {
		return nil, fmt.Errorf("marshal auth request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/token/oidc", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build morsel auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("morsel github auth: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			ctxlog.From(ctx).Warn("close morsel auth response body", "err", closeErr)
		}
	}()

	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("this GitHub account has viewer access only — ask a platform admin to grant operator access")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("morsel auth returned %d — check that the server is running and GitHub OAuth is configured", resp.StatusCode)
	}

	var pair morselAuthResp
	if err := json.NewDecoder(resp.Body).Decode(&pair); err != nil {
		return nil, fmt.Errorf("decode morsel auth response: %w", err)
	}

	now := time.Now()
	return &Profile{
		APIURL:                apiURL,
		AccessToken:           pair.AccessToken,
		AccessTokenExpiresAt:  now.Add(time.Duration(pair.ExpiresIn) * time.Second).UTC().Format(time.RFC3339),
		RefreshToken:          pair.RefreshToken,
		RefreshTokenExpiresAt: now.Add(time.Duration(pair.RefreshExpiresIn) * time.Second).UTC().Format(time.RFC3339),
	}, nil
}

func (h *cliHandler) CompleteBootstrap(ctx context.Context, apiURL, bootstrapToken, idToken string) (*Profile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/bootstrap", nil)
	if err != nil {
		return nil, fmt.Errorf("build bootstrap request: %w", err)
	}
	if idToken != "" {
		req.Header.Set("Authorization", "Bearer "+idToken)
	}
	req.Header.Set("X-Bootstrap-Token", bootstrapToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("complete bootstrap: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			ctxlog.From(ctx).Warn("close bootstrap response body", "err", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bootstrap returned %d — check the bootstrap token and try again", resp.StatusCode)
	}

	var pair morselAuthResp
	if err := json.NewDecoder(resp.Body).Decode(&pair); err != nil {
		return nil, fmt.Errorf("decode bootstrap response: %w", err)
	}

	now := time.Now()
	return &Profile{
		APIURL:                apiURL,
		AccessToken:           pair.AccessToken,
		AccessTokenExpiresAt:  now.Add(time.Duration(pair.ExpiresIn) * time.Second).UTC().Format(time.RFC3339),
		RefreshToken:          pair.RefreshToken,
		RefreshTokenExpiresAt: now.Add(time.Duration(pair.RefreshExpiresIn) * time.Second).UTC().Format(time.RFC3339),
	}, nil
}

func (h *cliHandler) OperatorLogin(ctx context.Context, apiURL string) (*Profile, error) {
	clientID, err := fetchMorselGitHubClientID(ctx, apiURL)
	if err != nil {
		return nil, err
	}
	if clientID == "" {
		return nil, fmt.Errorf("GitHub OAuth is not configured on this Morsel instance")
	}

	dr, err := startGitHubDeviceFlow(ctx, clientID)
	if err != nil {
		return nil, err
	}

	fmt.Printf("\nVisit %s\nand enter code: %s\n\nWaiting for authorization...\n", dr.VerificationURI, dr.UserCode)

	ghToken, err := pollGitHubDeviceFlow(ctx, clientID, dr)
	if err != nil {
		return nil, err
	}

	return exchangeGitHubTokenWithMorsel(ctx, apiURL, ghToken)
}
