package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func (c *cli) serviceDeleteCmd() *cobra.Command {
	var confirmed bool

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Tear down all platform resources",
		RunE: func(_ *cobra.Command, _ []string) error {
			if !confirmed {
				return fmt.Errorf("pass --confirm to acknowledge this will destroy all platform resources")
			}
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			return c.handler.ServiceDelete(prof)
		},
	}
	cmd.Flags().BoolVar(&confirmed, "confirm", false, "required: confirm destructive teardown")
	return cmd
}

func (h *cliHandler) ServiceDelete(_ *Profile) error {
	return fmt.Errorf("not yet implemented")
}
