package tokens

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

func GenerateRefreshToken() ([]byte, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, "", fmt.Errorf("generate refresh token: %w", err)
	}
	return raw, base64.RawURLEncoding.EncodeToString(raw), nil
}

// Store this hash in the database — never store the raw bytes.
func HashRefreshToken(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
