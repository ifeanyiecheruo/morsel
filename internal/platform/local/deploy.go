package local

import (
	"context"

	"github.com/ifeanyiecheruo/morsel/internal/platform"
)

// localRegistryURL is the address of the in-cluster registry:2 provisioned during
// bootstrap. Docker Desktop exposes LoadBalancer services at localhost, so this
// URL works for both host-side push and in-cluster pod pull on Docker Desktop.
const localRegistryURL = "localhost:5000"

type localDeployer struct {
	secrets *localSecrets
}

// Credentials returns empty credentials for LocalPlatform — the local registry
// requires no authentication. Returns ErrNotImplemented when the deploy signing
// key has not yet been provisioned (i.e. bootstrap has not run).
func (ld *localDeployer) Credentials(ctx context.Context) (platform.DeployCredentials, error) {
	keys, err := ld.secrets.GetDeploySigningKeys(ctx)
	if err != nil {
		return platform.DeployCredentials{}, err
	}
	if len(keys) == 0 {
		return platform.DeployCredentials{}, platform.ErrNotImplemented
	}
	return platform.DeployCredentials{}, nil
}

func (ld *localDeployer) StagingRegistry() string { return localRegistryURL }
