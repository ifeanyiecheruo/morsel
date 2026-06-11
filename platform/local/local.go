// Package local provides the LocalPlatform implementation of platform.Platform.
// It has no cloud dependencies and runs entirely on the developer's machine.
package local

import (
	"github.com/ifeanyiecheruo/morsel/internal/secrets"
	"github.com/ifeanyiecheruo/morsel/platform"
)

// LocalPlatform implements platform.Platform with no cloud dependencies.
type LocalPlatform struct {
	store     *localSecretStore
	secretMgr *secrets.Manager
}

func New() *LocalPlatform {
	store := newLocalSecretStore()
	return &LocalPlatform{
		store:     store,
		secretMgr: secrets.New(store),
	}
}

func (lp *LocalPlatform) Bootstrap() platform.Bootstrapper { return &localBootstrapper{} }
func (lp *LocalPlatform) Deploy() platform.Deployer        { return &localDeployer{} }
func (lp *LocalPlatform) Blobs() platform.BlobStore        { return &localBlobStore{} }
func (lp *LocalPlatform) Secrets() platform.SecretStore    { return lp.store }
func (lp *LocalPlatform) Credentials() platform.CredentialProvider {
	return &localCredentialProvider{}
}
func (lp *LocalPlatform) DNS() platform.DNSProvider         { return &localDNSProvider{} }
func (lp *LocalPlatform) Certs() platform.CertProvider      { return &localCertProvider{} }
func (lp *LocalPlatform) Pricing() platform.PricingProvider { return &localPricingProvider{} }
