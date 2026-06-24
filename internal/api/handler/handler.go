// Package handler implements the ogen Handler and SecurityHandler interfaces
// for the Morsel API server.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/ogen-go/ogen/ogenerrors"

	"github.com/ifeanyiecheruo/morsel/internal/api/oas"
	"github.com/ifeanyiecheruo/morsel/internal/kube"
	"github.com/ifeanyiecheruo/morsel/internal/platform"
	"github.com/ifeanyiecheruo/morsel/internal/store"
	"github.com/ifeanyiecheruo/morsel/internal/tokens"
)

// Deployer is the subset of kube.Client methods used by the handler.
type Deployer interface {
	Apply(ctx context.Context, m kube.AppManifest) error
	WatchDeploymentRollout(ctx context.Context, namespace string) error
	RollbackDeployment(ctx context.Context, namespace, lastHealthyImage string) error
	AppStatus(ctx context.Context, namespace, appType string) string
}

// AppPlatform is the subset of platform.Platform that API handlers may consume.
// Expanded as stub methods are implemented.
type AppPlatform interface {
	Secrets() platform.Secrets
	Tokens() platform.Tokens
}

// Handler implements oas.Handler for all Morsel API operations.
type Handler struct {
	plat       AppPlatform
	store      *store.Store
	signingKey []byte
	deployer   Deployer
}

// New constructs a Handler.
func New(plat AppPlatform, s *store.Store, signingKey []byte, deployer Deployer) *Handler {
	return &Handler{plat: plat, store: s, signingKey: signingKey, deployer: deployer}
}

// apiError is the internal structured error type. It is written by WriteError
// as the morsel JSON error shape: {"error": {"code", "message", "remedy"}}.
type apiError struct {
	httpStatus int
	code       string
	message    string
	remedy     string
}

func (e *apiError) Error() string { return e.code + ": " + e.message }

var _ error = (*apiError)(nil)

// errNotImplemented is returned by stub handler methods for endpoints not yet built.
var errNotImplemented = &apiError{
	httpStatus: http.StatusNotImplemented,
	code:       "not_implemented",
	message:    "this endpoint is not yet implemented",
	remedy:     "check back in a future release",
}

// WriteError is the error handler for ogen's WithErrorHandler option. It
// translates any error into the morsel JSON error shape.
func WriteError(_ context.Context, w http.ResponseWriter, _ *http.Request, err error) {
	var ae *apiError
	if errors.As(err, &ae) {
		// Already a structured API error from a handler or security check.
	} else if code := ogenerrors.ErrorCode(err); code != http.StatusInternalServerError {
		// ogen routing/validation error (404, 405, 400, etc.) — surface it as structured.
		ae = ogenAPIError(code)
	} else {
		ae = &apiError{
			httpStatus: http.StatusInternalServerError,
			code:       "internal_error",
			message:    "an unexpected error occurred",
			remedy:     "contact your platform operator if the issue persists",
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(ae.httpStatus)
	type errorDetail struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Remedy  string `json:"remedy"`
	}
	_ = json.NewEncoder(w).Encode(struct {
		Error errorDetail `json:"error"`
	}{Error: errorDetail{Code: ae.code, Message: ae.message, Remedy: ae.remedy}})
}

// ogenAPIError converts an ogen HTTP status code into a structured apiError.
func ogenAPIError(code int) *apiError {
	switch code {
	case http.StatusNotFound:
		return &apiError{httpStatus: code, code: "not_found", message: "the requested resource was not found", remedy: "check the API documentation for valid endpoints"}
	case http.StatusMethodNotAllowed:
		return &apiError{httpStatus: code, code: "method_not_allowed", message: "this HTTP method is not allowed for this endpoint", remedy: "check the API documentation for allowed methods"}
	case http.StatusBadRequest:
		return &apiError{httpStatus: code, code: "invalid_request", message: "the request body or parameters are invalid", remedy: "check the request format against the API documentation"}
	case http.StatusUnauthorized:
		return &apiError{httpStatus: code, code: "invalid_token", message: "authentication is required", remedy: "include a valid Morsel access token in the Authorization header"}
	case http.StatusForbidden:
		return &apiError{httpStatus: code, code: "forbidden", message: "you do not have permission to access this resource", remedy: "check your token role and repository access"}
	default:
		return &apiError{httpStatus: code, code: "request_error", message: "the request could not be processed", remedy: "check the API documentation"}
	}
}

// claimsKey is the context key for JWT claims injected by HandleBearerAuth.
type claimsKey struct{}

func claimsFromContext(ctx context.Context) *tokens.Claims {
	v, _ := ctx.Value(claimsKey{}).(*tokens.Claims)
	return v
}

func repoSlug(org, repo string) string { return org + "/" + repo }

// checkRepoAccess enforces the repo claim for developer tokens; operator tokens bypass.
func checkRepoAccess(ctx context.Context, org, repo string) error {
	claims := claimsFromContext(ctx)
	if claims == nil {
		return &apiError{
			httpStatus: http.StatusUnauthorized,
			code:       "invalid_token",
			message:    "request is missing an Authorization: Bearer header",
			remedy:     "include a valid Morsel access token in the Authorization header",
		}
	}
	if claims.Role == tokens.RoleDeveloper && claims.Repo != repoSlug(org, repo) {
		return &apiError{
			httpStatus: http.StatusForbidden,
			code:       "repo_mismatch",
			message:    "token is not authorised for this repository",
			remedy:     "use a deploy token scoped to this repository",
		}
	}
	return nil
}

// requireOperator enforces the operator role.
func requireOperator(ctx context.Context) error {
	claims := claimsFromContext(ctx)
	if claims == nil || claims.Role != tokens.RoleOperator {
		return &apiError{
			httpStatus: http.StatusForbidden,
			code:       "insufficient_role",
			message:    "this operation requires operator role",
			remedy:     "log in as a platform operator",
		}
	}
	return nil
}

// ── Health ────────────────────────────────────────────────────────────────────

func (h *Handler) GetHealthz(_ context.Context) (*oas.GetHealthzOK, error) {
	return &oas.GetHealthzOK{Status: "ok"}, nil
}

// NewError converts any handler error into the ogen convenient-error envelope.
// Called by the ogen-generated server when a handler returns an error rather
// than a typed response value (including security failures).
func (h *Handler) NewError(_ context.Context, err error) *oas.ErrorInternalServerStatusCode {
	var ae *apiError
	if errors.As(err, &ae) {
		return &oas.ErrorInternalServerStatusCode{
			StatusCode: ae.httpStatus,
			Response: oas.ErrorResponse{
				Error: oas.ErrorDetail{
					Code:    ae.code,
					Message: ae.message,
					Remedy:  ae.remedy,
				},
			},
		}
	}
	// ogen security and routing errors carry a structured HTTP status code.
	if code := ogenerrors.ErrorCode(err); code != http.StatusInternalServerError {
		ae = ogenAPIError(code)
		return &oas.ErrorInternalServerStatusCode{
			StatusCode: ae.httpStatus,
			Response: oas.ErrorResponse{
				Error: oas.ErrorDetail{
					Code:    ae.code,
					Message: ae.message,
					Remedy:  ae.remedy,
				},
			},
		}
	}
	return &oas.ErrorInternalServerStatusCode{
		StatusCode: http.StatusInternalServerError,
		Response: oas.ErrorResponse{
			Error: oas.ErrorDetail{
				Code:    "internal_error",
				Message: "an unexpected error occurred",
				Remedy:  "contact your platform operator if the issue persists",
			},
		},
	}
}

// compile-time interface check.
var _ oas.Handler = (*Handler)(nil)
