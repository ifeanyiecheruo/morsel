package tokens

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	RoleDeveloper    = "developer"
	RoleOperator     = "operator"
	OperatorTokenTTL = 15 * time.Minute

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
