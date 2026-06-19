package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Profile holds the persisted authentication and connection state for one named profile.
// The JSON field names match the profile file schema in docs/specs/components/cli.md.
type Profile struct {
	Platform              string `json:"platform"`
	APIURL                string `json:"api_url"`
	AccessToken           string `json:"access_token"`
	AccessTokenExpiresAt  string `json:"access_token_expires_at"`
	RefreshToken          string `json:"refresh_token"`
	RefreshTokenExpiresAt string `json:"refresh_token_expires_at"`

	// LocalPlatform-only fields
	Kubeconfig    string `json:"kubeconfig,omitempty"`
	Kubecontext   string `json:"kubecontext,omitempty"`
	ClusterServer string `json:"cluster_server,omitempty"`

	// GCPPlatform-only fields
	Project string `json:"project,omitempty"`
	Region  string `json:"region,omitempty"`
}

func readProfile(name string) (*Profile, error) {
	path, err := profilePath(name)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read profile %q: %w", name, err)
	}
	var prof Profile
	if err := json.Unmarshal(data, &prof); err != nil {
		return nil, fmt.Errorf("parse profile %q: %w", name, err)
	}
	return &prof, nil
}

func writeProfile(name string, prof *Profile) error {
	path, err := profilePath(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create profile directory: %w", err)
	}
	data, err := json.MarshalIndent(prof, "", "  ")
	if err != nil {
		return fmt.Errorf("encode profile: %w", err)
	}
	return os.WriteFile(path, data, 0600)
}

func deleteProfile(name string) error {
	path, err := profilePath(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete profile %q: %w", name, err)
	}
	return nil
}

func profilePath(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".config", "morsel", name+".profile.json"), nil
}
