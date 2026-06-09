package platform

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"time"
)

// ErrNotImplemented is returned by stub methods on platform implementations
// that have not yet been built.
var ErrNotImplemented = errors.New("not implemented")

// Platform is the single dependency boundary between Morsel business logic
// and all cloud/infrastructure concerns. No business logic file imports a
// cloud SDK — only this package.
type Platform interface {
	Bootstrap()   Bootstrapper
	Deploy()      Deployer
	Blobs()       BlobStore
	Secrets()     SecretStore
	Credentials() CredentialProvider
	DNS()         DNSProvider
	Certs()       CertProvider
	Pricing()     PricingProvider
}

// Bootstrapper provisions all platform resources needed to run Morsel.
type Bootstrapper interface {
	// Prompts returns the wizard questions the platform needs answered before provisioning.
	Prompts() []Prompt

	// Plan describes what will be created and estimated costs given the operator's answers.
	Plan(answers map[string]string) Plan

	// Provision performs full platform provisioning idempotently. Safe to re-run on upgrade.
	Provision(ctx context.Context, answers map[string]string) error
}

// Deployer provides registry and credential information needed for a deploy run.
type Deployer interface {
	// Credentials returns the Morsel token and registry auth needed for a deploy.
	Credentials(ctx context.Context) (DeployCredentials, error)

	// StagingRegistry returns the staging registry URL for image push.
	StagingRegistry() string
}

// BlobStore is the object storage interface. Key namespacing is applied by the
// blob service before calling these methods — the implementation does not namespace.
type BlobStore interface {
	Get(ctx context.Context, namespace, key string) (io.ReadCloser, error)
	Put(ctx context.Context, namespace, key string, body io.Reader, size int64) error
	List(ctx context.Context, namespace, prefix, cursor string, limit int) (keys []string, nextCursor string, err error)
	Delete(ctx context.Context, namespace, key string) error
	Usage(ctx context.Context, namespace string) (int64, error)
}

// SecretStore is the platform secret read/write interface.
type SecretStore interface {
	Get(ctx context.Context, name string) ([]byte, error)
	Set(ctx context.Context, name string, value []byte) error
	Delete(ctx context.Context, name string) error
}

// CredentialProvider handles both ambient service identity (AmbientToken) and the
// deploy identity token lifecycle (DeployToken / ValidateDeployToken).
type CredentialProvider interface {
	// AmbientToken returns a short-lived platform access token for ambient service identity.
	// On GCPPlatform this is a Workload Identity token. On LocalPlatform it returns "".
	AmbientToken(ctx context.Context) (string, error)

	// DeployToken generates a deploy identity token for the current repo.
	// Called client-side (morsel app deploy) before exchanging at POST /api/token/deploy.
	DeployToken(ctx context.Context) (string, error)

	// ValidateDeployToken validates a deploy identity token and returns the repo slug.
	// Called server-side by the POST /api/token/deploy handler.
	ValidateDeployToken(ctx context.Context, token string) (slug string, err error)
}

// DNSProvider manages DNS records for app subdomains.
type DNSProvider interface {
	CreateRecord(ctx context.Context, domain, name, recordType, value string, ttl int) error
	DeleteRecord(ctx context.Context, domain, name, recordType string) error
	RecordExists(ctx context.Context, domain, name, recordType string) (bool, error)
}

// CertProvider provisions and renews TLS certificates via ACME DNS-01.
type CertProvider interface {
	// Provision obtains a TLS certificate for the given domain.
	Provision(ctx context.Context, domain string) (*tls.Certificate, error)

	// Renew renews an existing certificate if it expires within renewBefore.
	Renew(ctx context.Context, domain string, renewBefore time.Duration) (*tls.Certificate, error)
}

// PricingProvider fetches current list prices from the platform billing API.
type PricingProvider interface {
	Prices(ctx context.Context) (Prices, error)
}

// Prompt is a single wizard question presented to the operator during bootstrap.
type Prompt struct {
	Key         string
	Label       string
	Description string
	Default     string
	Required    bool
	Secret      bool               // mask input in terminal
	Validate    func(string) error // optional inline validation; not serialised
}

// Plan describes what will be created and estimated costs before operator confirmation.
type Plan struct {
	Summary   string
	Resources []Resource
}

// Resource is one provisioned item in a bootstrap Plan.
type Resource struct {
	Name            string
	Description     string
	EstimatedCostMo float64
}

// DeployCredentials is the result of a successful deploy token exchange.
// Returned by POST /api/token/deploy; used by morsel app deploy for all
// subsequent API calls and image push.
type DeployCredentials struct {
	MorselToken  string // 10-min developer access token scoped to the repo slug
	RegistryAuth string // base64-encoded docker auth config for staging registry push
}

// Prices holds the current list prices used for cost estimation.
// Fetched daily from the platform pricing API and stored as immutable snapshots.
type Prices struct {
	ComputeCPUPerCoreHour float64
	ComputeMemPerGBHour   float64
	StoragePerGBMonth     float64
	RegistryPerGBMonth    float64
	FetchedAt             time.Time
}
