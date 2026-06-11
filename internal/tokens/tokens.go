// Package tokens provides JWT issuance and verification for the Morsel API.
package tokens

import (
	"crypto/rand"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	deployTokenTTL   = 10 * time.Minute
	operatorTokenTTL = 15 * time.Minute
)

// GenerateKey generates a random 32-byte HMAC-SHA256 signing key.
func GenerateKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate signing key: %w", err)
	}
	return key, nil
}

// IssueToken signs claims with key using HS256 and returns the compact JWT string.
func IssueToken(key []byte, claims Claims) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(key)
}

// Verify parses and validates a Morsel access token. Returns the claims on success.
func Verify(key []byte, tokenStr string) (*Claims, error) {
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
