package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func (c *cli) serviceBootstrapCmd() *cobra.Command {
	var platformFlag string
	var kubeconfigFlag string

	cmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Install or upgrade the platform",
		RunE: func(_ *cobra.Command, _ []string) error {
			return c.handler.ServiceBootstrap(platformFlag, kubeconfigFlag)
		},
	}
	cmd.Flags().StringVar(&platformFlag, "platform", "", "platform implementation to use (gcp|local)")
	cmd.Flags().StringVar(&kubeconfigFlag, "kubeconfig", "", "path to kubeconfig (LocalPlatform only)")
	if err := cmd.MarkFlagRequired("platform"); err != nil {
		panic(err)
	}
	return cmd
}

func (h *cliHandler) ServiceBootstrap(_, _ string) error {
	return fmt.Errorf("not yet implemented")
}
