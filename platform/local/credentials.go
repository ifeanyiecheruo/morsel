package local

import (
	"context"

	"github.com/ifeanyiecheruo/morsel/platform"
)

type localCredentialProvider struct{}

// AmbientToken returns "" — no ambient cloud identity is needed locally.
func (lc *localCredentialProvider) AmbientToken(_ context.Context) (string, error) { return "", nil }

func (lc *localCredentialProvider) DeployToken(_ context.Context) (string, error) {
	return "", platform.ErrNotImplemented
}
func (lc *localCredentialProvider) ValidateDeployToken(_ context.Context, _ string) (string, error) {
	return "", platform.ErrNotImplemented
}
