package local

import (
	"context"
	"fmt"

	"github.com/ifeanyiecheruo/morsel/internal/platform"
)

type localBootstrapper struct {
	secrets        *localSecrets
	kubeconfigPath string
	kubeContext    string
	clusterServer  string
}

func (lb *localBootstrapper) CheckPrerequisites(ctx context.Context, kubeconfig string) error {
	if kubeconfig == "" {
		kubeconfig = DefaultKubeconfigPath()
	}
	kc, err := LoadKubeconfig(kubeconfig, "")
	if err != nil {
		return fmt.Errorf(
			"could not read kubeconfig at %s\n\nPossible remediation:\n"+
				"  • Start your local Kubernetes cluster (Docker Desktop, Rancher Desktop, kind, …)\n"+
				"  • If the kubeconfig is in a non-default location, use --kubeconfig to specify the path\n"+
				"  • Verify the file exists: kubectl config view",
			kubeconfig,
		)
	}
	if err := kc.CheckAccess(ctx); err != nil {
		return fmt.Errorf(
			"cannot reach cluster %q at %s\n\nPossible remediation:\n"+
				"  • Start your local Kubernetes cluster (Docker Desktop, Rancher Desktop, kind, …)\n"+
				"  • Check the active context:  kubectl config current-context\n"+
				"  • Verify connectivity:       kubectl cluster-info\n"+
				"  • Kubeconfig in use:         %s",
			kc.ContextName, kc.ServerURL, kubeconfig,
		)
	}
	lb.kubeconfigPath = kubeconfig
	lb.kubeContext = kc.ContextName
	lb.clusterServer = kc.ServerURL
	return nil
}

func (lb *localBootstrapper) KubeconfigPath() string { return lb.kubeconfigPath }
func (lb *localBootstrapper) KubeContext() string    { return lb.kubeContext }
func (lb *localBootstrapper) ClusterServer() string  { return lb.clusterServer }

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

// Provision generates cryptographic keys needed to run Morsel.
// Safe to re-run — key generation is idempotent.
func (lb *localBootstrapper) Provision(ctx context.Context, _ map[string]string) error {
	if _, err := lb.secrets.EnsureDeploySigningKey(ctx); err != nil {
		return fmt.Errorf("generate deploy signing key: %w", err)
	}
	return nil
}
