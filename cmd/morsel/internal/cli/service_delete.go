package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel/internal/client/oas"
)

func (c *cli) serviceDeleteCmd() *cobra.Command {
	var confirmed bool
	var kubecontext, namespace string

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
			return c.handler.ServiceDelete(cmd.Context(), prof, kubecontext, namespace)
		},
	}
	cmd.Flags().BoolVar(&confirmed, "confirm", false, "required: confirm destructive teardown")
	cmd.Flags().StringVar(&kubecontext, "kubecontext", "", "kubectl context to use in teardown instructions (for use when the cluster is unreachable)")
	cmd.Flags().StringVar(&namespace, "namespace", "", "Kubernetes namespace override for teardown instructions (default: from service)")
	return cmd
}

func (h *cliHandler) ServiceDelete(ctx context.Context, prof *Profile, kubecontext, namespace string) error {
	if namespace == "" {
		if client, err := h.clientFor(prof); err == nil {
			if resp, err := client.Inner().GetDeploymentInfo(ctx); err == nil {
				if info, ok := resp.(*oas.DeploymentInfo); ok {
					namespace = info.Namespace
				}
			}
		}
	}
	if namespace == "" {
		namespace = "morsel"
	}

	fmt.Println("✓ Bootstrap state deleted.")
	fmt.Println()
	fmt.Println("  Cluster resources were NOT removed automatically.")
	fmt.Println("  Delete them manually:")
	fmt.Println()

	kubectl := "kubectl"
	if kubecontext != "" {
		kubectl += " --context " + kubecontext
	}
	fmt.Printf("    %s delete namespace %s %s-services --ignore-not-found\n\n", kubectl, namespace, namespace)

	return nil
}
