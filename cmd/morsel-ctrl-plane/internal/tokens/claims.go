package tokens

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	RoleDeveloper    = "developer"
	RoleOperator     = "operator"
	OperatorTokenTTL = 15 * time.Minute
	AdminSessionTTL  = 8 * time.Hour

	deployTokenTTL = 10 * time.Minute
)

type Claims struct {
	jwt.RegisteredClaims
	Repo string `json:"repo,omitempty"` // set for developer tokens; empty for operator tokens
	Role string `json:"role"`
}

func CreateOperatorClaims(subject string) Claims {
	now := time.Now()
	return Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(OperatorTokenTTL)),
		},
		Role: RoleOperator,
	}
}

// CreateAdminSessionClaims issues an operator JWT with a long TTL for the admin
// UI browser session. The role is still RoleOperator so the API middleware
// accepts it if needed, but the session is meant for cookie-based auth only.
func CreateAdminSessionClaims(subject string) Claims {
	now := time.Now()
	return Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(AdminSessionTTL)),
		},
		Role: RoleOperator,
	}
}

func CreateDeployClaims(slug string) Claims {
	now := time.Now()
	return Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "repo:" + slug,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(deployTokenTTL)),
		},
		Repo: slug,
		Role: RoleDeveloper,
	}
}
