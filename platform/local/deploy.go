package local

import (
	"context"

	"github.com/ifeanyiecheruo/morsel/platform"
)

type localDeployer struct{}

func (d *localDeployer) Credentials(_ context.Context) (platform.DeployCredentials, error) {
	return platform.DeployCredentials{}, platform.ErrNotImplemented
}
func (d *localDeployer) StagingRegistry() string { return "" }
