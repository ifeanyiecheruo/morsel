package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/api/handler"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/api/middleware"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/api/server"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/api/wellknown"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/names"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/platform"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/store"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
	"github.com/ifeanyiecheruo/morsel/internal/kube"
)

// NewMux constructs the root HTTP handler for the Morsel API using the
// ogen-generated router. Panics if the server cannot be constructed (indicates
// a programmer error such as a nil handler).
func NewMux(ctx context.Context, plat platform.Platform, s *store.Store, deployer handler.AppDeployer) http.Handler {
	keys, err := plat.Secrets().EnsureSigningKey(ctx)
	if err != nil || len(keys) == 0 {
		panic("morsel api: signing key unavailable: " + err.Error())
	}
	signingKey := keys[0]
	h := handler.New(plat, s, signingKey, deployer)
	sec := handler.NewSecurityHandler(signingKey)

	srv, err := server.NewServer(h, sec,
		server.WithErrorHandler(handler.WriteError),
		server.WithNotFound(func(w http.ResponseWriter, _ *http.Request) {
			writeJSONError(w, http.StatusNotFound, "not_found", "the requested resource was not found", "check the API documentation for valid endpoints")
		}),
		server.WithMethodNotAllowed(func(w http.ResponseWriter, _ *http.Request, _ string) {
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "this HTTP method is not allowed for this endpoint", "check the API documentation for allowed methods")
		}),
	)
	if err != nil {
		panic("morsel api: failed to construct ogen server: " + err.Error())
	}

	mux := http.NewServeMux()
	mux.Handle("/.well-known/", wellknown.New("/.well-known"))
	mux.Handle("/internal/wake", internalWakeHandler(ctx, s, deployer, plat))
	mux.Handle("/", srv)

	return middleware.InjectLogger(ctxlog.From(ctx), middleware.LogRequests(mux))
}

// wakeResponse is returned by the internal wake endpoint.
type wakeResponse struct {
	ServiceAddr string `json:"service_addr"`
}

// internalWakeHandler handles POST /internal/wake?host={hostname} requests from
// the wake proxy. It scales up the app if hibernated, waits for it to become
// ready, then returns the in-cluster service address for the proxy to forward to.
func internalWakeHandler(_ context.Context, s *store.Store, deployer handler.AppDeployer, plat platform.Platform) http.Handler {
	wakeToken := os.Getenv("WAKE_PROXY_TOKEN")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required", "use POST")
			return
		}

		if wakeToken != "" {
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if got != wakeToken {
				writeJSONError(w, http.StatusUnauthorized, "unauthorized", "invalid wake token", "")
				return
			}
		}

		host := r.URL.Query().Get("host")
		if host == "" {
			writeJSONError(w, http.StatusBadRequest, "invalid_request", "host query param required", "use ?host={hostname}")
			return
		}

		reqCtx := r.Context()

		// Find the app whose public hostname matches.
		apps, err := s.ListAllApps(reqCtx)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "internal_error", "list apps failed", "")
			return
		}

		baseDomain := plat.BaseDomain()
		var matchedApp *struct {
			id        int64
			appType   string
			namespace string
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
				}{id: app.ID, appType: app.Type, namespace: app.Namespace.String}
				break
			}
		}
		if matchedApp == nil {
			writeJSONError(w, http.StatusNotFound, "not_found", "no app found for host", "check the hostname")
			return
		}

		_ = s.UpdateLastActiveAt(reqCtx, matchedApp.id)

		if err := wakeApp(reqCtx, s, deployer, plat, matchedApp.id, matchedApp.appType, matchedApp.namespace); err != nil {
			writeJSONError(w, http.StatusServiceUnavailable, "wake_failed", fmt.Sprintf("wake error: %v", err), "retry in a moment")
			return
		}

		serviceAddr := names.AppServiceAddr(matchedApp.namespace)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(wakeResponse{ServiceAddr: serviceAddr})
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
		if err := deployer.RestoreHTTPRoute(ctx, namespace, host, plat.Namespace(), kube.GatewayExternal, 8080); err != nil {
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

type jsonErrorBody struct {
	Error jsonErrorDetail `json:"error"`
}

type jsonErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Remedy  string `json:"remedy"`
}

func writeJSONError(w http.ResponseWriter, status int, code, message, remedy string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(jsonErrorBody{Error: jsonErrorDetail{Code: code, Message: message, Remedy: remedy}})
}
