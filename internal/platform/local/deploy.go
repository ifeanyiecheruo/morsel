package local

import (
	"context"

	"github.com/ifeanyiecheruo/morsel/internal/platform"
	"github.com/ifeanyiecheruo/morsel/internal/secrets"
)

type localDeployer struct {
	secretMgr *secrets.Manager
}

func (ld *localDeployer) Credentials(ctx context.Context) (platform.DeployCredentials, error) {
	exists, err := ld.secretMgr.DeploySigningKeyExists(ctx)
	if err != nil {
		return platform.DeployCredentials{}, err
	}
	if !exists {
		return platform.DeployCredentials{}, platform.ErrNotImplemented
	}
	return platform.DeployCredentials{}, platform.ErrPrincipalNotAuthorized
}

func (ld *localDeployer) StagingRegistry() string { return "" }
