// Package tokens provides JWT issuance and verification for the Morsel API.
package tokens

import (
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

func IssueToken(key []byte, claims Claims) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(key)
}

func VerifyToken(key []byte, tokenStr string) (*Claims, error) {
	var claims Claims
	_, err := jwt.ParseWithClaims(tokenStr, &claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return key, nil
	})
	if err != nil {
		return nil, err
	}
	return &claims, nil
}
