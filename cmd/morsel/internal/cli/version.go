package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ifeanyiecheruo/morsel/internal/version"
)

func (c *cli) versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version and build information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), version.Get())
			return err
		},
	}
}
