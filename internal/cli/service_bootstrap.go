package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/ifeanyiecheruo/morsel/internal/platform"
	"github.com/ifeanyiecheruo/morsel/internal/platforms"
	"github.com/spf13/cobra"
)

func (c *cli) serviceBootstrapCmd() *cobra.Command {
	var platformFlag string
	var kubeconfigFlag string
	var yesFlag bool

	cmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Install or upgrade the platform",
		RunE: func(cmd *cobra.Command, _ []string) error {
			plat, err := platforms.Create(platformFlag, nil)
			if err != nil {
				return fmt.Errorf("unknown platform %q: %w", platformFlag, err)
			}

			prof, err := c.handler.ServiceBootstrap(cmd.Context(), platformFlag, kubeconfigFlag, plat, c.dockerfile, yesFlag)
			if err != nil {
				return err
			}
			// Preserve existing token fields so a re-bootstrap does not log the operator out.
			if c.profile != nil {
				prof.AccessToken = c.profile.AccessToken
				prof.AccessTokenExpiresAt = c.profile.AccessTokenExpiresAt
				prof.RefreshToken = c.profile.RefreshToken
				prof.RefreshTokenExpiresAt = c.profile.RefreshTokenExpiresAt
			}
			return c.handler.SaveProfile(cmd.Context(), c.profileName, prof)
		},
	}
	cmd.Flags().StringVar(&platformFlag, "platform", "", "platform implementation to use (gcp|local)")
	cmd.Flags().StringVar(&kubeconfigFlag, "kubeconfig", "", "path to kubeconfig file")
	cmd.Flags().BoolVarP(&yesFlag, "yes", "y", false, "accept defaults for all prompts and skip confirmation")
	if err := cmd.MarkFlagRequired("platform"); err != nil {
		panic(err)
	}
	return cmd
}

func (h *cliHandler) ServiceBootstrap(ctx context.Context, platformName, kubeconfig string, plat platform.Platform, dockerfile []byte, yes bool) (*Profile, error) {
	b := plat.Bootstrap()

	ui := NewConsolePrompter(os.Stdin, os.Stdout)
	ui.autoAcceptDefault = yes

	answers, err := ui.Ask(b.Prompts())
	if err != nil {
		return nil, err
	}

	// CheckPrerequisites runs after prompts so it knows the provider and can
	// create a kind cluster (with the correct extraPortMappings) when needed.
	fmt.Printf("  Checking prerequisites... ")
	if err := b.CheckPrerequisites(ctx, kubeconfig, answers); err != nil {
		fmt.Println("✗")
		return nil, err
	}
	fmt.Printf("✓ %s\n", b.ClusterServer())

	ui.PrintPlan(b.Plan(answers))
	if !ui.Confirm("Proceed with provisioning? [y/N]: ") {
		return nil, fmt.Errorf("bootstrap cancelled")
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

	fmt.Println("✓ Bootstrap complete. Run 'morsel operator login' to authenticate.")
	return prof, nil
}
