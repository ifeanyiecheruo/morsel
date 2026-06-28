package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func (c *cli) serviceStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Report the health of all platform components",
		RunE: func(cmd *cobra.Command, _ []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			return c.handler.ServiceStatus(cmd.Context(), prof)
		},
	}
}

func (h *cliHandler) ServiceStatus(ctx context.Context, prof *Profile) error {
	if prof.APIURL == "" {
		fmt.Println("  ✗ API: no URL configured (run 'morsel service bootstrap' first)")
		return nil
	}

	client, err := h.clientFor(prof)
	if err != nil {
		fmt.Printf("  ✗ API: build client: %v\n", err)
		return nil
	}

	resp, err := client.Inner().GetHealthz(ctx)
	if err != nil {
		fmt.Printf("  ✗ API (%s): %v\n", prof.APIURL, err)
	} else {
		fmt.Printf("  ✓ API (%s): %s\n", prof.APIURL, resp.Status)
	}

	return nil
}
