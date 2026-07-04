// Package local provides the LocalPlatform implementation of platform.Platform.
// It has no cloud dependencies and runs entirely on the developer's machine.
package local

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/platform"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/store"
	"github.com/ifeanyiecheruo/morsel/internal/selfcert"
)

const saNamespacePath = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

// Name identifies this platform implementation.
func (lp *LocalPlatform) Name() string { return "local" }

// BaseDomain returns the base domain for app URLs on the local platform.
func (lp *LocalPlatform) BaseDomain() string { return selfcert.LocalBaseDomain }

// Namespace returns the Kubernetes namespace this service is running in,
// read from the service-account projection that Kubernetes injects into every pod.
func (lp *LocalPlatform) Namespace() string { return controlPlaneNamespace() }

// controlPlaneNamespace reads the pod's namespace from the service-account
// projection when running in-cluster, and falls back to "morsel" otherwise.
func controlPlaneNamespace() string {
	data, err := os.ReadFile(saNamespacePath)
	if err == nil {
		return strings.TrimSpace(string(data))
	}
	return "morsel"
}

// LocalPlatform implements platform.Platform with no cloud dependencies.
type LocalPlatform struct {
	secrets *localSecrets
	tok     *localTokens
	store   *store.Store
}

// New creates a LocalPlatform. The kube client for secret storage is built
// lazily on first secret access using the in-cluster service-account config;
// the morsel-api always runs in-cluster so this never fails in production.
func New(s *store.Store) *LocalPlatform {
	return NewWithSecretStore(s, newKubeSecretStore(controlPlaneNamespace()))
}

// NewWithSecretStore creates a LocalPlatform with an explicit SecretStore backend.
// Intended for tests that need to run without a live Kubernetes cluster.
func NewWithSecretStore(s *store.Store, ss SecretStore) *LocalPlatform {
	sec := &localSecrets{store: ss}
	tok := &localTokens{secrets: sec, store: s}
	return &LocalPlatform{secrets: sec, tok: tok, store: s}
}

// localDataDir returns the directory used to store local platform state.
// Inside a pod it reads MORSEL_LOCAL_DATA_DIR so the bootstrap code can
// mount a shared volume at a known path without relying on the home directory.
func localDataDir() string {
	if dir := os.Getenv("MORSEL_LOCAL_DATA_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".morsel", "local")
}

// DBPath returns the path to the SQLite database for the local platform.
func DBPath() string {
	return filepath.Join(localDataDir(), "morsel.db")
}

func (lp *LocalPlatform) Deploy() platform.AppDeployer      { return &localAppDeployer{} }
func (lp *LocalPlatform) Blobs() platform.BlobStore         { return &localBlobStore{} }
func (lp *LocalPlatform) Secrets() platform.Secrets         { return lp.secrets }
func (lp *LocalPlatform) Tokens() platform.Tokens           { return lp.tok }
func (lp *LocalPlatform) DNS() platform.DNSProvider         { return &localDNSProvider{} }
func (lp *LocalPlatform) Certs() platform.CertProvider      { return &localCertProvider{} }
func (lp *LocalPlatform) Pricing() platform.PricingProvider { return &localPricingProvider{} }

// SeedDefaults is called once on server startup via platform.Seeder.
// The initial operator is provisioned through 'morsel service deploy', so
// this is a no-op for the local platform.
func (lp *LocalPlatform) SeedDefaults(_ context.Context) error {
	return nil
}

var _ platform.Platform = (*LocalPlatform)(nil)
var _ platform.Seeder = (*LocalPlatform)(nil)
