package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func (c *cli) serviceUpgradeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Platform upgrade commands",
	}
	cmd.AddCommand(c.serviceUpgradeRetryCmd())
	return cmd
}

func (c *cli) serviceUpgradeRetryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "retry",
		Short: "Retry app redeployments that failed during the most recent platform upgrade",
		RunE: func(cmd *cobra.Command, _ []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			return c.handler.ServiceUpgradeRetry(cmd.Context(), prof)
		},
	}
}

func (h *cliHandler) ServiceUpgradeRetry(_ context.Context, _ *Profile) error {
	return fmt.Errorf("not yet implemented")
}
