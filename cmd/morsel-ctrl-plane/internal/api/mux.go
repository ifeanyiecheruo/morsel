package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/api/handler"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/api/server"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/api/wellknown"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/cost"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/middleware"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/names"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/platform"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/store"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
	"github.com/ifeanyiecheruo/morsel/internal/health"
	"github.com/ifeanyiecheruo/morsel/internal/kube"
)

// NewMux constructs the root HTTP handler for the Morsel API using the
// ogen-generated router. Panics if the server cannot be constructed (indicates
// a programmer error such as a nil handler).
// githubClientID is exposed via GET /api/auth/github/config for CLI Device Flow.
func NewMux(ctx context.Context, plat platform.Platform, s *store.Store, deployer handler.AppDeployer, receiver *health.Receiver, githubClientID string) http.Handler {
	keys, err := plat.Secrets().EnsureSigningKey(ctx)
	if err != nil || len(keys) == 0 {
		panic("morsel api: signing key unavailable: " + err.Error())
	}
	signingKey := keys[0]
	h := handler.New(plat, s, signingKey, deployer, receiver, githubClientID)
	sec := handler.NewSecurityHandler(signingKey)

	srv, err := server.NewServer(h, sec,
		server.WithErrorHandler(handler.WriteError),
		server.WithNotFound(func(w http.ResponseWriter, r *http.Request) {
			writeJSONError(r.Context(), w, http.StatusNotFound, "not_found", "the requested resource was not found", "check the API documentation for valid endpoints")
		}),
		server.WithMethodNotAllowed(func(w http.ResponseWriter, r *http.Request, _ string) {
			writeJSONError(r.Context(), w, http.StatusMethodNotAllowed, "method_not_allowed", "this HTTP method is not allowed for this endpoint", "check the API documentation for allowed methods")
		}),
	)
	if err != nil {
		panic("morsel api: failed to construct ogen server: " + err.Error())
	}

	// ── API mux (api.<baseDomain>) ────────────────────────────────────────────
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("GET /livez", receiver.LivezHandler)
	apiMux.HandleFunc("GET /readyz", receiver.ReadyzHandler)
	apiMux.Handle("/.well-known/", wellknown.New("/.well-known"))
	apiMux.Handle("/internal/wake", internalWakeHandler(ctx, s, deployer, plat))
	apiMux.HandleFunc("POST /bootstrap", h.HandleBootstrap)
	apiMux.HandleFunc("POST /token/oidc", h.HandleGitHubAuth)
	apiMux.HandleFunc("GET /github/config", h.HandleGitHubConfig)
	apiMux.HandleFunc("PATCH /api/operator/principals/{github_login}", h.HandlePrincipalPatch)
	apiMux.HandleFunc("GET /api/operator/apps", h.HandleAdminListApps)
	apiMux.HandleFunc("GET /api/operator/stale", h.HandleAdminListStale)
	apiMux.HandleFunc("POST /api/operator/stale/{org}/{repo}/{appName}/ignore", h.HandleAdminIgnoreStale)
	apiMux.Handle("/", srv)

	return middleware.InjectLogger(ctxlog.From(ctx), middleware.LogRequests(apiMux))
}

// wakeResponse is returned by the internal wake endpoint.
type wakeResponse struct {
	ServiceAddr string `json:"service_addr"`
}

// internalWakeHandler handles POST /internal/wake?host={hostname} requests from
// the wake proxy. It scales up the app if hibernated, waits for it to become
// ready, then returns the in-cluster service address for the proxy to forward to.
func internalWakeHandler(_ context.Context, s *store.Store, deployer handler.AppDeployer, plat platform.Platform) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(r.Context(), w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required", "use POST")
			return
		}

		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" {
			writeJSONError(r.Context(), w, http.StatusUnauthorized, "unauthorized", "missing wake token", "")
			return
		}
		if err := deployer.VerifyWakeToken(r.Context(), token); err != nil {
			writeJSONError(r.Context(), w, http.StatusUnauthorized, "unauthorized", "invalid wake token", "")
			return
		}

		host := r.URL.Query().Get("host")
		if host == "" {
			writeJSONError(r.Context(), w, http.StatusBadRequest, "invalid_request", "host query param required", "use ?host={hostname}")
			return
		}

		reqCtx := r.Context()

		// Find the app whose public hostname matches.
		apps, err := s.ListAllApps(reqCtx)
		if err != nil {
			writeJSONError(reqCtx, w, http.StatusInternalServerError, "internal_error", "list apps failed", "")
			return
		}

		baseDomain := plat.BaseDomain()
		var matchedApp *struct {
			id        int64
			appType   string
			namespace string
			repoSlug  string
			name      string
		}
		for _, app := range apps {
			if !app.Namespace.Valid {
				continue
			}
			if names.AppHostname(app.Name, names.RepoName(app.RepoSlug), baseDomain) == host {
				matchedApp = &struct {
					id        int64
					appType   string
					namespace string
					repoSlug  string
					name      string
				}{id: app.ID, appType: app.Type, namespace: app.Namespace.String, repoSlug: app.RepoSlug, name: app.Name}
				break
			}
		}
		if matchedApp == nil {
			writeJSONError(reqCtx, w, http.StatusNotFound, "not_found", "no app found for host", "check the hostname")
			return
		}

		if err := s.UpdateLastActiveAt(reqCtx, matchedApp.id); err != nil {
			ctxlog.From(reqCtx).Warn("update last active at", "err", err)
		}

		// Budget enforcement: block wake when a limit is active and the app is not exempt.
		if retryAfter, blocked := isBudgetBlocked(reqCtx, s, matchedApp.repoSlug, matchedApp.name); blocked {
			w.Header().Set("Retry-After", retryAfter)
			writeJSONError(reqCtx, w, http.StatusServiceUnavailable, "budget_soft_limit",
				"platform is over budget for this period",
				"wait for the next billing period")
			return
		}

		if err := wakeApp(reqCtx, s, deployer, plat, matchedApp.id, matchedApp.appType, matchedApp.namespace); err != nil {
			writeJSONError(reqCtx, w, http.StatusServiceUnavailable, "wake_failed", fmt.Sprintf("wake error: %v", err), "retry in a moment")
			return
		}

		serviceAddr := names.AppServiceAddr(matchedApp.namespace)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(wakeResponse{ServiceAddr: serviceAddr}); err != nil {
			ctxlog.From(reqCtx).Warn("write wake response", "err", err)
		}
	})
}

func wakeApp(ctx context.Context, s *store.Store, deployer handler.AppDeployer, plat platform.Platform, appID int64, appType, namespace string) error {
	switch appType {
	case "http":
		if err := deployer.ScaleDeployment(ctx, namespace, 1); err != nil {
			return err
		}
		if err := deployer.WatchDeploymentReady(ctx, namespace, 5*time.Minute); err != nil {
			return err
		}
		// Restore HTTPRoute — requires knowing the host.
		app, err := s.GetAppByNamespace(ctx, namespace)
		if err != nil {
			return err
		}
		host := names.AppHostname(app.Name, names.RepoName(app.RepoSlug), plat.BaseDomain())
		if err := deployer.RestoreHTTPRoute(ctx, namespace, host, plat.Namespace(), kube.GatewayExternal); err != nil {
			return err
		}
	case "worker":
		if err := deployer.ScaleDeployment(ctx, namespace, 1); err != nil {
			return err
		}
	case "cron":
		if err := deployer.UnsuspendCronJob(ctx, namespace); err != nil {
			return err
		}
	}
	if err := s.SetAppAwake(ctx, appID); err != nil {
		return err
	}
	return s.UpdateAppStatus(ctx, appID, "running")
}

// isBudgetBlocked returns (retryAfter, true) when a budget limit is active and
// the app is not exempt. retryAfter is an RFC 7231 HTTP-date for the next
// billing period start so the client knows when to retry.
func isBudgetBlocked(ctx context.Context, s *store.Store, repoSlug, appName string) (string, bool) {
	cfg, err := s.GetPlatformConfig(ctx)
	if err != nil || (cfg.BudgetSoftLimitActive == 0 && cfg.BudgetHardLimitActive == 0) {
		return "", false
	}
	exempt, exemptErr := s.IsAppExempt(ctx, repoSlug, appName)
	if exemptErr != nil {
		ctxlog.From(ctx).Warn("check app budget exemption", "err", exemptErr)
	}
	if exempt {
		return "", false
	}
	retryAfter := cost.NextBillingPeriodStart(time.Now().UTC()).Format(http.TimeFormat)
	return retryAfter, true
}

type jsonErrorBody struct {
	Error jsonErrorDetail `json:"error"`
}

type jsonErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Remedy  string `json:"remedy"`
}

func writeJSONError(ctx context.Context, w http.ResponseWriter, status int, code, message, remedy string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(jsonErrorBody{Error: jsonErrorDetail{Code: code, Message: message, Remedy: remedy}}); err != nil {
		ctxlog.From(ctx).Warn("write error response", "err", err)
	}
}
