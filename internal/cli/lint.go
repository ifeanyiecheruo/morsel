package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func (c *cli) lintCmd() *cobra.Command {
	var staged bool
	var fix bool

	cmd := &cobra.Command{
		Use:   "lint",
		Short: "Validate *.morsel.json files in .morsel/",
		RunE: func(_ *cobra.Command, _ []string) error {
			return c.handler.Lint(staged, fix)
		},
	}
	cmd.Flags().BoolVar(&staged, "staged", false, "validate only git-staged files")
	cmd.Flags().BoolVar(&fix, "fix", false, "auto-remediate safe issues")
	return cmd
}

func (h *cliHandler) Lint(_, _ bool) error {
	return fmt.Errorf("not yet implemented")
}
