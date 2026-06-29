package cli

import "github.com/spf13/cobra"

func (c *cli) serviceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Platform lifecycle commands (operator use)",
	}
	cmd.AddCommand(c.serviceDeployCmd())
	cmd.AddCommand(c.serviceStatusCmd())
	cmd.AddCommand(c.serviceDeleteCmd())
	cmd.AddCommand(c.serviceUpgradeCmd())
	return cmd
}
