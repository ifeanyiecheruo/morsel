package cli

import (
	"bufio"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ifeanyiecheruo/morsel/internal/api/oas"
	"github.com/ifeanyiecheruo/morsel/internal/apiclient"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
	"github.com/ifeanyiecheruo/morsel/internal/tokens"
	"github.com/spf13/cobra"
)

func (c *cli) operatorLoginCmd() *cobra.Command {
	var apiURL, credential string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate to the Morsel instance",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if apiURL == "" {
				if c.profile == nil {
					return fmt.Errorf("no profile found — provide --api-url to connect to a Morsel instance")
				}
				apiURL = c.profile.APIURL
			}

			if credential == "" {
				if _, err := fmt.Fprint(cmd.OutOrStdout(), "Credential: "); err != nil {
					return fmt.Errorf("write prompt: %w", err)
				}
				line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
				if err != nil {
					return fmt.Errorf("read credential: %w", err)
				}
				credential = strings.TrimRight(line, "\r\n")
			}
			if credential == "" {
				return fmt.Errorf("credential is required")
			}

			prof, err := c.handler.OperatorLogin(cmd.Context(), apiURL, credential)
			if err != nil {
				return err
			}

			// Preserve bootstrap fields from any existing profile.
			if c.profile != nil {
				prof.Platform = c.profile.Platform
				prof.Kubeconfig = c.profile.Kubeconfig
				prof.Kubecontext = c.profile.Kubecontext
				prof.ClusterServer = c.profile.ClusterServer
				prof.Project = c.profile.Project
				prof.Region = c.profile.Region
			}

			if err := writeProfile(c.profileName, prof); err != nil {
				return err
			}
			ctxlog.From(cmd.Context()).Info("authenticated", "api_url", apiURL)
			return nil
		},
	}
	cmd.Flags().StringVar(&apiURL, "api-url", "", "Morsel API URL override (default: from profile)")
	cmd.Flags().StringVar(&credential, "credential", "", "operator credential (skips interactive prompt)")
	return cmd
}

func (h *cliHandler) OperatorLogin(ctx context.Context, apiURL, credential string) (*Profile, error) {
	client, err := apiclient.New(apiURL, "")
	if err != nil {
		return nil, fmt.Errorf("build api client: %w", err)
	}
	resp, err := client.Inner().TokenOIDC(ctx, &oas.TokenOIDCReq{Credential: credential})
	if err != nil {
		return nil, fmt.Errorf("authenticate: %w", err)
	}
	pair, ok := resp.(*oas.TokenPairResponse)
	if !ok {
		return nil, fmt.Errorf("login rejected: credential not authorized")
	}
	now := time.Now()
	return &Profile{
		APIURL:                apiURL,
		AccessToken:           pair.AccessToken,
		AccessTokenExpiresAt:  now.Add(time.Duration(pair.ExpiresIn) * time.Second).UTC().Format(time.RFC3339),
		RefreshToken:          pair.RefreshToken,
		RefreshTokenExpiresAt: now.Add(tokens.OperatorRefreshTTL).UTC().Format(time.RFC3339),
	}, nil
}
