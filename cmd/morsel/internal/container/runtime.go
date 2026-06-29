package container

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Runtime abstracts the container runtime (docker or podman).
type Runtime interface {
	// Name returns the runtime binary name ("docker" or "podman").
	Name() string
	// Build builds an image from dockerfile bytes. buildArgs are appended
	// between the -f flag and the build context (e.g. --build-arg KEY=VAL).
	Build(ctx context.Context, dockerfile []byte, tag, buildContext string, buildArgs ...string) error
	// Push pushes a built image to the registry inferred from tag.
	Push(ctx context.Context, tag string) error
	// SaveArchive saves the image in docker-archive format to destPath.
	// kind requires docker-archive format regardless of the underlying runtime.
	SaveArchive(ctx context.Context, tag, destPath string) error

	// Host returns the DOCKER_HOST URI that tools communicating with this
	// runtime via the Docker API (e.g. k3d) must set. Returns "" when the
	// default socket is correct and no override is needed.
	Host() (string, error)
	// Exec runs cmd with args inside containerName. env is a slice of "KEY=VAL"
	// strings injected into the container environment.
	Exec(ctx context.Context, containerName, cmd string, args, env []string) error
	// ApplyDNSWorkaround fixes DNS resolution inside containerName if the
	// runtime requires it. Returns nil without doing anything when no fix is needed.
	ApplyDNSWorkaround(ctx context.Context, containerName string) error
	// ApplyCgroupWorkaround ensures the host cgroup configuration supports
	// running a Kubernetes node. Returns nil without doing anything when no fix
	// is needed.
	ApplyCgroupWorkaround(ctx context.Context) error

	// ImageID returns the sha256 hex digest of the image identified by tag,
	// without the "sha256:" prefix.
	ImageID(ctx context.Context, tag string) (string, error)
	// Tag creates a new tag alias dst pointing at the same image as src.
	Tag(ctx context.Context, src, dst string) error
}

// CreateRuntime detects and returns the available container runtime.
// It returns an error if neither docker nor podman can be reached.
func CreateRuntime() (Runtime, error) {
	if exec.Command("docker", "info").Run() == nil {
		return dockerRuntime{}, nil
	}
	if exec.Command("podman", "info").Run() == nil {
		return podmanRuntime{}, nil
	}
	return nil, fmt.Errorf("no container runtime found: install Docker Desktop or Podman")
}

// dockerRuntime

type dockerRuntime struct{}

func (dockerRuntime) Name() string { return "docker" }

func (dockerRuntime) Build(ctx context.Context, dockerfile []byte, tag, buildContext string, buildArgs ...string) error {
	args := append([]string{"build", "-t", tag, "-f", "-"}, buildArgs...)
	args = append(args, buildContext)
	return execBuild(ctx, "docker", dockerfile, args...)
}

func (dockerRuntime) Push(ctx context.Context, tag string) error {
	return runCmd(exec.CommandContext(ctx, "docker", "push", tag))
}

func (dockerRuntime) SaveArchive(ctx context.Context, tag, destPath string) error {
	return runCmd(exec.CommandContext(ctx, "docker", "save", "-o", destPath, tag))
}

func (dockerRuntime) Host() (string, error) { return "", nil }

func (dockerRuntime) Exec(ctx context.Context, containerName, cmd string, args, env []string) error {
	execArgs := []string{"exec"}
	for _, e := range env {
		execArgs = append(execArgs, "--env", e)
	}
	execArgs = append(execArgs, containerName, cmd)
	execArgs = append(execArgs, args...)
	return runCmd(exec.CommandContext(ctx, "docker", execArgs...))
}

func (dockerRuntime) ApplyDNSWorkaround(_ context.Context, _ string) error { return nil }
func (dockerRuntime) ApplyCgroupWorkaround(_ context.Context) error        { return nil }

func (dockerRuntime) ImageID(ctx context.Context, tag string) (string, error) {
	return inspectImageID(ctx, "docker", tag)
}
func (dockerRuntime) Tag(ctx context.Context, src, dst string) error {
	return runCmd(exec.CommandContext(ctx, "docker", "tag", src, dst))
}

// podmanRuntime

type podmanRuntime struct{}

func (podmanRuntime) Name() string { return "podman" }

func (podmanRuntime) Build(ctx context.Context, dockerfile []byte, tag, buildContext string, buildArgs ...string) error {
	args := append([]string{"build", "-t", tag, "-f", "-"}, buildArgs...)
	args = append(args, buildContext)
	return execBuild(ctx, "podman", dockerfile, args...)
}

func (podmanRuntime) Push(ctx context.Context, tag string) error {
	args := []string{"push"}
	if isLocalhostTag(tag) {
		args = append(args, "--tls-verify=false")
	}
	args = append(args, tag)
	return runCmd(exec.CommandContext(ctx, "podman", args...))
}

func (podmanRuntime) SaveArchive(ctx context.Context, tag, destPath string) error {
	// kind requires docker-archive format; podman defaults to OCI archive
	return runCmd(exec.CommandContext(ctx, "podman", "save", "--format", "docker-archive", "-o", destPath, tag))
}

func (podmanRuntime) Host() (string, error) {
	if runtime.GOOS == "windows" {
		out, err := exec.Command("podman", "machine", "inspect",
			"--format", "{{.ConnectionInfo.PodmanPipe.Path}}").Output()
		if err != nil {
			return "", fmt.Errorf("query podman machine pipe: %w", err)
		}
		pipePath := strings.TrimSpace(string(out))
		if pipePath == "" {
			return "", fmt.Errorf("podman machine reported empty pipe path")
		}
		// \\.\pipe\foo  →  npipe:////./pipe/foo
		normalized := strings.TrimLeft(strings.ReplaceAll(pipePath, `\`, "/"), "/")
		return "npipe:////" + normalized, nil
	}
	out, err := exec.Command("podman", "info", "--format", "{{.Host.RemoteSocket.Path}}").Output()
	if err != nil {
		return "", fmt.Errorf("query podman socket: %w", err)
	}
	p := strings.TrimSpace(string(out))
	if p == "" {
		return "", fmt.Errorf("podman reported empty socket path")
	}
	return "unix://" + p, nil
}

// Exec tunnels into the Podman machine via `podman machine ssh` because on
// Windows the containers run inside the WSL VM and are not directly reachable
// from a Windows process. The inner command is built as a single shell string
// with each token single-quoted.
func (podmanRuntime) Exec(ctx context.Context, containerName, cmd string, args, env []string) error {
	parts := []string{"podman", "exec"}
	for _, e := range env {
		parts = append(parts, "--env", shellQuote(e))
	}
	parts = append(parts, shellQuote(containerName), shellQuote(cmd))
	for _, a := range args {
		parts = append(parts, shellQuote(a))
	}
	return runCmd(exec.CommandContext(ctx, "podman", "machine", "ssh", strings.Join(parts, " ")))
}

func (p podmanRuntime) ApplyCgroupWorkaround(ctx context.Context) error {
	// ApplyCgroupWorkaround writes the systemd user-slice delegate drop-in and
	// reloads the daemon so the cpuset controller is propagated through the cgroup
	// hierarchy. Without this, kubelet's CPU manager cannot write to cpuset.cpus
	// inside pod cgroup subtrees and the node stays NotReady.

	const shellCmd = `sudo mkdir -p /etc/systemd/system/user@.service.d && ` +
		`printf '[Service]\nDelegate=memory pids cpu io cpuset\n' | ` +
		`sudo tee /etc/systemd/system/user@.service.d/delegate.conf > /dev/null && ` +
		`sudo systemctl daemon-reload`
	return runCmd(exec.CommandContext(ctx, "podman", "machine", "ssh", shellCmd))
}

func (p podmanRuntime) ApplyDNSWorkaround(ctx context.Context, containerName string) error {
	// On Podman, aardvark-dns doesn't forward external DNS queries reliably from
	// inside k3d containers on Windows/WSL. After cluster creation, overwrite
	// /etc/resolv.conf in each k3s node to use 1.1.1.1 directly so containerd
	// can pull images. The volume-mount approach conflicts with k3d's own DNS fix,
	// so we patch the file after k3d has finished its startup sequence.
	return p.Exec(ctx, containerName, "sh", []string{"-c", "echo nameserver 1.1.1.1 > /etc/resolv.conf"}, nil)
}

func (podmanRuntime) ImageID(ctx context.Context, tag string) (string, error) {
	return inspectImageID(ctx, "podman", tag)
}
func (podmanRuntime) Tag(ctx context.Context, src, dst string) error {
	return runCmd(exec.CommandContext(ctx, "podman", "tag", src, dst))
}

// inspectImageID returns the sha256 hex digest of tag using the given runtime binary.
// Both docker and podman return the digest with a "sha256:" prefix which is stripped.
func inspectImageID(ctx context.Context, binary, tag string) (string, error) {
	out, err := exec.CommandContext(ctx, binary, "image", "inspect", "--format", "{{.Id}}", tag).Output()
	if err != nil {
		return "", fmt.Errorf("%s image inspect: %w", binary, err)
	}
	return strings.TrimPrefix(strings.TrimSpace(string(out)), "sha256:"), nil
}

// shellQuote wraps s in single quotes for safe embedding in a POSIX shell command.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// isLocalhostTag reports whether tag targets a localhost or loopback registry,
// which requires --tls-verify=false for Podman.
func isLocalhostTag(tag string) bool {
	return strings.HasPrefix(tag, "localhost:") ||
		strings.HasPrefix(tag, "127.") ||
		strings.HasPrefix(tag, "[::1]:")
}

// execBuild writes dockerfile to a temp file, substitutes the temp path for
// any "-" placeholder in args (the -f - convention), then runs the command.
// A temp file is used instead of stdin because Docker Desktop's WSL2 backend
// does not reliably receive stdin from a Windows-hosted Go process.
func execBuild(ctx context.Context, name string, dockerfile []byte, args ...string) error {
	f, err := os.CreateTemp("", "morsel-Dockerfile-*")
	if err != nil {
		return fmt.Errorf("create temp dockerfile: %w", err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if err := func() error {
		defer func() { _ = f.Close() }()
		if _, err := f.Write(dockerfile); err != nil {
			return fmt.Errorf("write dockerfile: %w", err)
		}
		return nil
	}(); err != nil {
		return err
	}
	final := make([]string, len(args))
	copy(final, args)
	for i, a := range final {
		if a == "-" {
			final[i] = f.Name()
			break
		}
	}
	return runCmd(exec.CommandContext(ctx, name, final...))
}

// runCmd routes stdout and stderr to the terminal and runs the command.
func runCmd(cmd *exec.Cmd) error {
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
