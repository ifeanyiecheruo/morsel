package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/ifeanyiecheruo/morsel/internal/api/handler"
	"github.com/ifeanyiecheruo/morsel/internal/api/middleware"
	"github.com/ifeanyiecheruo/morsel/internal/api/oas"
	"github.com/ifeanyiecheruo/morsel/internal/api/wellknown"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
	dbqueries "github.com/ifeanyiecheruo/morsel/internal/db/queries"
	"github.com/ifeanyiecheruo/morsel/platform"
)

// AppPlatform is the subset of platform.Platform that API handlers are allowed
// to consume. Secrets() and Bootstrap() are deliberately absent: secrets must
// be accessed through secrets.Manager, and bootstrapping is a CLI concern.
type AppPlatform interface {
	Credentials() platform.CredentialProvider
	Deploy() platform.Deployer
	Blobs() platform.BlobStore
	DNS() platform.DNSProvider
	Certs() platform.CertProvider
	Pricing() platform.PricingProvider
}

// NewMux constructs the root HTTP handler for the Morsel API using the
// ogen-generated router. Panics if the server cannot be constructed (indicates
// a programmer error such as a nil handler).
func NewMux(ctx context.Context, plat AppPlatform, signingKey []byte, queries *dbqueries.Queries) http.Handler {
	h := handler.New(plat, signingKey, queries)
	sec := handler.NewSecurityHandler(signingKey)

	srv, err := oas.NewServer(h, sec,
		oas.WithErrorHandler(handler.WriteError),
		oas.WithNotFound(func(w http.ResponseWriter, _ *http.Request) {
			writeJSONError(w, http.StatusNotFound, "not_found", "the requested resource was not found", "check the API documentation for valid endpoints")
		}),
		oas.WithMethodNotAllowed(func(w http.ResponseWriter, _ *http.Request, _ string) {
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "this HTTP method is not allowed for this endpoint", "check the API documentation for allowed methods")
		}),
	)
	if err != nil {
		panic("morsel api: failed to construct ogen server: " + err.Error())
	}

	wk, err := wellknown.New("/.well-known")
	if err != nil {
		panic("morsel api: failed to bundle OpenAPI spec: " + err.Error())
	}

	mux := http.NewServeMux()
	mux.Handle("/.well-known/", wk)
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
