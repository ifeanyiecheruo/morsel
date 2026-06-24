package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ifeanyiecheruo/morsel/internal/api/oas"
)

func (c *cli) appHistoryCmd(org, repo *string) *cobra.Command {
	return &cobra.Command{
		Use:   "history NAME",
		Short: "Show the operation history of an app",
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
			return c.handler.AppHistory(cmd.Context(), prof, resolvedOrg, resolvedRepo, args[0])
		},
	}
}

func (h *cliHandler) AppHistory(ctx context.Context, prof *Profile, org, repo, name string) error {
	client, err := h.clientFor(prof)
	if err != nil {
		return err
	}
	res, err := client.Inner().GetAppHistory(ctx, oas.GetAppHistoryParams{Org: org, Repo: repo, Name: name})
	if err != nil {
		return fmt.Errorf("get app history: %w", err)
	}
	ops, ok := res.(*oas.GetAppHistoryOKApplicationJSON)
	if !ok {
		return fmt.Errorf("unexpected response: %T", res)
	}
	if len(*ops) == 0 {
		fmt.Println("  (no operations)")
		return nil
	}
	fmt.Printf("%-38s %-10s %-10s %-18s %s\n", "ID", "TYPE", "STATUS", "STARTED", "COMPLETED")
	for _, op := range *ops {
		completed := "-"
		if op.CompletedAt.Set && !op.CompletedAt.Null {
			completed = op.CompletedAt.Value.Format("2006-01-02 15:04")
		}
		fmt.Printf("%-38s %-10s %-10s %-18s %s\n",
			op.ID,
			op.Type,
			string(op.Status),
			op.CreatedAt.Format("2006-01-02 15:04"),
			completed,
		)
	}
	return nil
}
