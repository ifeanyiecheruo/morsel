package cli

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel/internal/client/oas"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel/internal/platform"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel/internal/platforms"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
	"github.com/spf13/cobra"
)

func (c *cli) serviceDeployCmd() *cobra.Command {
	var platformFlag string
	var kubeconfigFlag string
	var yesFlag bool
	var forceFlag bool
	var initialUsernameFlag string
	var outInitialPasswdFlag string
	var noLoginFlag bool

	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Install or upgrade the platform",
		RunE: func(cmd *cobra.Command, _ []string) error {
			loggedIn := c.profile != nil

			if loggedIn && !forceFlag {
				name, err := c.handler.ServiceDeployPlatform(cmd.Context(), c.profile)
				if err != nil {
					if isConnectionError(err) {
						return fmt.Errorf(
							"morsel instance at %s is unreachable — if the cluster was deleted, re-run with --force to recreate it",
							c.profile.APIURL,
						)
					}
					return fmt.Errorf("get platform from instance: %w", err)
				}
				if cmd.Flags().Changed("platform") && platformFlag != name {
					return fmt.Errorf("instance is running platform %q but --platform %q was specified", name, platformFlag)
				}
				platformFlag = name
			} else {
				// Not logged in, or --force was given — platform must be supplied explicitly.
				if platformFlag == "" {
					if forceFlag {
						return fmt.Errorf("--platform is required when using --force")
					}
					return fmt.Errorf("--platform is required when not logged in")
				}
			}

			b, err := platforms.New(platformFlag)
			if err != nil {
				return fmt.Errorf("unknown platform %q: %w", platformFlag, err)
			}

			prof, err := c.handler.ServiceDeploy(cmd.Context(), kubeconfigFlag, b, c.dockerfile, yesFlag)
			if err != nil {
				return err
			}
			// Preserve existing token fields so a re-deploy does not log the operator out.
			if c.profile != nil {
				prof.AccessToken = c.profile.AccessToken
				prof.AccessTokenExpiresAt = c.profile.AccessTokenExpiresAt
				prof.RefreshToken = c.profile.RefreshToken
				prof.RefreshTokenExpiresAt = c.profile.RefreshTokenExpiresAt
			}
			if err := c.handler.SaveProfile(cmd.Context(), c.profileName, prof); err != nil {
				return err
			}

			bootstrapToken := ""
			if bt, ok := b.(interface{ BootstrapToken() string }); ok {
				bootstrapToken = bt.BootstrapToken()
			}
			passwd, err := c.handler.BootstrapOperator(cmd.Context(), prof.APIURL, initialUsernameFlag, bootstrapToken)
			if err != nil {
				return err
			}

			if passwd != "" {
				// First deploy — output the password.
				if outInitialPasswdFlag != "" {
					if err := os.WriteFile(outInitialPasswdFlag, []byte(passwd+"\n"), 0600); err != nil {
						return fmt.Errorf("write initial password to %s: %w", outInitialPasswdFlag, err)
					}
					fmt.Printf("Initial operator %q password written to: %s\n", initialUsernameFlag, outInitialPasswdFlag)
				} else {
					fmt.Printf("Initial operator %q password: %s\n", initialUsernameFlag, passwd)
				}

				if !noLoginFlag {
					loginProf, _, err := c.handler.OperatorLogin(cmd.Context(), prof.APIURL, initialUsernameFlag, passwd)
					if err != nil {
						return fmt.Errorf("auto-login: %w", err)
					}
					prof.AccessToken = loginProf.AccessToken
					prof.AccessTokenExpiresAt = loginProf.AccessTokenExpiresAt
					prof.RefreshToken = loginProf.RefreshToken
					prof.RefreshTokenExpiresAt = loginProf.RefreshTokenExpiresAt
					if err := c.handler.SaveProfile(cmd.Context(), c.profileName, prof); err != nil {
						return err
					}
					fmt.Printf("Logged in as %q.\n", initialUsernameFlag)
				} else {
					fmt.Println("Run 'morsel operator login' to authenticate.")
				}
			}

			return nil
		},
	}
	cmd.Flags().StringVar(&platformFlag, "platform", "", "platform implementation to use (local)")
	cmd.Flags().StringVar(&kubeconfigFlag, "kubeconfig", "", "path to kubeconfig file")
	cmd.Flags().BoolVarP(&yesFlag, "yes", "y", false, "accept defaults for all prompts and skip confirmation")
	cmd.Flags().BoolVar(&forceFlag, "force", false, "recreate cluster resources even when the instance is unreachable (requires --platform)")
	cmd.Flags().StringVar(&initialUsernameFlag, "initial-username", "admin", "username for the initial operator (first deploy only)")
	cmd.Flags().StringVar(&outInitialPasswdFlag, "out-initial-passwd", "", "write the initial operator password to this file instead of printing it")
	cmd.Flags().BoolVar(&noLoginFlag, "no-login", false, "skip automatic login after first-time bootstrap")
	return cmd
}

// isConnectionError reports whether err is a network connectivity failure
// (connection refused, host unreachable, DNS failure, etc.).
func isConnectionError(err error) bool {
	var netErr *net.OpError
	return errors.As(err, &netErr)
}

func (h *cliHandler) ServiceDeployPlatform(ctx context.Context, prof *Profile) (string, error) {
	c, err := h.clientFor(prof)
	if err != nil {
		return "", err
	}
	res, err := c.Inner().GetDeploymentInfo(ctx)
	if err != nil {
		return "", fmt.Errorf("get deployment info: %w", err)
	}
	info, ok := res.(*oas.DeploymentInfo)
	if !ok {
		return "", fmt.Errorf("get deployment info: unexpected response type")
	}
	return info.Platform, nil
}

func (h *cliHandler) ServiceDeploy(ctx context.Context, kubeconfig string, b platform.ServiceDeployer, dockerfile []byte, yes bool) (*Profile, error) {
	ui := NewConsolePrompter(os.Stdin, os.Stdout)
	ui.autoAcceptDefault = yes

	answers, err := ui.Ask(b.Prompts())
	if err != nil {
		return nil, err
	}

	// CheckPrerequisites runs after prompts so it knows the provider and can
	// create a k3d cluster (with the correct extraPortMappings) when needed.
	fmt.Printf("  Checking prerequisites... ")
	if err := b.CheckPrerequisites(ctx, kubeconfig, answers); err != nil {
		fmt.Println("✗")
		return nil, err
	}
	fmt.Printf("✓ %s\n", b.ClusterServer())

	ui.PrintPlan(b.Plan(answers))
	if !ui.Confirm("Proceed with provisioning? [y/N]: ") {
		return nil, fmt.Errorf("deploy cancelled")
	}

	fmt.Printf("  Provisioning... ")
	if err := b.Provision(ctx, answers, dockerfile); err != nil {
		fmt.Println("✗")
		return nil, err
	}
	fmt.Println("✓")

	return &Profile{APIURL: b.APIURL()}, nil
}

func (h *cliHandler) BootstrapOperator(ctx context.Context, apiURL, username, bootstrapToken string) (string, error) {
	return bootstrapOperator(ctx, apiURL, username, bootstrapToken)
}

// bootstrapOperator calls POST /bootstrap on a freshly provisioned instance.
// Returns the generated password on first deploy (201 Created) so the caller
// can log in. Returns "" on subsequent deploys (409 Conflict, no-op).
func bootstrapOperator(ctx context.Context, apiURL, username, bootstrapToken string) (string, error) {
	passwd, err := generatePassword()
	if err != nil {
		return "", fmt.Errorf("generate initial password: %w", err)
	}

	body, err := json.Marshal(map[string]string{"username": username, "password": passwd})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/bootstrap", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("bootstrap request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if bootstrapToken != "" {
		req.Header.Set("X-Bootstrap-Token", bootstrapToken)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("bootstrap: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			ctxlog.From(ctx).Warn("close response body", "err", closeErr)
		}
	}()

	switch resp.StatusCode {
	case http.StatusCreated:
		return passwd, nil
	case http.StatusConflict:
		return "", nil
	default:
		return "", fmt.Errorf("bootstrap: unexpected status %d", resp.StatusCode)
	}
}

func generatePassword() (string, error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
