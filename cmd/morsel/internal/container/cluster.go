package container

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Cluster abstracts how container images are made available to a local
// Kubernetes cluster. Implementations exist for kind, Docker Desktop, and
// minikube.
type Cluster interface {
	// Ensure creates the cluster if it does not exist and verifies it is
	// ready to accept workloads. For Docker Desktop and minikube, which
	// manage their own lifecycle, this is a no-op.
	Ensure(ctx context.Context) error
	// BuildAndLoad builds the container image and ensures it is visible to
	// the cluster. buildArgs are forwarded to the build command
	// (e.g. --build-arg KEY=VAL). The mechanism differs per provider:
	//   - kind: build → save docker-archive tar → kind load image-archive
	//   - docker-desktop: build only (daemon is shared; no load step needed)
	//   - minikube: minikube image build (targets minikube's internal runtime)
	BuildAndLoad(ctx context.Context, dockerfile []byte, tag, buildContext string, buildArgs ...string) error
}

const (
	// kindClusterName is the fixed name of the kind cluster that bootstrap creates.
	kindClusterName = "morsel-local"
	// kindNodePort is the NodePort that the morsel-api service exposes inside
	// the kind cluster. Must match kube.apiNodePort.
	kindNodePort = 30080
	// KindHostPort is the host port that kind's extraPortMappings maps to
	// kindNodePort, making the morsel-api reachable at localhost:8080.
	// Exported so the local bootstrapper can construct the external API URL.
	KindHostPort = 8080
	// registryNodePort is the NodePort that the local registry service exposes
	// inside the kind cluster. Must match kube.registryKubeNodePort.
	registryNodePort = 30050
	// RegistryHostPort is the host port mapped to registryNodePort by kind's
	// extraPortMappings, making the registry reachable at localhost:5000 for push.
	RegistryHostPort = 5000
)

// NewCluster returns the Cluster implementation for provider. provider must be
// one of "kind", "docker-desktop", or "minikube". kindSearchDirs are extra
// directories appended to FindKind's search path (e.g. a repo's .local/bin).
func NewCluster(rt Runtime, provider string, kindSearchDirs ...string) (Cluster, error) {
	switch provider {
	case "kind":
		kindPath, err := FindKind(kindSearchDirs...)
		if err != nil {
			return nil, err
		}
		return newKindCluster(rt, kindPath), nil
	case "docker-desktop":
		return newDockerDesktopCluster(rt), nil
	case "minikube":
		return newMinikubeCluster(), nil
	default:
		return nil, fmt.Errorf("unknown provider %q: must be kind, docker-desktop, or minikube", provider)
	}
}

func newKindCluster(rt Runtime, kindPath string) Cluster {
	return kindCluster{rt: rt, kindPath: kindPath}
}

func newDockerDesktopCluster(rt Runtime) Cluster { return dockerDesktopCluster{rt: rt} }

func newMinikubeCluster() Cluster { return minikubeCluster{} }

// kindCluster

type kindCluster struct {
	rt       Runtime
	kindPath string
}

// Ensure creates the kind cluster with the configured port mapping if it does
// not exist. If the cluster already exists, it verifies the port mapping is
// present and returns an actionable error if it is missing.
func (k kindCluster) Ensure(ctx context.Context) error {
	out, _ := exec.CommandContext(ctx, k.kindPath, "get", "clusters").Output()
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) == kindClusterName {
			return k.checkPortMapping()
		}
	}
	fmt.Printf("\n  Creating kind cluster %q…\n", kindClusterName)
	return k.create(ctx)
}

// checkPortMapping returns an error when the cluster exists but lacks the
// required host-port mapping, giving the operator a clear remediation command.
func (k kindCluster) checkPortMapping() error {
	nodeName := kindClusterName + "-control-plane"
	out, err := exec.Command(k.rt.Name(), "port", nodeName).Output()
	if err != nil {
		return nil // can't inspect — let connectivity check surface the issue
	}
	outStr := string(out)
	if strings.Contains(outStr, fmt.Sprintf(":%d", KindHostPort)) &&
		strings.Contains(outStr, fmt.Sprintf(":%d", RegistryHostPort)) {
		return nil
	}
	return fmt.Errorf(
		"kind cluster %q exists but is missing required port mappings (localhost:%d and localhost:%d)\n\n"+
			"Delete the cluster and re-run bootstrap to recreate it with the correct port mappings:\n"+
			"  kind delete cluster --name %s\n"+
			"  morsel service bootstrap --platform local",
		kindClusterName, KindHostPort, RegistryHostPort, kindClusterName,
	)
}

// create creates the kind cluster with the morsel-api port mapping.
func (k kindCluster) create(ctx context.Context) error {
	// containerdConfigPatches: tell the kind node's containerd to redirect
	// localhost:5000 pulls to localhost:30050 (the registry NodePort) so that
	// pods can pull images pushed from the host via the same localhost:5000 tag.
	config := fmt.Sprintf(`kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:%d"]
  endpoint = ["http://localhost:%d"]
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: %d
    hostPort: %d
    protocol: TCP
  - containerPort: %d
    hostPort: %d
    protocol: TCP
`, RegistryHostPort, registryNodePort, kindNodePort, KindHostPort, registryNodePort, RegistryHostPort)

	f, err := os.CreateTemp("", "morsel-kind-config-*.yaml")
	if err != nil {
		return fmt.Errorf("create kind config: %w", err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.WriteString(config); err != nil {
		_ = f.Close()
		return fmt.Errorf("write kind config: %w", err)
	}
	_ = f.Close()

	args := []string{"create", "cluster", "--name", kindClusterName, "--config", f.Name()}

	cmd := exec.CommandContext(ctx, k.kindPath, args...)
	if k.rt.Name() == "podman" {
		cmd.Env = append(os.Environ(), "KIND_EXPERIMENTAL_PROVIDER=podman")
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (k kindCluster) BuildAndLoad(ctx context.Context, dockerfile []byte, tag, buildContext string, buildArgs ...string) error {
	if err := k.rt.Build(ctx, dockerfile, tag, buildContext, buildArgs...); err != nil {
		return fmt.Errorf("build: %w", err)
	}

	archive, err := os.CreateTemp("", "morsel-image-*.tar")
	if err != nil {
		return fmt.Errorf("create image archive: %w", err)
	}
	archivePath := archive.Name()
	_ = archive.Close()
	defer func() { _ = os.Remove(archivePath) }()

	if err := k.rt.SaveArchive(ctx, tag, archivePath); err != nil {
		return fmt.Errorf("save image archive: %w", err)
	}

	load := exec.CommandContext(ctx, k.kindPath, "load", "image-archive", archivePath, "--name", kindClusterName)
	load.Stdout = os.Stdout
	load.Stderr = os.Stderr
	if err := load.Run(); err != nil {
		return fmt.Errorf("kind load image-archive: %w", err)
	}
	return nil
}

// dockerDesktopCluster

type dockerDesktopCluster struct{ rt Runtime }

func (dockerDesktopCluster) Ensure(_ context.Context) error { return nil }

func (d dockerDesktopCluster) BuildAndLoad(ctx context.Context, dockerfile []byte, tag, buildContext string, buildArgs ...string) error {
	return d.rt.Build(ctx, dockerfile, tag, buildContext, buildArgs...)
}

// minikubeCluster

type minikubeCluster struct{}

func (minikubeCluster) Ensure(_ context.Context) error { return nil }

func (minikubeCluster) BuildAndLoad(ctx context.Context, dockerfile []byte, tag, buildContext string, buildArgs ...string) error {
	args := []string{"image", "build", "-t", tag, "-f", "-"}
	args = append(args, buildArgs...)
	args = append(args, buildContext)
	return execBuild(ctx, "minikube", dockerfile, args...)
}
