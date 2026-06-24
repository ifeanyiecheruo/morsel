package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ifeanyiecheruo/morsel/internal/api/oas"
)

func (c *cli) appSyncCmd(org, repo *string) *cobra.Command {
	return &cobra.Command{
		Use:   "sync NAME",
		Short: "Write server app state to .morsel/NAME.morsel.json",
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
			return c.handler.AppSync(cmd.Context(), prof, resolvedOrg, resolvedRepo, args[0])
		},
	}
}

func (h *cliHandler) AppSync(ctx context.Context, prof *Profile, org, repo, name string) error {
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

	cfg := morselConfig{
		Name: app.Name.Or(""),
		Type: apiTypeToMorsel(string(app.Type)),
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.MkdirAll(".morsel", 0o755); err != nil {
		return fmt.Errorf("create .morsel directory: %w", err)
	}
	path := ".morsel/" + name + ".morsel.json"
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	fmt.Printf("wrote %s\n", path)
	return nil
}

// apiTypeToMorsel converts an API app type back to the morsel.json type name.
func apiTypeToMorsel(apiType string) string {
	if apiType == "cron" {
		return "cronjob"
	}
	return apiType
}
