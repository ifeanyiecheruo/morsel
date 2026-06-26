package container

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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
