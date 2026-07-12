// Package names is the single source of truth for the naming conventions used
// throughout the Morsel control plane.
//
// Three names appear in multiple places and must stay consistent:
//
//   - Repo slug     ("org/repo")            — the DB primary key for a repo
//   - App namespace (k8s namespace string)  — delegated to kube.AppNamespace
//   - App hostname  ("app.repo.app.domain") — public DNS name for HTTP apps
//
// All code that constructs or parses these names must go through this package.
package names

import (
	"strconv"
	"strings"

	"github.com/ifeanyiecheruo/morsel/internal/kube"
)

// RepoSlug returns the canonical repository key ("org/repo") used in the
// database and in API path parameters.
func RepoSlug(org, repo string) string { return org + "/" + repo }

// RepoOrg returns the organisation component of a repo slug ("org/repo" → "org").
// Returns an empty string if slug contains no "/".
func RepoOrg(slug string) string {
	org, _, found := strings.Cut(slug, "/")
	if !found {
		return ""
	}
	return org
}

// RepoName returns the bare repository name from a repo slug ("org/repo" → "repo").
// If slug contains no "/", it is returned unchanged.
func RepoName(slug string) string {
	_, name, found := strings.Cut(slug, "/")
	if !found {
		return slug
	}
	return name
}

// AppNamespace returns the Kubernetes namespace for an app.
func AppNamespace(repoSlug, appName string) string {
	return kube.AppNamespace(repoSlug, appName)
}

// AppHostname returns the public DNS hostname for an app.
// repoName must be the bare repository name — use RepoName to extract it from
// a slug if needed.
func AppHostname(appName, repoName, baseDomain string) string {
	return appName + "." + repoName + ".app." + baseDomain
}

// AppURL returns the HTTPS URL for an app, omitting the port when it is the
// standard HTTPS port (443).
func AppURL(appName, repoName, baseDomain string, gatewayPort int) string {
	host := AppHostname(appName, repoName, baseDomain)
	if gatewayPort == 443 {
		return "https://" + host
	}
	return "https://" + host + ":" + strconv.Itoa(gatewayPort)
}

// AppServiceAddr returns the in-cluster HTTP address for an app's Kubernetes
// Service. Morsel always names the Service "app" inside the app's namespace.
func AppServiceAddr(namespace string) string {
	return "http://app." + namespace + ".svc.cluster.local:8080"
}
