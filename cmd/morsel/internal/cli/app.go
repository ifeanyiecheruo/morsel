package cli

import (
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func (c *cli) appCmd() *cobra.Command {
	var org, repo string

	cmd := &cobra.Command{
		Use:   "app",
		Short: "Create, list, and manage apps",
	}
	cmd.PersistentFlags().StringVar(&org, "org", "", "organisation or user owning the repo (defaults to git remote origin)")
	cmd.PersistentFlags().StringVar(&repo, "repo", "", "repository name (defaults to git remote origin)")

	cmd.AddCommand(c.appDeployCmd())
	cmd.AddCommand(c.appSyncCmd(&org, &repo))
	cmd.AddCommand(c.appListCmd(&org, &repo))
	cmd.AddCommand(c.appGetCmd(&org, &repo))
	cmd.AddCommand(c.appStatusCmd(&org, &repo))
	cmd.AddCommand(c.appDeleteCmd(&org, &repo))
	cmd.AddCommand(c.appHistoryCmd(&org, &repo))
	return cmd
}

// resolveOrgRepo returns the provided org and repo, falling back to the git
// remote origin if either is empty.
func resolveOrgRepo(org, repo string) (string, string, error) {
	if org != "" && repo != "" {
		return org, repo, nil
	}
	detectedOrg, detectedRepo, err := gitRemoteOrgRepo()
	if err != nil {
		return "", "", err
	}
	if org == "" {
		org = detectedOrg
	}
	if repo == "" {
		repo = detectedRepo
	}
	return org, repo, nil
}

// gitRemoteOrgRepo extracts org and repo from the git remote named "origin".
func gitRemoteOrgRepo() (org, repo string, err error) {
	out, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err != nil {
		return "", "", fmt.Errorf("cannot detect repo from git remote (pass --org and --repo explicitly): %w", err)
	}
	return parseRemoteURL(strings.TrimSpace(string(out)))
}

// gitRootDir returns the absolute path of the current git repository root.
func gitRootDir() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("not inside a git repository (git rev-parse failed): %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// gitCurrentSHA returns the full commit SHA of HEAD, or empty string on error.
func gitCurrentSHA() string {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// localOrgRepo derives ("localhost", "<slugified-dirname>") from the git root.
func localOrgRepo() (org, repo string, err error) {
	root, err := gitRootDir()
	if err != nil {
		return "", "", err
	}
	dirname := filepath.Base(root)
	var b strings.Builder
	for _, r := range strings.ToLower(dirname) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	return "localhost", strings.Trim(b.String(), "-"), nil
}

// parseRemoteURL handles SSH (git@host:org/repo.git) and HTTPS remote URLs.
func parseRemoteURL(rawURL string) (org, repo string, err error) {
	if at := strings.Index(rawURL, "@"); at >= 0 {
		colon := strings.Index(rawURL[at:], ":") + at
		if colon <= at {
			return "", "", fmt.Errorf("unrecognised SSH remote URL: %s", rawURL)
		}
		path := strings.TrimSuffix(rawURL[colon+1:], ".git")
		parts := strings.SplitN(path, "/", 2)
		if len(parts) != 2 {
			return "", "", fmt.Errorf("unrecognised SSH remote path in %s", rawURL)
		}
		return parts[0], parts[1], nil
	}
	parsed, parseErr := url.Parse(rawURL)
	if parseErr != nil {
		return "", "", fmt.Errorf("parse git remote URL %s: %w", rawURL, parseErr)
	}
	path := strings.TrimSuffix(strings.TrimPrefix(parsed.Path, "/"), ".git")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("unrecognised HTTPS remote path in %s", rawURL)
	}
	return parts[0], parts[1], nil
}
