package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ifeanyiecheruo/morsel/internal/api/oas"
)

func (c *cli) appDeleteCmd(org, repo *string) *cobra.Command {
	return &cobra.Command{
		Use:   "delete NAME",
		Short: "Delete an app and its resources",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			resolvedOrg, resolvedRepo, err := resolveOrgRepo(*org, *repo)
			if err != nil {
				return err
			}
			return c.handler.AppDelete(cmd.Context(), prof, resolvedOrg, resolvedRepo, args[0])
		},
	}
}

func (h *cliHandler) AppDelete(ctx context.Context, prof *Profile, org, repo, name string) error {
	client, err := h.clientFor(prof)
	if err != nil {
		return err
	}
	res, err := client.Inner().DeleteApp(ctx, oas.DeleteAppParams{Org: org, Repo: repo, Name: name})
	if err != nil {
		return fmt.Errorf("delete app: %w", err)
	}
	headers, ok := res.(*oas.AcceptedOperationHeaders)
	if !ok {
		return fmt.Errorf("unexpected response: %T", res)
	}

	opID := headers.Response.OperationID
	fmt.Printf("Deleting %s/%s/%s (operation %s)...\n", org, repo, name, opID)
	return pollOperation(ctx, client, org, repo, name, opID)
}
