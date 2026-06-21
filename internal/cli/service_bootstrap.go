package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/ifeanyiecheruo/morsel/internal/platforms"
	"github.com/ifeanyiecheruo/morsel/internal/secrets"
	"github.com/spf13/cobra"
)

func (c *cli) serviceBootstrapCmd() *cobra.Command {
	var platformFlag string
	var kubeconfigFlag string

	cmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Install or upgrade the platform",
		RunE: func(cmd *cobra.Command, _ []string) error {
			prof, err := c.handler.ServiceBootstrap(cmd.Context(), platformFlag, kubeconfigFlag)
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
	if err := cmd.MarkFlagRequired("platform"); err != nil {
		panic(err)
	}
	return cmd
}

func (h *cliHandler) ServiceBootstrap(ctx context.Context, platformName, kubeconfig string) (*Profile, error) {
	plat, err := platforms.Create(platformName)
	if err != nil {
		return nil, fmt.Errorf("unknown platform %q: %w", platformName, err)
	}

	b := plat.Bootstrap()

	// Phase 1 — verify platform prerequisites (cluster connectivity, credentials, etc.)
	fmt.Printf("  Checking prerequisites... ")
	if err := b.CheckPrerequisites(ctx, kubeconfig); err != nil {
		fmt.Println("✗")
		return nil, err
	}
	fmt.Printf("✓ %s\n", b.ClusterServer())

	// Check for a previously saved wizard config so re-runs skip the wizard.
	mgr := secrets.New(plat.Secrets())
	savedConfig, err := mgr.BootstrapConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load existing bootstrap config: %w", err)
	}

	var answers map[string]string
	if savedConfig != nil {
		fmt.Println("  Existing bootstrap configuration found — skipping wizard.")
		answers = savedConfig
	} else {
		p := NewConsolePrompter(os.Stdin, os.Stdout)
		answers, err = p.Ask(b.Prompts())
		if err != nil {
			return nil, err
		}
		p.PrintPlan(b.Plan(answers))
		if !p.Confirm("Proceed with provisioning? [y/N]: ") {
			return nil, fmt.Errorf("bootstrap cancelled")
		}
	}

	// Phase 2 — provision (writes signing keys + persists bootstrap config).
	fmt.Printf("  Provisioning... ")
	if err := b.Provision(ctx, answers); err != nil {
		fmt.Println("✗")
		return nil, err
	}
	fmt.Println("✓")

	prof := &Profile{
		Platform:      platformName,
		Kubeconfig:    b.KubeconfigPath(),
		Kubecontext:   b.KubeContext(),
		ClusterServer: b.ClusterServer(),
		APIURL:        "https://morsel-api.morsel.svc.cluster.local:8080",
	}

	fmt.Println("✓ Bootstrap complete. Run 'morsel operator login' to authenticate.")
	return prof, nil
}
