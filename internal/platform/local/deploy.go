package local

import (
	"context"

	"github.com/ifeanyiecheruo/morsel/internal/platform"
)

type localDeployer struct {
	secrets *localSecrets
}

func (ld *localDeployer) Credentials(ctx context.Context) (platform.DeployCredentials, error) {
	keys, err := ld.secrets.GetDeploySigningKeys(ctx)
	if err != nil {
		return platform.DeployCredentials{}, err
	}
	if len(keys) == 0 {
		return platform.DeployCredentials{}, platform.ErrNotImplemented
	}
	return platform.DeployCredentials{}, platform.ErrPrincipalNotAuthorized
}

func (ld *localDeployer) StagingRegistry() string { return "" }
