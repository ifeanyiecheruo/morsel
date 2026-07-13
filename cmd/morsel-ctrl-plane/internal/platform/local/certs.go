package local

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/platform"
	"github.com/ifeanyiecheruo/morsel/internal/selfcert"
)

type localCertProvider struct{}

// Provision returns the wildcard TLS certificate committed under
// internal/selfcert/embedded, so a developer who has trusted it once doesn't
// have to re-trust a freshly generated cert on every provision.
func (lct *localCertProvider) Provision(_ context.Context, _ string) (*tls.Certificate, error) {
	return selfcert.LoadEmbeddedWildcardCert()
}

// Renew returns the same embedded certificate as Provision. For the local
// platform renewal is a no-op — the embedded cert is long-lived and rotated
// manually (see internal/selfcert/embedded/README.md) rather than via ACME.
func (lct *localCertProvider) Renew(_ context.Context, _ string, _ time.Duration) (*tls.Certificate, error) {
	return selfcert.LoadEmbeddedWildcardCert()
}

var _ platform.CertProvider = (*localCertProvider)(nil)
