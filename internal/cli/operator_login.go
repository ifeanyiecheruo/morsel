package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func (c *cli) operatorLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Authenticate to the Morsel instance",
		RunE: func(cmd *cobra.Command, _ []string) error {
			prof, err := c.handler.OperatorLogin(cmd.Context())
			if err != nil {
				return err
			}
			return writeProfile(c.profileName, prof)
		},
	}
}

func (h *cliHandler) OperatorLogin(_ context.Context) (*Profile, error) {
	return nil, fmt.Errorf("not yet implemented")
}
