package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ifeanyiecheruo/morsel/internal/api/oas"
)

func (c *cli) appGetCmd(org, repo *string) *cobra.Command {
	return &cobra.Command{
		Use:   "get NAME",
		Short: "Show details of a single app",
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
			return c.handler.AppGet(cmd.Context(), prof, resolvedOrg, resolvedRepo, args[0])
		},
	}
}

func (h *cliHandler) AppGet(ctx context.Context, prof *Profile, org, repo, name string) error {
	client, err := h.clientFor(prof)
	if err != nil {
		return err
	}
	res, err := client.Inner().GetApp(ctx, oas.GetAppParams{Org: org, Repo: repo, Name: name})
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}
	app, ok := res.(*oas.App)
	if !ok {
		return fmt.Errorf("unexpected response: %T", res)
	}
	fmt.Printf("Name:      %s\n", app.Name.Or("(unnamed)"))
	fmt.Printf("Type:      %s\n", string(app.Type))
	fmt.Printf("Status:    %s\n", app.Status)
	fmt.Printf("Image:     %s\n", app.Image.Or("-"))
	fmt.Printf("Namespace: %s\n", app.Namespace.Or("-"))
	if app.CreatedAt.Set {
		fmt.Printf("Created:   %s\n", app.CreatedAt.Value.Format("2006-01-02 15:04"))
	}
	if app.UpdatedAt.Set {
		fmt.Printf("Updated:   %s\n", app.UpdatedAt.Value.Format("2006-01-02 15:04"))
	}
	return nil
}
