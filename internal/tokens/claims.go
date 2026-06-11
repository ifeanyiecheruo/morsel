package tokens

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	RoleDeveloper = "developer"
	RoleOperator  = "operator"

	deployTokenTTL   = 10 * time.Minute
	OperatorTokenTTL = 15 * time.Minute
)

// Claims is the JWT payload for all Morsel access tokens.
type Claims struct {
	jwt.RegisteredClaims
	Repo string `json:"repo,omitempty"` // set for developer tokens; empty for operator tokens
	Role string `json:"role"`
}

// CreateOperatorClaims builds the Claims for an operator access token with the given subject.
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

// CreateDeployClaims builds the Claims for a developer deploy token scoped to slug.
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
