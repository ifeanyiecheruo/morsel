package container

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
)

// Cluster abstracts how container images are made available to a local
// Kubernetes cluster. Implementations exist for k3d, Docker Desktop, and
// minikube.
type Cluster interface {
	// Ensure creates the cluster if it does not exist and verifies it is
	// ready to accept workloads. For Docker Desktop and minikube, which
	// manage their own lifecycle, this is a no-op.
	Ensure(ctx context.Context) error
	// BuildAndLoad builds the container image, assigns it a content-addressed
	// tag derived from the image digest, ensures it is visible to the cluster,
	// and returns the content-addressed tag. buildArgs are forwarded to the
	// build command (e.g. --build-arg KEY=VAL). The mechanism differs per provider:
	//   - k3d: build then k3d image import
	//   - docker-desktop: build only (daemon is shared; no load step needed)
	//   - minikube: minikube image build (targets minikube's internal runtime)
	// The returned tag changes whenever the image content changes, so callers
	// can use it as the Deployment image to trigger automatic rollouts.
	BuildAndLoad(ctx context.Context, dockerfile []byte, tag, buildContext string, buildArgs ...string) (string, error)
}

const (
	// clusterName is the fixed name of the local cluster that bootstrap creates.
	clusterName = "morsel-local"
	// apiNodePort is the NodePort that the morsel-api service exposes inside
	// the cluster. Must match kube.apiNodePort.
	apiNodePort = 30080
	// APIHostPort is the host port mapped to apiNodePort, making the morsel-api
	// reachable at localhost:18080. Uses a non-privileged port so Docker Desktop
	// (wslrelay) can bind it without HTTP.SYS interference on Windows.
	APIHostPort = 18080
	// registryNodePort is the NodePort that the local registry service exposes
	// inside the cluster. Must match kube.registryKubeNodePort.
	registryNodePort = 30050
	// RegistryHostPort is the host port mapped to registryNodePort, making the
	// registry reachable at localhost:5000 for push.
	RegistryHostPort = 5000
	// HTTPHostPort is the host port mapped to container port 80 on k3s nodes.
	// Currently unused (reserved for future HTTP redirect support); Envoy Gateway
	// only binds port 443. Non-privileged to avoid HTTP.SYS interception on Windows.
	HTTPHostPort = 9080
	// HTTPSHostPort is the host port mapped to container port 443 on k3s nodes.
	// Envoy Gateway's proxy LoadBalancer service binds port 443; Klipper ServiceLB
	// exposes it as a hostPort, making apps reachable at localhost:9443 over HTTPS.
	// Non-privileged to avoid HTTP.SYS interception on Windows.
	HTTPSHostPort = 9443
)

// NewCluster returns the Cluster implementation for provider. provider must be
// one of "k3d", "docker-desktop", or "minikube". k3dSearchDirs are extra
// directories appended to FindK3d's search path (e.g. a repo's .local/bin).
func NewCluster(rt Runtime, provider string, k3dSearchDirs ...string) (Cluster, error) {
	switch provider {
	case "k3d":
		k3dPath, err := FindK3d(k3dSearchDirs...)
		if err != nil {
			return nil, err
		}
		return newK3dCluster(rt, k3dPath), nil
	case "docker-desktop":
		return newDockerDesktopCluster(rt), nil
	case "minikube":
		return newMinikubeCluster(), nil
	default:
		return nil, fmt.Errorf("unknown provider %q: must be k3d, docker-desktop, or minikube", provider)
	}
}

// ── k3dCluster ────────────────────────────────────────────────────────────────

func newK3dCluster(rt Runtime, k3dPath string) Cluster {
	return k3dCluster{rt: rt, k3dPath: k3dPath}
}

type k3dCluster struct {
	rt      Runtime
	k3dPath string
}

func (k k3dCluster) Ensure(ctx context.Context) error {
	out, _ := exec.CommandContext(ctx, k.k3dPath, "cluster", "list", "--no-headers").Output()
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), clusterName) {
			return nil // already exists
		}
	}
	fmt.Printf("\n  Creating k3d cluster %q…\n", clusterName)
	return k.create(ctx)
}

func (k k3dCluster) create(ctx context.Context) error {
	if err := k.rt.ApplyCgroupWorkaround(ctx); err != nil {
		return fmt.Errorf("cgroup workaround: %w", err)
	}

	// registries.yaml tells k3s containerd to mirror localhost:5000 pulls to the
	// in-cluster registry NodePort so pods can pull images that were pushed from
	// the host via localhost:5000.
	registriesYAML := fmt.Sprintf(`mirrors:
  "localhost:%d":
    endpoint:
      - "http://localhost:%d"
`, RegistryHostPort, registryNodePort)

	f, err := os.CreateTemp("", "morsel-k3d-registries-*.yaml")
	if err != nil {
		return fmt.Errorf("create registries config: %w", err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.WriteString(registriesYAML); err != nil {
		_ = f.Close()
		return fmt.Errorf("write registries config: %w", err)
	}
	_ = f.Close()

	args := []string{
		"cluster", "create", clusterName,
		// morsel-api NodePort
		"--port", fmt.Sprintf("%d:%d@server:0", APIHostPort, apiNodePort),
		// in-cluster registry NodePort
		"--port", fmt.Sprintf("%d:%d@server:0", RegistryHostPort, registryNodePort),
		// Port 80/443 via non-privileged host ports to avoid HTTP.SYS interception
		// and WSL relay conflicts on Windows. Envoy Gateway's proxy binds port 443;
		// port 80 is reserved for future HTTP redirect support.
		"--port", fmt.Sprintf("%d:80@server:0", HTTPHostPort),
		"--port", fmt.Sprintf("%d:443@server:0", HTTPSHostPort),
		// Disable built-in Traefik; Envoy Gateway is installed during bootstrap.
		"--k3s-arg", "--disable=traefik@server:0",
		// KubeletInUserNamespace: without this, kubelet attempts to write kernel
		// parameters (e.g. /proc/sys/kernel/dmesg_restrict, net.ipv4 sysctls) during
		// node initialization. Those writes are denied inside a rootless Podman
		// container running in a user namespace, causing kubelet to crash before the
		// node ever reaches Ready. The feature gate makes kubelet skip those sysctl
		// writes when it detects it is running inside a user namespace.
		"--k3s-arg", "--kubelet-arg=feature-gates=KubeletInUserNamespace=true@server:0",
		// registry mirror for containerd inside k3s nodes
		"--registry-config", f.Name(),
	}

	cmd, err := k.exec(ctx, args...)
	if err != nil {
		return err
	}
	if err := ctxlog.RunCmd(ctx, cmd); err != nil {
		return err
	}

	if err := k.rt.ApplyDNSWorkaround(ctx, "k3d-"+clusterName+"-server-0"); err != nil {
		return err
	}

	// Merge the new cluster's kubeconfig into the default kubeconfig so that
	// subsequent kubectl / kube.Client calls see the cluster without requiring
	// the user to run kubeconfig merge manually.
	merge, err := k.exec(ctx, "kubeconfig", "merge", clusterName, "--kubeconfig-merge-default")
	if err != nil {
		return fmt.Errorf("merge kubeconfig: %w", err)
	}
	if out, err := merge.CombinedOutput(); err != nil {
		return fmt.Errorf("merge kubeconfig: %w: %s", err, out)
	}
	return nil
}

func (k k3dCluster) exec(ctx context.Context, args ...string) (*exec.Cmd, error) {
	host, err := k.rt.Host()
	if err != nil {
		return nil, fmt.Errorf("get runtime host: %w", err)
	}
	if ctxlog.GetMode(ctx).VerboseSubprocesses() {
		args = append([]string{"--verbose"}, args...)
	}
	env := os.Environ()
	if host != "" {
		env = append(env, "DOCKER_HOST="+host)
	}
	cmd := exec.CommandContext(ctx, k.k3dPath, args...)
	cmd.Env = env
	return cmd, nil
}

func (k k3dCluster) BuildAndLoad(ctx context.Context, dockerfile []byte, tag, buildContext string, buildArgs ...string) (string, error) {
	if err := k.rt.Build(ctx, dockerfile, tag, buildContext, buildArgs...); err != nil {
		return "", fmt.Errorf("build: %w", err)
	}
	ct, err := contentTag(ctx, k.rt, tag)
	if err != nil {
		return "", err
	}
	load, err := k.exec(ctx, "image", "import", ct, "--cluster", clusterName)
	if err != nil {
		return "", fmt.Errorf("k3d image import: %w", err)
	}
	if err := ctxlog.RunCmd(ctx, load); err != nil {
		return "", fmt.Errorf("k3d image import: %w", err)
	}
	return ct, nil
}

// contentTag derives a content-addressed tag from tag by inspecting the built
// image's digest. Example: "localhost/morsel-api:local" → "localhost/morsel-api:sha-abc123def456".
// It also creates the new alias in the local runtime so k3d can import it.
func contentTag(ctx context.Context, rt Runtime, tag string) (string, error) {
	id, err := rt.ImageID(ctx, tag)
	if err != nil {
		return "", fmt.Errorf("image id: %w", err)
	}
	if len(id) > 12 {
		id = id[:12]
	}
	repo := strings.SplitN(tag, ":", 2)[0]
	dst := repo + ":sha-" + id
	if err := rt.Tag(ctx, tag, dst); err != nil {
		return "", fmt.Errorf("retag: %w", err)
	}
	return dst, nil
}

// ── dockerDesktopCluster ─────────────────────────────────────────────────────

type dockerDesktopCluster struct{ rt Runtime }

func newDockerDesktopCluster(rt Runtime) Cluster { return dockerDesktopCluster{rt: rt} }

func (dockerDesktopCluster) Ensure(_ context.Context) error { return nil }

func (d dockerDesktopCluster) BuildAndLoad(ctx context.Context, dockerfile []byte, tag, buildContext string, buildArgs ...string) (string, error) {
	if err := d.rt.Build(ctx, dockerfile, tag, buildContext, buildArgs...); err != nil {
		return "", fmt.Errorf("build: %w", err)
	}
	return contentTag(ctx, d.rt, tag)
}

// ── minikubeCluster ──────────────────────────────────────────────────────────

type minikubeCluster struct{}

func newMinikubeCluster() Cluster { return minikubeCluster{} }

func (minikubeCluster) Ensure(_ context.Context) error { return nil }

func (minikubeCluster) BuildAndLoad(ctx context.Context, dockerfile []byte, tag, buildContext string, buildArgs ...string) (string, error) {
	args := []string{"image", "build", "-t", tag, "-f", "-"}
	args = append(args, buildArgs...)
	args = append(args, buildContext)
	// minikube builds inside its own container runtime; host-side inspect is not
	// available, so the input tag is returned unchanged.
	return tag, execBuild(ctx, "minikube", dockerfile, args...)
}
