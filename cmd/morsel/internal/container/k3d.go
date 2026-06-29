package container

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// FindK3d resolves the path to the k3d CLI. It checks PATH, any
// extraSearchDirs (e.g. the repo's .local/bin from make install-tools),
// common Go install locations, and $GOBIN/$GOPATH/bin.
func FindK3d(extraSearchDirs ...string) (string, error) {
	if path, err := exec.LookPath("k3d"); err == nil {
		return path, nil
	}
	for _, dir := range extraSearchDirs {
		if dir == "" {
			continue
		}
		if path, ok := StatExecutable(filepath.Join(dir, "k3d")); ok {
			return path, nil
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		if path, ok := StatExecutable(filepath.Join(home, "go", "bin", "k3d")); ok {
			return path, nil
		}
	}
	if gobin := os.Getenv("GOBIN"); gobin != "" {
		if path, ok := StatExecutable(filepath.Join(gobin, "k3d")); ok {
			return path, nil
		}
	}
	if gopath := os.Getenv("GOPATH"); gopath != "" {
		if path, ok := StatExecutable(filepath.Join(gopath, "bin", "k3d")); ok {
			return path, nil
		}
	}
	return "", fmt.Errorf(
		"k3d CLI not found in PATH, .local/bin, or ~/go/bin\n\n" +
			"Run 'make install-tools' to install it, or see https://k3d.io/stable/#installation",
	)
}

// StatExecutable checks whether path or path+".exe" exists and is a regular
// file, returning the resolved path. The .exe variant handles Windows without
// requiring build tags.
func StatExecutable(path string) (string, bool) {
	for _, p := range []string{path, path + ".exe"} {
		if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
			return p, true
		}
	}
	return "", false
}
