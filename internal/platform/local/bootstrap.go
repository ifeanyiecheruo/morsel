package local

import (
	"context"
	"fmt"

	"github.com/ifeanyiecheruo/morsel/internal/platform"
	"github.com/ifeanyiecheruo/morsel/internal/secrets"
)

// AnswerKeyKubeconfig is the answers-map key injected by the bootstrap command
// handler to pass the kubeconfig path to Provision(). It is not a wizard prompt.
const AnswerKeyKubeconfig = "_kubeconfig"

type localBootstrapper struct {
	secretMgr *secrets.Manager
}

func (lb *localBootstrapper) Prompts() []platform.Prompt {
	return []platform.Prompt{
		{
			Key:      "github_org",
			Label:    "GitHub organisation slug",
			Required: true,
		},
		{
			Key:     "domain",
			Label:   "Domain",
			Default: "morsel.localhost",
		},
		{
			Key:     "k8s_namespace",
			Label:   "Base Kubernetes namespace",
			Default: "morsel",
		},
	}
}

func (lb *localBootstrapper) Plan(answers map[string]string) platform.Plan {
	ns := answers["k8s_namespace"]
	if ns == "" {
		ns = "morsel"
	}
	svcNS := ns + "-services"
	domain := answers["domain"]
	if domain == "" {
		domain = "morsel.localhost"
	}
	return platform.Plan{
		Summary: "A full Morsel control plane will be provisioned inside your local Kubernetes cluster. No cloud account or billing is required.",
		Resources: []platform.Resource{
			{Name: "Local container registry", Description: "registry:2 in namespace " + ns},
			{Name: "Morsel API", Description: "Control plane service in namespace " + ns},
			{Name: "Blob service", Description: "Object storage service in namespace " + svcNS},
			{Name: "Queue service", Description: "Async messaging service in namespace " + svcNS},
			{Name: "Shared Postgres", Description: "Database service in namespace " + svcNS},
			{Name: "Envoy Gateway", Description: "Ingress for *." + domain},
			{Name: "Self-signed TLS certificate", Description: "Wildcard cert for *." + domain},
		},
	}
}

// Provision writes bootstrap configuration and generates cryptographic keys.
// When answers includes the "_kubeconfig" key, it also verifies cluster access.
// Actual Kubernetes resource installation is performed by the bootstrap command
// handler after Provision returns — Provision is intentionally limited to state
// that must survive across bootstrap re-runs regardless of cluster state.
func (lb *localBootstrapper) Provision(ctx context.Context, answers map[string]string) error {
	if lb.secretMgr == nil {
		return platform.ErrNotImplemented
	}

	// Verify cluster access when a kubeconfig path was supplied.
	if kubeconfigPath := answers[AnswerKeyKubeconfig]; kubeconfigPath != "" {
		kc, err := LoadKubeconfig(kubeconfigPath, "")
		if err != nil {
			return fmt.Errorf("load kubeconfig: %w", err)
		}
		if err := kc.CheckAccess(ctx); err != nil {
			return fmt.Errorf("cluster access check: %w", err)
		}
	}

	// Generate deploy signing key (idempotent — no-op if already present).
	if _, err := lb.secretMgr.DeploySigningKey(ctx); err != nil {
		return fmt.Errorf("generate deploy signing key: %w", err)
	}

	// Persist wizard answers so subsequent bootstrap runs can skip the wizard.
	configToSave := make(map[string]string, len(answers))
	for k, v := range answers {
		if k != AnswerKeyKubeconfig { // do not persist the kubeconfig path
			configToSave[k] = v
		}
	}
	if err := lb.secretMgr.SetBootstrapConfig(ctx, configToSave); err != nil {
		return fmt.Errorf("persist bootstrap config: %w", err)
	}

	return nil
}
