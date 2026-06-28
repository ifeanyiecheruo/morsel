package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/api/handler"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/api/middleware"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/api/server"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/api/wellknown"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/store"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
	"github.com/ifeanyiecheruo/morsel/internal/platform"
)

// AppPlatform is the subset of platform.Platform that API handlers are allowed
// to consume. Bootstrap() is deliberately absent: bootstrapping is a CLI concern.
type AppPlatform interface {
	Namespace() string
	BaseDomain() string
	Secrets() platform.Secrets
	Tokens() platform.Tokens
	Deploy() platform.Deployer
	Blobs() platform.BlobStore
	DNS() platform.DNSProvider
	Certs() platform.CertProvider
	Pricing() platform.PricingProvider
}

// NewMux constructs the root HTTP handler for the Morsel API using the
// ogen-generated router. Panics if the server cannot be constructed (indicates
// a programmer error such as a nil handler).
func NewMux(ctx context.Context, plat AppPlatform, s *store.Store, deployer handler.Deployer) http.Handler {
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
	mux.Handle("/", srv)

	return middleware.InjectLogger(ctxlog.From(ctx), middleware.LogRequests(mux))
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
