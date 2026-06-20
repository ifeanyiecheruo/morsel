package local

import (
	"context"

	"github.com/ifeanyiecheruo/morsel/internal/platform"
)

type localDeployer struct{}

func (ld *localDeployer) Credentials(_ context.Context) (platform.DeployCredentials, error) {
	return platform.DeployCredentials{}, platform.ErrNotImplemented
}
func (ld *localDeployer) StagingRegistry() string { return "" }
