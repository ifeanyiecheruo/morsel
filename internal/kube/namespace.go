package kube

import "strings"

// AppNamespace derives the Kubernetes namespace for an app from its repo slug
// and app name. The repo slug is the canonical "org/repo" form as stored in the
// database. The formula is:
//
//	slugify(repoSlug) + "--" + slugify(appName)   (named app)
//	slugify(repoSlug)                              (appName == "")
//
// where slugify replaces "/" and "_" with "-".
func AppNamespace(repoSlug, appName string) string {
	r := strings.NewReplacer("/", "-", "_", "-")
	base := r.Replace(repoSlug)
	if appName == "" {
		return base
	}
	return base + "--" + r.Replace(appName)
}
