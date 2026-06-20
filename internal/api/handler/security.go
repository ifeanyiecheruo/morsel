package handler

import (
	"context"

	"github.com/ifeanyiecheruo/morsel/internal/api/oas"
	"github.com/ifeanyiecheruo/morsel/internal/tokens"
)

// SecurityHandler implements oas.SecurityHandler for bearerAuth.
type SecurityHandler struct {
	signingKey []byte
}

// NewSecurityHandler constructs a SecurityHandler.
func NewSecurityHandler(signingKey []byte) *SecurityHandler {
	return &SecurityHandler{signingKey: signingKey}
}

// HandleBearerAuth validates the JWT from the Authorization header, then stores
// the claims in the context so handler methods can read them via claimsFromContext.
func (s *SecurityHandler) HandleBearerAuth(ctx context.Context, _ oas.OperationName, t oas.BearerAuth) (context.Context, error) {
	if t.Token == "" {
		return ctx, &apiError{
			httpStatus: 401,
			code:       "invalid_token",
			message:    "Authorization: Bearer header is required",
			remedy:     "include a valid Morsel access token in the Authorization header",
		}
	}
	claims, err := tokens.Verify(s.signingKey, t.Token)
	if err != nil {
		return ctx, &apiError{
			httpStatus: 401,
			code:       "invalid_token",
			message:    "access token is invalid or expired",
			remedy:     "obtain a new token via POST /api/token/deploy or POST /api/token/refresh",
		}
	}
	return context.WithValue(ctx, claimsKey{}, claims), nil
}

// compile-time interface check.
var _ oas.SecurityHandler = (*SecurityHandler)(nil)
