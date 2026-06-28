package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func (c *cli) operatorAppExemptParentCmd() *cobra.Command {
	appCmd := &cobra.Command{
		Use:   "app",
		Short: "App-scoped operator commands",
	}
	exemptCmd := &cobra.Command{
		Use:   "exempt",
		Short: "Manage per-app budget-control exemptions",
	}
	exemptCmd.AddCommand(c.operatorAppExemptAddCmd())
	exemptCmd.AddCommand(c.operatorAppExemptRemoveCmd())
	appCmd.AddCommand(exemptCmd)
	return appCmd
}

func (c *cli) operatorRepoExemptParentCmd() *cobra.Command {
	repoCmd := &cobra.Command{
		Use:   "repo",
		Short: "Repo-scoped operator commands",
	}
	exemptCmd := &cobra.Command{
		Use:   "exempt",
		Short: "Manage per-repo budget-control exemptions",
	}
	exemptCmd.AddCommand(c.operatorRepoExemptAddCmd())
	exemptCmd.AddCommand(c.operatorRepoExemptRemoveCmd())
	repoCmd.AddCommand(exemptCmd)
	return repoCmd
}

func (c *cli) operatorAppExemptAddCmd() *cobra.Command {
	var repo, app string

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a budget-control exemption for a specific app",
		RunE: func(cmd *cobra.Command, _ []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			return c.handler.AppExemptAdd(cmd.Context(), prof, repo, app)
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "repository (org/repo)")
	cmd.Flags().StringVar(&app, "app", "", "app name")
	if err := cmd.MarkFlagRequired("repo"); err != nil {
		panic(err)
	}
	if err := cmd.MarkFlagRequired("app"); err != nil {
		panic(err)
	}
	return cmd
}

func (c *cli) operatorAppExemptRemoveCmd() *cobra.Command {
	var repo, app string

	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove a budget-control exemption for a specific app",
		RunE: func(cmd *cobra.Command, _ []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			return c.handler.AppExemptRemove(cmd.Context(), prof, repo, app)
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "repository (org/repo)")
	cmd.Flags().StringVar(&app, "app", "", "app name")
	if err := cmd.MarkFlagRequired("repo"); err != nil {
		panic(err)
	}
	if err := cmd.MarkFlagRequired("app"); err != nil {
		panic(err)
	}
	return cmd
}

func (c *cli) operatorRepoExemptAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <org/repo>",
		Short: "Add a budget-control exemption for all apps in a repo",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			return c.handler.RepoExemptAdd(cmd.Context(), prof, args[0])
		},
	}
}

func (c *cli) operatorRepoExemptRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <org/repo>",
		Short: "Remove a budget-control exemption for all apps in a repo",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			return c.handler.RepoExemptRemove(cmd.Context(), prof, args[0])
		},
	}
}

func (h *cliHandler) AppExemptAdd(_ context.Context, _ *Profile, _, _ string) error {
	return fmt.Errorf("not yet implemented")
}

func (h *cliHandler) AppExemptRemove(_ context.Context, _ *Profile, _, _ string) error {
	return fmt.Errorf("not yet implemented")
}

func (h *cliHandler) RepoExemptAdd(_ context.Context, _ *Profile, _ string) error {
	return fmt.Errorf("not yet implemented")
}

func (h *cliHandler) RepoExemptRemove(_ context.Context, _ *Profile, _ string) error {
	return fmt.Errorf("not yet implemented")
}
