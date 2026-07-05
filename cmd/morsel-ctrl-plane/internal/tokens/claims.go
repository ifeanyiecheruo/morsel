package tokens

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	RoleDeveloper    = "developer"
	RoleOperator     = "operator"
	RoleAdmin        = "admin"
	OperatorTokenTTL = 15 * time.Minute
	AdminSessionTTL  = 8 * time.Hour

	deployTokenTTL = 10 * time.Minute
)

// IsOperatorRole reports whether r grants operator-level access (operator or admin).
func IsOperatorRole(r string) bool { return r == RoleOperator || r == RoleAdmin }

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

// CreateAdminAPIClaims issues a short-lived API token for an admin principal.
func CreateAdminAPIClaims(subject string) Claims {
	now := time.Now()
	return Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(OperatorTokenTTL)),
		},
		Role: RoleAdmin,
	}
}

// CreateAdminSessionClaims issues a long-lived JWT for the admin UI browser session.
// role should be RoleAdmin or RoleOperator depending on whether the principal is an admin.
func CreateAdminSessionClaims(subject, role string) Claims {
	now := time.Now()
	return Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(AdminSessionTTL)),
		},
		Role: role,
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
