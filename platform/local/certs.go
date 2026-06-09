package local

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/ifeanyiecheruo/morsel/platform"
)

type localCertProvider struct{}

func (c *localCertProvider) Provision(_ context.Context, _ string) (*tls.Certificate, error) {
	return nil, platform.ErrNotImplemented
}
func (c *localCertProvider) Renew(_ context.Context, _ string, _ time.Duration) (*tls.Certificate, error) {
	return nil, platform.ErrNotImplemented
}
