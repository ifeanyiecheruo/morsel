package cli

import "github.com/spf13/cobra"

func (c *cli) appCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "app",
		Short: "Developer commands",
	}
	cmd.AddCommand(c.appDeployCmd())
	return cmd
}
