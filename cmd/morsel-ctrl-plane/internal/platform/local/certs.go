package local

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/platform"
	"github.com/ifeanyiecheruo/morsel/internal/selfcert"
)

type localCertProvider struct{}

// Provision generates a self-signed wildcard TLS certificate for *.domain.
// The certificate is valid for 1 year.
func (lct *localCertProvider) Provision(_ context.Context, domain string) (*tls.Certificate, error) {
	return selfcert.GenerateSelfSignedWildcard(domain, 365*24*time.Hour)
}

// Renew generates a fresh self-signed wildcard TLS certificate for *.domain.
// For the local platform renewal is identical to provisioning — self-signed certs
// are regenerated rather than renewed via ACME.
func (lct *localCertProvider) Renew(_ context.Context, domain string, _ time.Duration) (*tls.Certificate, error) {
	return selfcert.GenerateSelfSignedWildcard(domain, 365*24*time.Hour)
}

var _ platform.CertProvider = (*localCertProvider)(nil)
