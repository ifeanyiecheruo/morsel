package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func (c *cli) operatorLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Revoke authentication and delete the profile",
		RunE: func(cmd *cobra.Command, _ []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			if err := c.handler.OperatorLogout(cmd.Context(), prof); err != nil {
				return err
			}
			return deleteProfile(c.profileName)
		},
	}
}

func (h *cliHandler) OperatorLogout(_ context.Context, _ *Profile) error {
	return fmt.Errorf("not yet implemented")
}
