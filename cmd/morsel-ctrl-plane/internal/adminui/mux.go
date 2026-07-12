// Package adminui assembles the HTTP handler for the Morsel admin UI service.
// The admin UI is a standalone service deployed via 'morsel-ctrl-plane run admin-ui'.
// All data access goes through the Morsel REST API; no internal state is accessed directly.
package adminui

import (
	"context"
	"net/http"

	uihandler "github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/adminui/handler"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/middleware"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
	"github.com/ifeanyiecheruo/morsel/internal/health"
)

// NewMux constructs the root HTTP handler for the Morsel admin UI.
// apiURL is the base URL of the Morsel REST API (e.g. "http://localhost:8080").
// sessionKey is used to sign session cookies; must be at least 32 bytes.
// githubClientID and githubClientSecret are the GitHub OAuth App credentials.
func NewMux(ctx context.Context, apiURL string, httpClient *http.Client, sessionKey []byte, githubClientID, githubClientSecret string, receiver *health.Receiver) http.Handler {
	h := uihandler.New(apiURL, httpClient, sessionKey, githubClientID, githubClientSecret)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", receiver.LivezHandler)
	mux.HandleFunc("GET /readyz", receiver.ReadyzHandler)

	// Unauthenticated routes.
	mux.HandleFunc("GET /login", h.ServeLogin)
	mux.HandleFunc("GET /oidc/callback", h.HandleGitHubCallback)
	mux.HandleFunc("POST /logout", h.HandleLogout)

	// Session-protected routes.
	protected := h.RequireSession
	mux.Handle("GET /apps", protected(http.HandlerFunc(h.ServeApps)))
	mux.Handle("GET /apps/{org}/{repo}/{appName}/delete", protected(http.HandlerFunc(h.ServeAppDeleteConfirm)))
	mux.Handle("POST /apps/{org}/{repo}/{appName}/delete", protected(http.HandlerFunc(h.HandleAppDelete)))
	mux.Handle("POST /apps/{org}/{repo}/{appName}/hibernate", protected(http.HandlerFunc(h.HandleAppHibernate)))
	mux.Handle("POST /apps/{org}/{repo}/{appName}/wake", protected(http.HandlerFunc(h.HandleAppWake)))

	mux.Handle("GET /repos", protected(http.HandlerFunc(h.ServeRepos)))
	mux.Handle("GET /repos/{org}/{repo}/delete-all", protected(http.HandlerFunc(h.ServeRepoDeleteAllConfirm)))
	mux.Handle("POST /repos/{org}/{repo}/delete-all", protected(http.HandlerFunc(h.HandleRepoDeleteAll)))
	mux.Handle("POST /repos/{org}/{repo}/promote", protected(http.HandlerFunc(h.HandleRepoPromote)))
	mux.Handle("POST /repos/{org}/{repo}/demote", protected(http.HandlerFunc(h.HandleRepoDemote)))

	mux.Handle("GET /approvals", protected(http.HandlerFunc(h.ServeApprovals)))
	mux.Handle("POST /approvals/batch", protected(http.HandlerFunc(h.HandleApprovalsBatch)))

	mux.Handle("GET /cost", protected(http.HandlerFunc(h.ServeCost)))
	mux.Handle("GET /status", protected(http.HandlerFunc(h.ServeStatus)))

	mux.Handle("GET /stale", protected(http.HandlerFunc(h.ServeStale)))
	mux.Handle("POST /stale/{org}/{repo}/{appName}/ignore", protected(http.HandlerFunc(h.HandleStaleIgnore)))

	mux.Handle("GET /operators", protected(http.HandlerFunc(h.ServeOperators)))

	mux.HandleFunc("GET /healthz", receiver.HealthzHandler)

	// Root → /apps redirect.
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/apps", http.StatusFound)
	})

	return middleware.InjectLogger(ctxlog.From(ctx), middleware.LogRequests(mux))
}
