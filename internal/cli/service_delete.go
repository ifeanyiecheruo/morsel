package cli

import (
	"context"
	"fmt"

	"github.com/ifeanyiecheruo/morsel/internal/platform"
	"github.com/ifeanyiecheruo/morsel/internal/secrets"
	"github.com/spf13/cobra"
)

func (c *cli) serviceDeleteCmd() *cobra.Command {
	var confirmed bool

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Tear down all platform resources",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !confirmed {
				return fmt.Errorf("pass --confirm to acknowledge this will destroy all platform resources")
			}
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			plat, err := c.requirePlatform()
			if err != nil {
				return err
			}
			return c.handler.ServiceDelete(cmd.Context(), plat, prof)
		},
	}
	cmd.Flags().BoolVar(&confirmed, "confirm", false, "required: confirm destructive teardown")
	return cmd
}

func (h *cliHandler) ServiceDelete(ctx context.Context, plat platform.Platform, prof *Profile) error {
	mgr := secrets.New(plat.Secrets())

	// Wipe the signing key and bootstrap config from the local secret store so
	// that the next bootstrap starts clean. Keys and tokens issued before this
	// point are immediately invalidated by their absence.
	if err := mgr.DeleteBootstrapState(ctx); err != nil {
		return fmt.Errorf("delete bootstrap state: %w", err)
	}

	fmt.Println("✓ Bootstrap state deleted.")

	if prof.ClusterServer != "" {
		fmt.Println()
		fmt.Printf("  Cluster resources were NOT removed automatically.\n")
		fmt.Printf("  Delete them manually:\n\n")
		fmt.Printf("    kubectl --context %s delete namespace morsel morsel-services --ignore-not-found\n\n", prof.Kubecontext)
	}

	return nil
}
