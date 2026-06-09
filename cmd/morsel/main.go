package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ifeanyiecheruo/morsel/internal/platforms"
)

func main() {
	platformFlag := flag.String("platform", "", "platform implementation to use (local|gcp); overrides profile")
	flag.Parse()

	_, err := platforms.Create(resolvedPlatformName(*platformFlag))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Subcommand routing is added in subsequent features.
}

// resolvedPlatformName returns the flag value if set, otherwise reads from the profile.
func resolvedPlatformName(override string) string {
	if override != "" {
		return override
	}
	return loadProfilePlatform()
}

// loadProfilePlatform reads the platform name from the active profile file.
// Returns "" if no profile exists — callers default to "local".
func loadProfilePlatform() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(home, ".config", "morsel", "profile.json"))
	if err != nil {
		return ""
	}
	var p struct {
		Platform string `json:"platform"`
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return ""
	}
	return p.Platform
}
