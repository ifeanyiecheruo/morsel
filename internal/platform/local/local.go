// Package local provides the LocalPlatform implementation of platform.Platform.
// It has no cloud dependencies and runs entirely on the developer's machine.
package local

import (
	"context"

	"github.com/ifeanyiecheruo/morsel/internal/platform"
	"github.com/ifeanyiecheruo/morsel/internal/secrets"
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

func (lp *LocalPlatform) Bootstrap() platform.Bootstrapper {
	return &localBootstrapper{secretMgr: lp.secretMgr}
}
func (lp *LocalPlatform) Deploy() platform.Deployer     { return &localDeployer{secretMgr: lp.secretMgr} }
func (lp *LocalPlatform) Blobs() platform.BlobStore     { return &localBlobStore{} }
func (lp *LocalPlatform) Secrets() platform.SecretStore { return lp.store }
func (lp *LocalPlatform) Credentials() platform.CredentialProvider {
	return &localCredentialProvider{secretMgr: lp.secretMgr}
}
func (lp *LocalPlatform) DNS() platform.DNSProvider         { return &localDNSProvider{} }
func (lp *LocalPlatform) Certs() platform.CertProvider      { return &localCertProvider{} }
func (lp *LocalPlatform) Pricing() platform.PricingProvider { return &localPricingProvider{} }

// SeedDefaults installs the default operator principal if none have been
// configured yet. Called once on server startup via platform.Seeder.
func (lp *LocalPlatform) SeedDefaults(ctx context.Context) error {
	existing, err := lp.secretMgr.OperatorPrincipals(ctx)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	return lp.secretMgr.SetOperatorPrincipals(ctx, []string{"operator@example.com"})
}

var _ platform.Seeder = (*LocalPlatform)(nil)
