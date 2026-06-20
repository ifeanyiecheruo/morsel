package cli

import (
	"context"
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
		RunE: func(cmd *cobra.Command, _ []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			return c.handler.OperatorPrincipalAdd(cmd.Context(), prof, principal)
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
		RunE: func(cmd *cobra.Command, _ []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			return c.handler.OperatorPrincipalRemove(cmd.Context(), prof, principal)
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
		RunE: func(cmd *cobra.Command, _ []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			return c.handler.OperatorPrincipalList(cmd.Context(), prof)
		},
	}
}

func (h *cliHandler) OperatorPrincipalAdd(_ context.Context, _ *Profile, _ string) error {
	return fmt.Errorf("not yet implemented")
}

func (h *cliHandler) OperatorPrincipalRemove(_ context.Context, _ *Profile, _ string) error {
	return fmt.Errorf("not yet implemented")
}

func (h *cliHandler) OperatorPrincipalList(_ context.Context, _ *Profile) error {
	return fmt.Errorf("not yet implemented")
}
