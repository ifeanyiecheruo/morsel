package cli

import (
	"context"
	"fmt"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel/internal/client/oas"
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

func (h *cliHandler) OperatorPrincipalAdd(ctx context.Context, prof *Profile, principal string) error {
	client, err := h.clientFor(prof)
	if err != nil {
		return err
	}
	res, err := client.Inner().AddOperatorPrincipal(ctx, &oas.PrincipalReq{Principal: principal})
	if err != nil {
		return fmt.Errorf("add principal: %w", err)
	}
	switch r := res.(type) {
	case *oas.OperatorPrincipals:
		_ = r
		fmt.Printf("✓ Added %s as a principal.\n", principal)
		return nil
	default:
		return fmt.Errorf("unexpected response: %T", res)
	}
}

func (h *cliHandler) OperatorPrincipalRemove(ctx context.Context, prof *Profile, principal string) error {
	client, err := h.clientFor(prof)
	if err != nil {
		return err
	}
	res, err := client.Inner().RemoveOperatorPrincipal(ctx, oas.RemoveOperatorPrincipalParams{Principal: principal})
	if err != nil {
		return fmt.Errorf("remove principal: %w", err)
	}
	switch r := res.(type) {
	case *oas.OperatorPrincipals:
		_ = r
		fmt.Printf("✓ Removed %s from principals.\n", principal)
		return nil
	default:
		return fmt.Errorf("unexpected response: %T", res)
	}
}

func (h *cliHandler) OperatorPrincipalList(ctx context.Context, prof *Profile) error {
	client, err := h.clientFor(prof)
	if err != nil {
		return err
	}
	res, err := client.Inner().ListOperatorPrincipals(ctx)
	if err != nil {
		return fmt.Errorf("list principals: %w", err)
	}
	switch r := res.(type) {
	case *oas.OperatorPrincipals:
		if len(r.Principals) == 0 {
			fmt.Println("  (no principals configured)")
			return nil
		}
		for _, p := range r.Principals {
			fmt.Printf("  %s\n", p)
		}
		return nil
	default:
		return fmt.Errorf("unexpected response: %T", res)
	}
}
