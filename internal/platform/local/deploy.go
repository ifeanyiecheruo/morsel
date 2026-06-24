package local

import (
	"context"

	"github.com/ifeanyiecheruo/morsel/internal/platform"
)

type localDeployer struct {
	secrets *localSecrets
}

func (ld *localDeployer) Credentials(ctx context.Context) (platform.DeployCredentials, error) {
	exists, err := ld.secrets.DeploySigningKeyExists(ctx)
	if err != nil {
		return platform.DeployCredentials{}, err
	}
	if !exists {
		return platform.DeployCredentials{}, platform.ErrNotImplemented
	}
	return platform.DeployCredentials{}, platform.ErrPrincipalNotAuthorized
}

func (ld *localDeployer) StagingRegistry() string { return "" }
