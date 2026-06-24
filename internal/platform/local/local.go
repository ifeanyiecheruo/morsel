// Package local provides the LocalPlatform implementation of platform.Platform.
// It has no cloud dependencies and runs entirely on the developer's machine.
package local

import (
	"context"
	"os"
	"path/filepath"

	"github.com/ifeanyiecheruo/morsel/internal/platform"
	"github.com/ifeanyiecheruo/morsel/internal/store"
)

// LocalPlatform implements platform.Platform with no cloud dependencies.
type LocalPlatform struct {
	secrets *localSecrets
	store   *store.Store
}

func New(s *store.Store) *LocalPlatform {
	fileStore := newLocalFileSecretStore()
	sec := &localSecrets{fileStore: fileStore, store: s}
	return &LocalPlatform{secrets: sec, store: s}
}

// DBPath returns the path to the SQLite database for the local platform.
func DBPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".morsel", "local", "morsel.db")
}

func (lp *LocalPlatform) Bootstrap() platform.Bootstrapper {
	return &localBootstrapper{secrets: lp.secrets}
}
func (lp *LocalPlatform) Deploy() platform.Deployer         { return &localDeployer{secrets: lp.secrets} }
func (lp *LocalPlatform) Blobs() platform.BlobStore         { return &localBlobStore{} }
func (lp *LocalPlatform) Secrets() platform.Secrets         { return lp.secrets }
func (lp *LocalPlatform) DNS() platform.DNSProvider         { return &localDNSProvider{} }
func (lp *LocalPlatform) Certs() platform.CertProvider      { return &localCertProvider{} }
func (lp *LocalPlatform) Pricing() platform.PricingProvider { return &localPricingProvider{} }

// SeedDefaults installs the default operator principal if none have been
// configured yet. Called once on server startup via platform.Seeder.
func (lp *LocalPlatform) SeedDefaults(ctx context.Context) error {
	if lp.store == nil {
		return nil
	}
	existing, err := lp.store.ListPrincipals(ctx)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	return lp.store.AddPrincipal(ctx, "operator@example.com")
}

var _ platform.Seeder = (*LocalPlatform)(nil)
