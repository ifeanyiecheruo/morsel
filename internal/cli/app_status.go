package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ifeanyiecheruo/morsel/internal/api/oas"
)

func (c *cli) appStatusCmd(org, repo *string) *cobra.Command {
	return &cobra.Command{
		Use:   "status NAME",
		Short: "Show the live pod status of an app",
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
			return c.handler.AppStatus(cmd.Context(), prof, resolvedOrg, resolvedRepo, args[0])
		},
	}
}

func (h *cliHandler) AppStatus(ctx context.Context, prof *Profile, org, repo, name string) error {
	client, err := h.clientFor(prof)
	if err != nil {
		return err
	}
	res, err := client.Inner().GetAppStatus(ctx, oas.GetAppStatusParams{Org: org, Repo: repo, Name: name})
	if err != nil {
		return fmt.Errorf("get app status: %w", err)
	}
	status, ok := res.(*oas.GetAppStatusOK)
	if !ok {
		return fmt.Errorf("unexpected response: %T", res)
	}
	fmt.Println(status.Status)
	return nil
}
