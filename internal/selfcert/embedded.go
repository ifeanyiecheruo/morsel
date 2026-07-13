package selfcert

import (
	"crypto/tls"
	"embed"
	"encoding/base64"
	"fmt"
	"strings"
)

//go:embed embedded/cert.pem.b64 embedded/key.pem.b64
var embeddedFS embed.FS

// LoadEmbeddedWildcardCert returns the wildcard certificate for LocalBaseDomain
// that ships in the repo, base64-encoded, under internal/selfcert/embedded.
//
// The cert is generated once (see internal/selfcert/embedded/README.md) and
// committed rather than regenerated on every bootstrap, so a developer who has
// trusted it in their OS/browser trust store only has to do so once instead of
// on every local platform provision.
func LoadEmbeddedWildcardCert() (*tls.Certificate, error) {
	certPEM, err := readEmbeddedPEM("embedded/cert.pem.b64")
	if err != nil {
		return nil, fmt.Errorf("read embedded cert: %w", err)
	}
	keyPEM, err := readEmbeddedPEM("embedded/key.pem.b64")
	if err != nil {
		return nil, fmt.Errorf("read embedded key: %w", err)
	}

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("build tls cert: %w", err)
	}
	return &tlsCert, nil
}

func readEmbeddedPEM(path string) ([]byte, error) {
	encoded, err := embeddedFS.ReadFile(path)
	if err != nil {
		return nil, err
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encoded)))
	if err != nil {
		return nil, fmt.Errorf("decode base64: %w", err)
	}
	return decoded, nil
}
