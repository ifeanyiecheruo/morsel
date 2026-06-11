package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/ifeanyiecheruo/morsel/internal/api/routes"
	"github.com/ifeanyiecheruo/morsel/internal/tokens"
)

type claimsContextKey struct{}

func RequireAuth(signingKey []byte, next http.Handler) http.Handler {
	return routes.ErrorHandlerFunc(func(resp http.ResponseWriter, req *http.Request) error {
		tokenStr, ok := strings.CutPrefix(req.Header.Get("Authorization"), "Bearer ")
		if !ok || tokenStr == "" {
			return &routes.APIError{
				HTTPStatus: http.StatusUnauthorized,
				Code:       "invalid_token",
				Message:    "request is missing an Authorization: Bearer header",
				Remedy:     "include a valid Morsel access token in the Authorization header",
			}
		}

		claims, err := tokens.Verify(signingKey, tokenStr)
		if err != nil {
			return &routes.APIError{
				HTTPStatus: http.StatusUnauthorized,
				Code:       "invalid_token",
				Message:    "access token is invalid or expired",
				Remedy:     "re-authenticate to obtain a fresh access token",
			}
		}

		next.ServeHTTP(resp, req.WithContext(
			context.WithValue(req.Context(), claimsContextKey{}, claims),
		))
		return nil
	})
}

// Must be used inside a RequireAuth-protected mux so claims are always present.
func RequireRepo(handler func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error {
	return func(resp http.ResponseWriter, req *http.Request) error {
		claims := claimsFromContext(req.Context())
		if claims == nil {
			return &routes.APIError{
				HTTPStatus: http.StatusUnauthorized,
				Code:       "invalid_token",
				Message:    "request is missing an Authorization: Bearer header",
				Remedy:     "include a valid Morsel access token in the Authorization header",
			}
		}
		if claims.Role == tokens.RoleDeveloper && claims.Repo != repoSlug(req) {
			return &routes.APIError{
				HTTPStatus: http.StatusForbidden,
				Code:       "repo_mismatch",
				Message:    "token is not authorised for this repository",
				Remedy:     "use a deploy token scoped to this repository",
			}
		}
		return handler(resp, req)
	}
}

// Must be used inside a RequireAuth-protected mux so claims are always present.
func RequireOperator(handler func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error {
	return func(resp http.ResponseWriter, req *http.Request) error {
		claims := claimsFromContext(req.Context())
		if claims == nil || claims.Role != tokens.RoleOperator {
			return &routes.APIError{
				HTTPStatus: http.StatusForbidden,
				Code:       "insufficient_role",
				Message:    "this operation requires operator role",
				Remedy:     "log in as a platform operator",
			}
		}
		return handler(resp, req)
	}
}

func claimsFromContext(ctx context.Context) *tokens.Claims {
	v, _ := ctx.Value(claimsContextKey{}).(*tokens.Claims)
	return v
}

// Route patterns use two segments because slugs like "org/my-repo" contain a slash.
func repoSlug(r *http.Request) string {
	return r.PathValue("org") + "/" + r.PathValue("repo")
}
