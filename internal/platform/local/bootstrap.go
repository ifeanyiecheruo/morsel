package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ifeanyiecheruo/morsel/internal/container"
	"github.com/ifeanyiecheruo/morsel/internal/kube"
	"github.com/ifeanyiecheruo/morsel/internal/platform"
)

const (
	// apiImage is the local tag used for the morsel-api image built during bootstrap.
	// The localhost/ prefix is required for Podman, which stores all locally built
	// images under localhost/ and does not resolve the short form via the Docker API.
	apiImage = "localhost/morsel-api:local"
	// apiDBPath is the path inside the morsel-api container where the SQLite database
	// is stored. The parent directory (/data) is mounted from a PVC.
	apiDBPath = "/data/morsel.db"
)

type localBootstrapper struct {
	secrets        *localSecrets
	kubeconfigPath string
	kubeContext    string
	clusterServer  string
	cluster        container.Cluster
	repoRoot       string
}

// CheckPrerequisites ensures the target cluster exists and is reachable.
// answers must include k8s_provider. For "kind", the morsel-local cluster is
// created (with extraPortMappings for localhost:8080) if it does not yet exist.
func (lb *localBootstrapper) CheckPrerequisites(ctx context.Context, kubeconfig string, answers map[string]string) error {
	provider := answers["k8s_provider"]
	if provider == "" {
		provider = "kind"
	}

	rt, err := container.CreateRuntime()
	if err != nil {
		return err
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("find repo root: %w", err)
	}
	lb.repoRoot = repoRoot

	cluster, err := container.NewCluster(rt, provider, filepath.Join(repoRoot, ".local", "bin"))
	if err != nil {
		return err
	}
	lb.cluster = cluster

	if err := cluster.Ensure(ctx); err != nil {
		return err
	}

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

// APIURL returns the host-accessible URL for the morsel-api after bootstrap.
// For the local platform this is the kind extraPortMappings host port (8080).
func (lb *localBootstrapper) APIURL() string {
	return fmt.Sprintf("http://localhost:%d", container.KindHostPort)
}

func (lb *localBootstrapper) Prompts() []platform.Prompt {
	return []platform.Prompt{
		{
			Key:     "k8s_namespace",
			Label:   "Base Kubernetes namespace",
			Default: "morsel",
		},
		{
			Key:      "k8s_provider",
			Label:    "Kubernetes provider",
			Default:  "kind",
			Required: true,
			Choices:  []string{"kind", "docker-desktop", "minikube"},
		},
	}
}

func (lb *localBootstrapper) Plan(answers map[string]string) platform.Plan {
	ns := answers["k8s_namespace"]
	if ns == "" {
		ns = "morsel"
	}
	return platform.Plan{
		Summary: "A Morsel control plane will be provisioned inside your local Kubernetes cluster. No cloud account or billing is required.",
		Resources: []platform.Resource{
			{Name: "Local container registry", Description: "registry:2 in namespace " + ns},
			{Name: "Morsel API", Description: "Control plane service in namespace " + ns + " on localhost:8080"},
		},
	}
}

// Provision generates cryptographic keys and provisions Kubernetes resources.
// Safe to re-run — all operations are idempotent.
func (lb *localBootstrapper) Provision(ctx context.Context, answers map[string]string, dockerfile []byte) error {
	if _, err := lb.secrets.EnsureDeploySigningKey(ctx); err != nil {
		return fmt.Errorf("generate deploy signing key: %w", err)
	}

	// kubeconfigPath and cluster are set by CheckPrerequisites; skip K8s
	// provisioning in contexts where that step was not run (e.g. unit tests).
	if lb.kubeconfigPath == "" {
		return nil
	}

	if len(dockerfile) == 0 {
		return fmt.Errorf("expected Dockerfile not embedded — rebuild the morsel CLI from source")
	}

	ns := answers["k8s_namespace"]
	if ns == "" {
		ns = "morsel"
	}

	fmt.Println("Building morsel-api image…")
	if err := lb.cluster.BuildAndLoad(ctx, dockerfile, apiImage, lb.repoRoot,
		"--build-arg", fmt.Sprintf("MORSEL_UID=%d", kube.MorselUID),
		"--build-arg", fmt.Sprintf("MORSEL_GID=%d", kube.MorselGID),
	); err != nil {
		return fmt.Errorf("build morsel-api: %w", err)
	}

	kubeClient, err := kube.New(lb.kubeconfigPath)
	if err != nil {
		return fmt.Errorf("build kubernetes client: %w", err)
	}

	if err := kubeClient.EnsureRegistry(ctx, ns); err != nil {
		return fmt.Errorf("provision registry: %w", err)
	}

	if err := kubeClient.EnsureAPI(ctx, ns, apiImage, apiDBPath); err != nil {
		return fmt.Errorf("provision morsel-api: %w", err)
	}

	fmt.Println("Waiting for morsel-api to become healthy…")
	if err := kubeClient.WaitForAPIReady(ctx, ns, 3*time.Minute); err != nil {
		return fmt.Errorf("morsel-api did not become healthy: %w", err)
	}

	return nil
}

// findRepoRoot walks up from the current working directory until it finds a
// go.mod, which marks the root of the morsel source tree.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf(
				"go.mod not found — run bootstrap from inside the morsel source tree",
			)
		}
		dir = parent
	}
}
