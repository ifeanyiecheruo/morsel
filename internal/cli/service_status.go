package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func (c *cli) serviceStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Report the health of all platform components",
		RunE: func(cmd *cobra.Command, _ []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			return c.handler.ServiceStatus(cmd.Context(), prof)
		},
	}
}

func (h *cliHandler) ServiceStatus(_ context.Context, _ *Profile) error {
	return fmt.Errorf("not yet implemented")
}
