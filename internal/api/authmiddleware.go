package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/ifeanyiecheruo/morsel/internal/tokens"
)

type claimsContextKey struct{}

// requireAuth is middleware that verifies the Bearer JWT on every request and
// attaches the parsed claims to the context. Unauthenticated requests are
// rejected with 401 before reaching the wrapped handler.
func requireAuth(signingKey []byte, next http.Handler) http.Handler {
	return http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		tokenStr, ok := strings.CutPrefix(req.Header.Get("Authorization"), "Bearer ")
		if !ok || tokenStr == "" {
			writeError(resp, &APIError{
				HTTPStatus: http.StatusUnauthorized,
				Code:       "invalid_token",
				Message:    "request is missing an Authorization: Bearer header",
				Remedy:     "include a valid Morsel access token in the Authorization header",
			})
			return
		}

		claims, err := tokens.Verify(signingKey, tokenStr)
		if err != nil {
			writeError(resp, &APIError{
				HTTPStatus: http.StatusUnauthorized,
				Code:       "invalid_token",
				Message:    "access token is invalid or expired",
				Remedy:     "re-authenticate to obtain a fresh access token",
			})
			return
		}

		next.ServeHTTP(resp, req.WithContext(
			context.WithValue(req.Context(), claimsContextKey{}, claims),
		))
	})
}

// requireRepo wraps a handler and enforces that the token's repo claim matches
// the {org}/{repo} path values. Operator tokens bypass the check.
// Must be used inside a requireAuth-protected mux so claims are always present.
func requireRepo(handler func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error {
	return func(resp http.ResponseWriter, req *http.Request) error {
		claims := claimsFromContext(req.Context())
		if claims == nil {
			return &APIError{
				HTTPStatus: http.StatusUnauthorized,
				Code:       "invalid_token",
				Message:    "request is missing an Authorization: Bearer header",
				Remedy:     "include a valid Morsel access token in the Authorization header",
			}
		}
		if claims.Role == tokens.RoleDeveloper && claims.Repo != repoSlug(req) {
			return &APIError{
				HTTPStatus: http.StatusForbidden,
				Code:       "repo_mismatch",
				Message:    "token is not authorised for this repository",
				Remedy:     "use a deploy token scoped to this repository",
			}
		}
		return handler(resp, req)
	}
}

// requireOperator wraps a handler and enforces operator role.
// Must be used inside a requireAuth-protected mux so claims are always present.
func requireOperator(handler func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error {
	return func(resp http.ResponseWriter, req *http.Request) error {
		claims := claimsFromContext(req.Context())
		if claims == nil || claims.Role != tokens.RoleOperator {
			return &APIError{
				HTTPStatus: http.StatusForbidden,
				Code:       "insufficient_role",
				Message:    "this operation requires operator role",
				Remedy:     "log in as a platform operator",
			}
		}
		return handler(resp, req)
	}
}

// claimsFromContext returns the verified claims attached to the request context,
// or nil if the request was not authenticated.
func claimsFromContext(ctx context.Context) *tokens.Claims {
	v, _ := ctx.Value(claimsContextKey{}).(*tokens.Claims)
	return v
}

// repoSlug reconstructs the repo slug from the {org} and {repo} path values.
// Route patterns use two segments because slugs like "org/my-repo" contain a slash.
func repoSlug(r *http.Request) string {
	return r.PathValue("org") + "/" + r.PathValue("repo")
}
