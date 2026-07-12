package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel/internal/client/oas"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
	"github.com/spf13/cobra"
)

func (c *cli) operatorPrincipalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "principal",
		Short: "Manage principals with operator or admin access",
	}
	cmd.AddCommand(c.operatorPrincipalListCmd())
	cmd.AddCommand(c.operatorPrincipalGrantOperatorCmd())
	cmd.AddCommand(c.operatorPrincipalRevokeOperatorCmd())
	cmd.AddCommand(c.operatorPrincipalGrantAdminCmd())
	cmd.AddCommand(c.operatorPrincipalRevokeAdminCmd())
	cmd.AddCommand(c.operatorPrincipalRemoveCmd())
	return cmd
}

func (c *cli) operatorPrincipalListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all principals with operator or admin access",
		RunE: func(cmd *cobra.Command, _ []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			return c.handler.OperatorPrincipalList(cmd.Context(), prof)
		},
	}
}

func (c *cli) operatorPrincipalGrantOperatorCmd() *cobra.Command {
	var login string
	cmd := &cobra.Command{
		Use:   "grant-operator",
		Short: "Grant operator role to a GitHub user",
		RunE: func(cmd *cobra.Command, _ []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			return c.handler.OperatorPrincipalAdd(cmd.Context(), prof, login)
		},
	}
	cmd.Flags().StringVar(&login, "login", "", "GitHub login of the principal")
	if err := cmd.MarkFlagRequired("login"); err != nil {
		panic(err)
	}
	return cmd
}

func (c *cli) operatorPrincipalRevokeOperatorCmd() *cobra.Command {
	var login string
	cmd := &cobra.Command{
		Use:   "revoke-operator",
		Short: "Revoke operator role from a principal",
		RunE: func(cmd *cobra.Command, _ []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			f := false
			return c.handler.OperatorPrincipalPatch(cmd.Context(), prof, login, &f, nil)
		},
	}
	cmd.Flags().StringVar(&login, "login", "", "GitHub login of the principal")
	if err := cmd.MarkFlagRequired("login"); err != nil {
		panic(err)
	}
	return cmd
}

func (c *cli) operatorPrincipalGrantAdminCmd() *cobra.Command {
	var login string
	cmd := &cobra.Command{
		Use:   "grant-admin",
		Short: "Grant admin role to a principal",
		RunE: func(cmd *cobra.Command, _ []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			t := true
			return c.handler.OperatorPrincipalPatch(cmd.Context(), prof, login, nil, &t)
		},
	}
	cmd.Flags().StringVar(&login, "login", "", "GitHub login of the principal")
	if err := cmd.MarkFlagRequired("login"); err != nil {
		panic(err)
	}
	return cmd
}

func (c *cli) operatorPrincipalRevokeAdminCmd() *cobra.Command {
	var login string
	cmd := &cobra.Command{
		Use:   "revoke-admin",
		Short: "Revoke admin role from a principal",
		RunE: func(cmd *cobra.Command, _ []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			f := false
			return c.handler.OperatorPrincipalPatch(cmd.Context(), prof, login, nil, &f)
		},
	}
	cmd.Flags().StringVar(&login, "login", "", "GitHub login of the principal")
	if err := cmd.MarkFlagRequired("login"); err != nil {
		panic(err)
	}
	return cmd
}

func (c *cli) operatorPrincipalRemoveCmd() *cobra.Command {
	var login string
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove a principal entirely",
		RunE: func(cmd *cobra.Command, _ []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			return c.handler.OperatorPrincipalRemove(cmd.Context(), prof, login)
		},
	}
	cmd.Flags().StringVar(&login, "login", "", "GitHub login of the principal")
	if err := cmd.MarkFlagRequired("login"); err != nil {
		panic(err)
	}
	return cmd
}

// ── cliHandler implementations ────────────────────────────────────────────────

func (h *cliHandler) OperatorPrincipalAdd(ctx context.Context, prof *Profile, login string) error {
	c, err := h.clientFor(prof)
	if err != nil {
		return err
	}
	res, err := c.Inner().AddOperatorPrincipal(ctx, &oas.PrincipalReq{Principal: login})
	if err != nil {
		return fmt.Errorf("grant operator: %w", err)
	}
	switch res.(type) {
	case *oas.OperatorPrincipals:
		fmt.Printf("Granted operator role to %s.\n", login)
		return nil
	default:
		return fmt.Errorf("unexpected response: %T", res)
	}
}

func (h *cliHandler) OperatorPrincipalRemove(ctx context.Context, prof *Profile, login string) error {
	c, err := h.clientFor(prof)
	if err != nil {
		return err
	}
	res, err := c.Inner().RemoveOperatorPrincipal(ctx, oas.RemoveOperatorPrincipalParams{Principal: login})
	if err != nil {
		return fmt.Errorf("remove principal: %w", err)
	}
	switch res.(type) {
	case *oas.OperatorPrincipals:
		fmt.Printf("Removed %s.\n", login)
		return nil
	default:
		return fmt.Errorf("unexpected response: %T", res)
	}
}

func (h *cliHandler) OperatorPrincipalList(ctx context.Context, prof *Profile) error {
	c, err := h.clientFor(prof)
	if err != nil {
		return err
	}
	res, err := c.Inner().ListOperatorPrincipals(ctx)
	if err != nil {
		return fmt.Errorf("list principals: %w", err)
	}
	r, ok := res.(*oas.OperatorPrincipals)
	if !ok {
		return fmt.Errorf("unexpected response: %T", res)
	}
	if len(r.Principals) == 0 {
		fmt.Println("  (no principals configured)")
		return nil
	}
	for _, p := range r.Principals {
		role := "operator"
		if p.IsAdmin {
			role = "admin"
		}
		fmt.Printf("  %-30s %s\n", p.GithubLogin, role)
	}
	return nil
}

func (h *cliHandler) OperatorPrincipalPatch(ctx context.Context, prof *Profile, login string, isOperator, isAdmin *bool) error {
	type patchReq struct {
		IsOperator *bool `json:"is_operator,omitempty"`
		IsAdmin    *bool `json:"is_admin,omitempty"`
	}
	body, err := json.Marshal(patchReq{IsOperator: isOperator, IsAdmin: isAdmin})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch,
		prof.APIURL+"/api/operator/principals/"+url.PathEscape(login),
		bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+prof.AccessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("patch principal: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			ctxlog.From(ctx).Warn("close response body", "err", closeErr)
		}
	}()

	switch resp.StatusCode {
	case http.StatusOK:
		fmt.Printf("Updated roles for %s.\n", login)
		return nil
	case http.StatusNotFound:
		return fmt.Errorf("principal %q not found", login)
	case http.StatusForbidden:
		return fmt.Errorf("admin role required to manage principals")
	default:
		return fmt.Errorf("patch returned %d", resp.StatusCode)
	}
}
