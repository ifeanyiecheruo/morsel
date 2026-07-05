package cli

import (
	"bufio"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel/internal/client"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel/internal/client/oas"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
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

			prof, passwordResetRequired, err := c.handler.OperatorLogin(cmd.Context(), apiURL, username, password)
			if err != nil {
				return err
			}

			if passwordResetRequired {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Your password must be changed before you can continue."); err != nil {
					return fmt.Errorf("write prompt: %w", err)
				}
				if _, err := fmt.Fprint(cmd.OutOrStdout(), "New password: "); err != nil {
					return fmt.Errorf("write prompt: %w", err)
				}
				newPassword, err := reader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("read new password: %w", err)
				}
				newPassword = strings.TrimRight(newPassword, "\r\n")
				if _, err := fmt.Fprint(cmd.OutOrStdout(), "Confirm new password: "); err != nil {
					return fmt.Errorf("write prompt: %w", err)
				}
				confirm, err := reader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("read confirmation: %w", err)
				}
				confirm = strings.TrimRight(confirm, "\r\n")
				if newPassword != confirm {
					return fmt.Errorf("passwords do not match")
				}
				if err := c.handler.OperatorChangePassword(cmd.Context(), prof, newPassword); err != nil {
					return err
				}
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

func (h *cliHandler) OperatorLogin(ctx context.Context, apiURL, username, password string) (*Profile, bool, error) {
	c, err := client.New(apiURL, "")
	if err != nil {
		return nil, false, fmt.Errorf("build api client: %w", err)
	}
	resp, err := c.Inner().TokenOIDC(ctx, &oas.TokenOIDCReq{Username: username, Password: password})
	if err != nil {
		return nil, false, fmt.Errorf("authenticate: %w", err)
	}
	pair, ok := resp.(*oas.TokenPairResponse)
	if !ok {
		return nil, false, fmt.Errorf("login rejected: credentials not authorized")
	}
	now := time.Now()
	prof := &Profile{
		APIURL:                apiURL,
		AccessToken:           pair.AccessToken,
		AccessTokenExpiresAt:  now.Add(time.Duration(pair.ExpiresIn) * time.Second).UTC().Format(time.RFC3339),
		RefreshToken:          pair.RefreshToken,
		RefreshTokenExpiresAt: now.Add(time.Duration(pair.RefreshExpiresIn) * time.Second).UTC().Format(time.RFC3339),
	}
	return prof, pair.PasswordResetRequired.Value, nil
}

func (h *cliHandler) OperatorChangePassword(ctx context.Context, prof *Profile, newPassword string) error {
	c, err := h.clientFor(prof)
	if err != nil {
		return err
	}
	res, err := c.Inner().ResetOperatorPassword(ctx, &oas.ChangePasswordReq{NewPassword: newPassword})
	if err != nil {
		return fmt.Errorf("change password: %w", err)
	}
	switch res.(type) {
	case *oas.ResetOperatorPasswordNoContent:
		return nil
	default:
		return fmt.Errorf("unexpected response: %T", res)
	}
}
