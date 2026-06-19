package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func (c *cli) operatorLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Authenticate to the Morsel instance",
		RunE: func(_ *cobra.Command, _ []string) error {
			prof, err := c.handler.OperatorLogin()
			if err != nil {
				return err
			}
			return writeProfile(c.profileName, prof)
		},
	}
}

func (h *cliHandler) OperatorLogin() (*Profile, error) {
	return nil, fmt.Errorf("not yet implemented")
}
