package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ifeanyiecheruo/morsel/internal/platform"
	"github.com/ifeanyiecheruo/morsel/internal/platform/local"
	"github.com/ifeanyiecheruo/morsel/internal/platforms"
	"github.com/ifeanyiecheruo/morsel/internal/secrets"
	"github.com/spf13/cobra"
)

func (c *cli) serviceBootstrapCmd() *cobra.Command {
	var platformFlag string
	var kubeconfigFlag string

	cmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Install or upgrade the platform",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if kubeconfigFlag != "" && platformFlag != "local" {
				return fmt.Errorf("--kubeconfig is only supported with --platform local")
			}
			prof, err := c.handler.ServiceBootstrap(cmd.Context(), platformFlag, kubeconfigFlag)
			if err != nil {
				return err
			}
			// Preserve existing token fields so a re-bootstrap does not log the operator out.
			if c.profile != nil {
				prof.AccessToken = c.profile.AccessToken
				prof.AccessTokenExpiresAt = c.profile.AccessTokenExpiresAt
				prof.RefreshToken = c.profile.RefreshToken
				prof.RefreshTokenExpiresAt = c.profile.RefreshTokenExpiresAt
			}
			return c.handler.SaveProfile(cmd.Context(), c.profileName, prof)
		},
	}
	cmd.Flags().StringVar(&platformFlag, "platform", "", "platform implementation to use (gcp|local)")
	cmd.Flags().StringVar(&kubeconfigFlag, "kubeconfig", "", "path to kubeconfig file (local platform only)")
	if err := cmd.MarkFlagRequired("platform"); err != nil {
		panic(err)
	}
	return cmd
}

// TODO: we have lots of code in here that is specific to the local platform
// thats what the platform interface is for, we need to redesign the interface to eliminate this
func (h *cliHandler) ServiceBootstrap(ctx context.Context, platformName, kubeconfig string) (*Profile, error) {
	plat, err := platforms.Create(platformName)
	if err != nil {
		return nil, fmt.Errorf("unknown platform %q: %w", platformName, err)
	}

	// Resolve kubeconfig path for local platform.
	if platformName == "local" && kubeconfig == "" {
		kubeconfig = local.DefaultKubeconfigPath()
	}

	// Phase 1 — verify cluster access before prompting so a bad config is caught early.
	var kc *local.KubeconfigContext
	if platformName == "local" {
		fmt.Printf("  Checking cluster access... ")
		kc, err = local.LoadKubeconfig(kubeconfig, "")
		if err != nil {
			fmt.Println("✗")
			fmt.Printf("\n  Could not read kubeconfig at %s.\n", kubeconfig)
			fmt.Println("  Possible remediation:")
			fmt.Println("    • Start your local Kubernetes cluster (Docker Desktop, Rancher Desktop, kind, …)")
			fmt.Println("    • If the kubeconfig is in a non-default location, use --kubeconfig to specify the path")
			fmt.Println("    • Verify the file exists: kubectl config view")
			fmt.Println()
			return nil, fmt.Errorf("kubeconfig not found or unreadable")
		}
		if err := kc.CheckAccess(ctx); err != nil {
			fmt.Println("✗")
			fmt.Printf("\n  Cannot reach cluster %q at %s.\n", kc.ContextName, kc.ServerURL)
			fmt.Println("  Possible remediation:")
			fmt.Println("    • Start your local Kubernetes cluster (Docker Desktop, Rancher Desktop, kind, …)")
			fmt.Println("    • Check the active context:  kubectl config current-context")
			fmt.Println("    • Verify connectivity:       kubectl cluster-info")
			fmt.Printf("    • Kubeconfig in use:         %s\n\n", kubeconfig)
			return nil, fmt.Errorf("cluster %q is not reachable", kc.ContextName)
		}
		fmt.Printf("✓ %s\n", kc.ServerURL)
	}

	// Check for a previously saved wizard config so re-runs skip the wizard.
	mgr := secrets.New(plat.Secrets())
	savedConfig, err := mgr.BootstrapConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load existing bootstrap config: %w", err)
	}

	var answers map[string]string
	if savedConfig != nil {
		fmt.Println("  Existing bootstrap configuration found — skipping wizard.")
		answers = savedConfig
	} else {
		answers, err = runBootstrapWizard(plat.Bootstrap().Prompts())
		if err != nil {
			return nil, err
		}

		plan := plat.Bootstrap().Plan(answers)
		printBootstrapPlan(plan)

		if !bootstrapConfirm("Proceed with provisioning? [y/N]: ") {
			return nil, fmt.Errorf("bootstrap cancelled")
		}
	}

	// Phase 2 — provision (writes signing keys + persists bootstrap config).
	answers[local.AnswerKeyKubeconfig] = kubeconfig
	fmt.Printf("  Provisioning... ")
	if err := plat.Bootstrap().Provision(ctx, answers); err != nil {
		fmt.Println("✗")
		return nil, err
	}
	fmt.Println("✓")

	// Build the profile with cluster connection info.
	prof := &Profile{Platform: platformName}
	if kc != nil {
		prof.Kubeconfig = kubeconfig
		prof.Kubecontext = kc.ContextName
		prof.ClusterServer = kc.ServerURL
		// Default local API URL; updated once Envoy Gateway is configured.
		prof.APIURL = "https://morsel-api.morsel.svc.cluster.local:8080"
	}

	fmt.Println("✓ Bootstrap complete. Run 'morsel operator login' to authenticate.")
	return prof, nil
}

// TODO: The code to take prompts and turn then into a map of answers should and to print plans, should be in its own file and testable independent of the cli command handlers
// runBootstrapWizard presents each prompt interactively and returns a map of answers.
func runBootstrapWizard(prompts []platform.Prompt) (map[string]string, error) {
	reader := bufio.NewReader(os.Stdin)
	answers := make(map[string]string, len(prompts))
	for _, p := range prompts {
		label := p.Label
		if p.Default != "" {
			label = fmt.Sprintf("%s [%s]", label, p.Default)
		}
		fmt.Printf("  %s: ", label)
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", p.Key, err)
		}
		val := strings.TrimRight(line, "\r\n")
		if val == "" {
			val = p.Default
		}
		if p.Required && val == "" {
			return nil, fmt.Errorf("%q is required", p.Label)
		}
		answers[p.Key] = val
	}
	return answers, nil
}

func printBootstrapPlan(plan platform.Plan) {
	fmt.Printf("\n  %s\n\n", plan.Summary)
	fmt.Println("  Resources to be created:")
	for _, r := range plan.Resources {
		fmt.Printf("    • %s — %s\n", r.Name, r.Description)
	}
	fmt.Println()
}

func bootstrapConfirm(prompt string) bool {
	fmt.Print("  " + prompt)
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	ans := strings.TrimRight(line, "\r\n")
	return strings.EqualFold(ans, "y") || strings.EqualFold(ans, "yes")
}
