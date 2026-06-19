package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func (c *cli) operatorPrincipalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "principal",
		Short: "Manage admin UI principals",
	}
	cmd.AddCommand(c.operatorPrincipalAddCmd())
	cmd.AddCommand(c.operatorPrincipalRemoveCmd())
	cmd.AddCommand(c.operatorPrincipalListCmd())
	return cmd
}

func (c *cli) operatorPrincipalAddCmd() *cobra.Command {
	var principal string

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Grant admin UI access to a principal",
		RunE: func(_ *cobra.Command, _ []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			return c.handler.OperatorPrincipalAdd(prof, principal)
		},
	}
	cmd.Flags().StringVar(&principal, "principal", "", "principal identity (email)")
	if err := cmd.MarkFlagRequired("principal"); err != nil {
		panic(err)
	}
	return cmd
}

func (c *cli) operatorPrincipalRemoveCmd() *cobra.Command {
	var principal string

	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Revoke admin UI access from a principal",
		RunE: func(_ *cobra.Command, _ []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			return c.handler.OperatorPrincipalRemove(prof, principal)
		},
	}
	cmd.Flags().StringVar(&principal, "principal", "", "principal identity (email)")
	if err := cmd.MarkFlagRequired("principal"); err != nil {
		panic(err)
	}
	return cmd
}

func (c *cli) operatorPrincipalListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all principals with admin UI access",
		RunE: func(_ *cobra.Command, _ []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			return c.handler.OperatorPrincipalList(prof)
		},
	}
}

func (h *cliHandler) OperatorPrincipalAdd(_ *Profile, _ string) error {
	return fmt.Errorf("not yet implemented")
}

func (h *cliHandler) OperatorPrincipalRemove(_ *Profile, _ string) error {
	return fmt.Errorf("not yet implemented")
}

func (h *cliHandler) OperatorPrincipalList(_ *Profile) error {
	return fmt.Errorf("not yet implemented")
}
