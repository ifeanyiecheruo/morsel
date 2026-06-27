package local

import (
	"context"

	"github.com/ifeanyiecheruo/morsel/internal/platform"
)

// localRegistryURL is the address of the in-cluster registry:2 provisioned during
// bootstrap. Docker Desktop exposes LoadBalancer services at localhost, so this
// URL works for both host-side push and in-cluster pod pull on Docker Desktop.
const localRegistryURL = "localhost:5000"

type localDeployer struct{}

// Credentials returns empty credentials — the local registry requires no authentication.
func (ld *localDeployer) Credentials(_ context.Context) (platform.DeployCredentials, error) {
	return platform.DeployCredentials{}, nil
}

func (ld *localDeployer) StagingRegistry() string { return localRegistryURL }
