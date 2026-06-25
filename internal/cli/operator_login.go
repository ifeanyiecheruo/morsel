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
	var apiURL, username, password string
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

			if apiURL == "" {
				return fmt.Errorf("no API URL configured — provide --api-url to connect to a Morsel instance")
			}

			reader := bufio.NewReader(cmd.InOrStdin())
			if !cmd.Flags().Changed("username") {
				if _, err := fmt.Fprint(cmd.OutOrStdout(), "Username: "); err != nil {
					return fmt.Errorf("write prompt: %w", err)
				}
				line, err := reader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("read username: %w", err)
				}
				username = strings.TrimRight(line, "\r\n")
			}
			if username == "" {
				return fmt.Errorf("username is required")
			}

			if !cmd.Flags().Changed("password") {
				if _, err := fmt.Fprint(cmd.OutOrStdout(), "Password: "); err != nil {
					return fmt.Errorf("write prompt: %w", err)
				}
				line, err := reader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("read password: %w", err)
				}
				password = strings.TrimRight(line, "\r\n")
			}

			prof, err := c.handler.OperatorLogin(cmd.Context(), apiURL, username, password)
			if err != nil {
				return err
			}

			if err := c.handler.SaveProfile(cmd.Context(), c.profileName, prof); err != nil {
				return err
			}
			ctxlog.From(cmd.Context()).Info("authenticated", "api_url", apiURL)
			return nil
		},
	}
	cmd.Flags().StringVar(&apiURL, "api-url", "", "Morsel API URL override (default: from profile)")
	cmd.Flags().StringVar(&username, "username", "", "operator username (skips interactive prompt)")
	cmd.Flags().StringVar(&password, "password", "", "operator password (skips interactive prompt)")
	return cmd
}

func (h *cliHandler) OperatorLogin(ctx context.Context, apiURL, username, password string) (*Profile, error) {
	client, err := apiclient.New(apiURL, "")
	if err != nil {
		return nil, fmt.Errorf("build api client: %w", err)
	}
	resp, err := client.Inner().TokenOIDC(ctx, &oas.TokenOIDCReq{Username: username, Password: password})
	if err != nil {
		return nil, fmt.Errorf("authenticate: %w", err)
	}
	pair, ok := resp.(*oas.TokenPairResponse)
	if !ok {
		return nil, fmt.Errorf("login rejected: credentials not authorized")
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
