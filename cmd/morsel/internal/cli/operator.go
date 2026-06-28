package cli

import "github.com/spf13/cobra"

func (c *cli) operatorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "operator",
		Short: "Operator management commands",
	}
	cmd.AddCommand(c.operatorLoginCmd())
	cmd.AddCommand(c.operatorLogoutCmd())
	cmd.AddCommand(c.operatorPrincipalCmd())
	cmd.AddCommand(c.operatorTierCmd())
	cmd.AddCommand(c.operatorAppExemptParentCmd())
	cmd.AddCommand(c.operatorRepoExemptParentCmd())
	return cmd
}
