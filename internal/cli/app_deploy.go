package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func (c *cli) appDeployCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "deploy",
		Short: "Deploy all apps declared in .morsel/",
		RunE: func(cmd *cobra.Command, _ []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			return c.handler.AppDeploy(cmd.Context(), prof)
		},
	}
}

func (h *cliHandler) AppDeploy(_ context.Context, _ *Profile) error {
	return fmt.Errorf("not yet implemented")
}
