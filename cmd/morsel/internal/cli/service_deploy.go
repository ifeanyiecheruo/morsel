package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel/internal/client/oas"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel/internal/platform"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel/internal/platforms"
	"github.com/spf13/cobra"
)

func (c *cli) serviceDeployCmd() *cobra.Command {
	var platformFlag string
	var kubeconfigFlag string
	var yesFlag bool

	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Install or upgrade the platform",
		RunE: func(cmd *cobra.Command, _ []string) error {
			loggedIn := c.profile != nil

			if loggedIn {
				name, err := c.handler.ServiceDeployPlatform(cmd.Context(), c.profile)
				if err != nil {
					return fmt.Errorf("get platform from instance: %w", err)
				}
				if cmd.Flags().Changed("platform") && platformFlag != name {
					return fmt.Errorf("instance is running platform %q but --platform %q was specified", name, platformFlag)
				}
				platformFlag = name
			} else {
				if platformFlag == "" {
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
			return c.handler.SaveProfile(cmd.Context(), c.profileName, prof)
		},
	}
	cmd.Flags().StringVar(&platformFlag, "platform", "", "platform implementation to use (local)")
	cmd.Flags().StringVar(&kubeconfigFlag, "kubeconfig", "", "path to kubeconfig file")
	cmd.Flags().BoolVarP(&yesFlag, "yes", "y", false, "accept defaults for all prompts and skip confirmation")
	return cmd
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

	prof := &Profile{
		APIURL: b.APIURL(),
	}

	fmt.Println("✓ Deploy complete. Run 'morsel operator login' to authenticate.")
	return prof, nil
}
