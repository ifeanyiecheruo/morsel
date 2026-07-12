package cli

import (
	"bytes"
	"context"
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

	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Install or upgrade the platform",
		RunE: func(cmd *cobra.Command, _ []string) error {
			loggedIn := c.profile != nil

			if loggedIn && !forceFlag {
				name, err := c.handler.ServiceDeployPlatform(cmd.Context(), c.profile)
				if err != nil {
					switch {
					case isConnectionError(err):
						return fmt.Errorf(
							"morsel instance at %s is unreachable — if the cluster was deleted, re-run with --force to recreate it",
							c.profile.APIURL,
						)
					case errors.Is(err, errStaleCredentials):
						if platformFlag == "" {
							return fmt.Errorf("%w\nspecify --platform to continue without a valid session", err)
						}
						fmt.Fprintf(os.Stderr, "warning: %v — proceeding with --platform=%s\n", err, platformFlag)
					default:
						return fmt.Errorf("get platform from instance: %w", err)
					}
				} else {
					if cmd.Flags().Changed("platform") && platformFlag != name {
						return fmt.Errorf("instance is running platform %q but --platform %q was specified", name, platformFlag)
					}
					platformFlag = name
				}
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
			if bootstrapToken != "" {
				clientID, clientIDErr := fetchMorselGitHubClientID(cmd.Context(), prof.APIURL)
				if clientIDErr != nil {
					ctxlog.From(cmd.Context()).Warn("fetch github client id", "err", clientIDErr)
				}
				var idToken string
				if clientID != "" {
					fmt.Println("Instance bootstrapped. Authenticate with GitHub to complete setup...")
					idProf, err := c.handler.OperatorLogin(cmd.Context(), prof.APIURL)
					if err != nil {
						return fmt.Errorf("authenticate: %w", err)
					}
					idToken = idProf.AccessToken
				} else {
					fmt.Println("Instance bootstrapped. No GitHub OAuth configured — creating local admin...")
				}
				adminProf, err := c.handler.CompleteBootstrap(cmd.Context(), prof.APIURL, bootstrapToken, idToken)
				if err != nil {
					return fmt.Errorf("complete bootstrap: %w", err)
				}
				prof.AccessToken = adminProf.AccessToken
				prof.AccessTokenExpiresAt = adminProf.AccessTokenExpiresAt
				prof.RefreshToken = adminProf.RefreshToken
				prof.RefreshTokenExpiresAt = adminProf.RefreshTokenExpiresAt
				if err := c.handler.SaveProfile(cmd.Context(), c.profileName, prof); err != nil {
					return err
				}
				fmt.Println("Authenticated as admin.")
			} else if c.profile != nil {
				// Re-deploy — the stored tokens may be stale. Attempt a silent
				// refresh so subsequent commands (e.g. morsel app deploy) work
				// without a manual login step.
				if _, err := silentRefresh(cmd.Context(), prof, c.profileName); err != nil {
					fmt.Fprintf(os.Stderr, "warning: could not refresh session — run 'morsel operator login' before deploying apps\n")
				}
			}

			return nil
		},
	}
	cmd.Flags().StringVar(&platformFlag, "platform", "", "platform implementation to use (local)")
	cmd.Flags().StringVar(&kubeconfigFlag, "kubeconfig", "", "path to kubeconfig file")
	cmd.Flags().BoolVarP(&yesFlag, "yes", "y", false, "accept defaults for all prompts and skip confirmation")
	cmd.Flags().BoolVar(&forceFlag, "force", false, "recreate cluster resources even when the instance is unreachable (requires --platform)")
	return cmd
}

// errStaleCredentials is returned by ServiceDeployPlatform when the stored
// token is rejected (401/403) by the running instance.
var errStaleCredentials = errors.New("session has expired — run 'morsel operator login' to re-authenticate")

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
	switch r := res.(type) {
	case *oas.DeploymentInfo:
		return r.Platform, nil
	case *oas.GetDeploymentInfoUnauthorized, *oas.GetDeploymentInfoForbidden:
		return "", errStaleCredentials
	default:
		return "", fmt.Errorf("get deployment info: unexpected response type %T", res)
	}
}

func (h *cliHandler) ServiceDeploy(ctx context.Context, kubeconfig string, b platform.ServiceDeployer, dockerfile []byte, yes bool) (*Profile, error) {
	ui := NewConsolePrompter(ctx, os.Stdin, os.Stdout)
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

func (h *cliHandler) BootstrapOperator(ctx context.Context, apiURL, bootstrapToken string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/bootstrap", bytes.NewReader([]byte("{}")))
	if err != nil {
		return fmt.Errorf("bootstrap request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Bootstrap-Token", bootstrapToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			ctxlog.From(ctx).Warn("close response body", "err", closeErr)
		}
	}()

	switch resp.StatusCode {
	case http.StatusNoContent:
		return nil
	case http.StatusUnauthorized:
		return fmt.Errorf("bootstrap token invalid or already used")
	default:
		return fmt.Errorf("bootstrap: unexpected status %d", resp.StatusCode)
	}
}
