package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ifeanyiecheruo/morsel/internal/api/oas"
)

func (c *cli) appListCmd(org, repo *string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all apps in a repository",
		RunE: func(cmd *cobra.Command, _ []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			resolvedOrg, resolvedRepo, err := resolveOrgRepo(*org, *repo)
			if err != nil {
				return err
			}
			return c.handler.AppList(cmd.Context(), prof, resolvedOrg, resolvedRepo)
		},
	}
}

func (h *cliHandler) AppList(ctx context.Context, prof *Profile, org, repo string) error {
	client, err := h.clientFor(prof)
	if err != nil {
		return err
	}
	res, err := client.Inner().ListApps(ctx, oas.ListAppsParams{Org: org, Repo: repo})
	if err != nil {
		return fmt.Errorf("list apps: %w", err)
	}
	apps, ok := res.(*oas.ListAppsOKApplicationJSON)
	if !ok {
		return fmt.Errorf("unexpected response: %T", res)
	}
	if len(*apps) == 0 {
		fmt.Println("  (no apps)")
		return nil
	}
	fmt.Printf("%-20s %-10s %-12s %s\n", "NAME", "TYPE", "STATUS", "IMAGE")
	for _, app := range *apps {
		fmt.Printf("%-20s %-10s %-12s %s\n",
			app.Name.Or("(unnamed)"),
			string(app.Type),
			app.Status,
			app.Image.Or("-"),
		)
	}
	return nil
}
