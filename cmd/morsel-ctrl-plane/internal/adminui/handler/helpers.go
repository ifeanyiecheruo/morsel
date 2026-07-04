package handler

import (
	"net/http"
)

// repoSlug reconstructs a repo slug from {org}/{repo} path values.
func repoSlug(r *http.Request) string {
	return r.PathValue("org") + "/" + r.PathValue("repo")
}

// appParams extracts (repoSlug, appName) from path values.
func appParams(r *http.Request) (slug, appName string) {
	return repoSlug(r), r.PathValue("appName")
}
