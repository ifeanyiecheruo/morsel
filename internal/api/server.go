package api

import (
	"context"
	"fmt"
	"net/http"

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

// NewMux constructs the root HTTP mux for the Morsel API.
//
// Route patterns use {org}/{repo} for the repo slug because slugs (e.g.
// "localhost/my-app") contain a slash and Go's {name} wildcard only matches
// a single path segment. repoSlug(r) reconstructs the full slug from both
// path values.
func NewMux(ctx context.Context, plat AppPlatform, signingKey []byte, queries *dbqueries.Queries) http.Handler {
	mux := http.NewServeMux()

	// public registers a route with no authentication.
	public := func(pattern string, h func(http.ResponseWriter, *http.Request) error) {
		mux.Handle(pattern, ErrorHandlerFunc(h))
	}

	// authenticated requires a valid token; any role is accepted.
	authenticated := func(pattern string, h func(http.ResponseWriter, *http.Request) error) {
		mux.Handle(pattern, requireAuth(signingKey, ErrorHandlerFunc(h)))
	}

	// repoScoped requires a valid token and enforces that a developer token's
	// repo claim matches the {org}/{repo} path values. Operator tokens bypass.
	repoScoped := func(pattern string, h func(http.ResponseWriter, *http.Request) error) {
		mux.Handle(pattern, requireAuth(signingKey, ErrorHandlerFunc(requireRepo(h))))
	}

	// operatorOnly requires a valid token with the operator role.
	operatorOnly := func(pattern string, h func(http.ResponseWriter, *http.Request) error) {
		mux.Handle(pattern, requireAuth(signingKey, ErrorHandlerFunc(requireOperator(h))))
	}

	stub := handleNotImplemented

	// ---- Public: token exchange ------------------------------------------------
	public("GET /healthz", handleHealthz)
	public("POST /api/token/deploy", handleTokenDeployRoute(plat.Credentials(), signingKey))
	public("POST /api/token/refresh", handleTokenRefreshRoute(queries, signingKey))
	public("POST /api/token/local-oidc", stub)

	// ---- Repo-scoped: developer + operator -------------------------------------
	repoScoped("POST /api/repos/{org}/{repo}/sync", stub)
	repoScoped("POST /api/repos/{org}/{repo}/apps", stub)
	repoScoped("DELETE /api/repos/{org}/{repo}/apps/{name}", stub)
	repoScoped("DELETE /api/repos/{org}/{repo}", stub)
	repoScoped("GET /api/repos/{org}/{repo}/apps", stub)
	repoScoped("GET /api/repos/{org}/{repo}/apps/{name}", stub)
	repoScoped("GET /api/repos/{org}/{repo}/apps/{name}/status", stub)
	repoScoped("GET /api/repos/{org}/{repo}/apps/{name}/history", stub)
	repoScoped("GET /api/repos/{org}/{repo}/apps/{name}/utilisation", stub)
	repoScoped("GET /api/repos/{org}/{repo}/apps/{name}/operations/{id}", stub)
	repoScoped("POST /api/repos/{org}/{repo}/apps/{name}/hibernate", stub)
	repoScoped("POST /api/repos/{org}/{repo}/apps/{name}/wake", stub)
	repoScoped("GET /api/repos/{org}/{repo}", stub)
	repoScoped("GET /api/repos/{org}/{repo}/approvals", stub)

	// ---- Authenticated: listing (developer sees own repo; handler filters) -------------
	authenticated("GET /api/repos", stub)

	// ---- Operator only ---------------------------------------------------------
	operatorOnly("GET /api/operator/config", stub)
	operatorOnly("PATCH /api/operator/config", stub)
	operatorOnly("PATCH /api/operator/repos/{org}/{repo}", stub)
	operatorOnly("GET /api/operator/approvals", stub)
	operatorOnly("GET /api/operator/approvals/{id}", stub)
	operatorOnly("POST /api/operator/approvals/batch", stub)
	operatorOnly("GET /api/operator/cost", stub)
	operatorOnly("GET /api/operator/status", stub)

	// ---- Catch-all: 404 --------------------------------------------------------
	public("/", func(resp http.ResponseWriter, req *http.Request) error {
		return &APIError{
			HTTPStatus: http.StatusNotFound,
			Code:       "not_found",
			Message:    fmt.Sprintf("%s %s not found", req.Method, req.URL.Path),
			Remedy:     "check the API documentation for valid endpoints",
		}
	})

	return injectLogger(ctxlog.From(ctx), logRequests(mux))
}

func handleNotImplemented(_ http.ResponseWriter, _ *http.Request) error {
	return &APIError{
		HTTPStatus: http.StatusNotImplemented,
		Code:       "not_implemented",
		Message:    "this endpoint is not yet implemented",
		Remedy:     "check back in a future release",
	}
}
