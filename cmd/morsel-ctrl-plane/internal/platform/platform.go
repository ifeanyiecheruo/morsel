// Package platform defines the control-plane-facing platform interface and all
// supporting types. It is consumed by the REST API server and the local platform
// implementation; the CLI has its own bootstrap-only interface in cmd/morsel.
package platform

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"time"
)

var (
	// ErrNotImplemented is returned by stub methods not yet built.
	ErrNotImplemented = errors.New("not implemented")

	// ErrSecretNotFound is returned when a named secret does not exist.
	ErrSecretNotFound = errors.New("secret not found")

	// ErrPrincipalNotAuthorized is returned by Secrets.ValidateOperatorCredential
	// when the identity is not an authorised operator. Callers map this to 401.
	ErrPrincipalNotAuthorized = errors.New("principal not authorized")
)

// Seeder is an optional interface implemented by platforms that need to
// install default configuration on first startup.
type Seeder interface {
	SeedDefaults(ctx context.Context) error
}

// Platform is the full control-plane-facing interface consumed by the REST API server.
type Platform interface {
	Namespace() string
	BaseDomain() string
	Deploy() Deployer
	Blobs() BlobStore
	Secrets() Secrets
	Tokens() Tokens
	DNS() DNSProvider
	Certs() CertProvider
	Pricing() PricingProvider
}

// Secrets manages raw key material for the platform. Each key type exposes a
// consistent Get / Ensure / Rotate / Delete model:
//
//   - GetXXXKeys returns the current key array (nil when not provisioned).
//   - EnsureXXXKey provisions a key if the array is empty and returns the array.
//   - RotateXXXKey removes the oldest key (index 0), appends a new one, and
//     returns the updated array. The length stays constant after the first rotation.
//   - DeleteXXXKey removes the entire key array from storage.
type Secrets interface {
	// Signing key — used to sign and verify Morsel API JWTs.
	GetSigningKeys(ctx context.Context) ([][]byte, error)
	EnsureSigningKey(ctx context.Context) ([][]byte, error)
	RotateSigningKey(ctx context.Context) ([][]byte, error)
	DeleteSigningKey(ctx context.Context) error

	// Deploy signing key — HMAC key for deploy identity JWTs.
	GetDeploySigningKeys(ctx context.Context) ([][]byte, error)
	EnsureDeploySigningKey(ctx context.Context) ([][]byte, error)
	RotateDeploySigningKey(ctx context.Context) ([][]byte, error)
	DeleteDeploySigningKey(ctx context.Context) error

	// Migrate runs pending secret store migrations. Safe to call on every startup.
	Migrate(ctx context.Context) error
}

// Tokens handles token retrieval and creation. Token creation requires key material
// from Secrets, so this interface sits above it.
type Tokens interface {
	// GetAmbientToken returns a platform-issued short-lived access token for
	// ambient service identity. Returns ("", nil) on platforms that use implicit
	// credentials (e.g., Workload Identity).
	GetAmbientToken(ctx context.Context) (string, error)

	// CreateDeployToken issues a signed deploy identity token for the given repository slug.
	CreateDeployToken(ctx context.Context, repository string) (string, error)

	// VerifyDeployToken validates a deploy identity token and returns the repo slug.
	VerifyDeployToken(ctx context.Context, token string) (slug string, err error)

	// ValidateOperatorCredential checks an operator login and returns the subject.
	// Returns ErrPrincipalNotAuthorized for any auth failure.
	ValidateOperatorCredential(ctx context.Context, username, password string) (subject string, err error)
}

// Deployer provides registry and credential information needed for a deploy run.
type Deployer interface {
	// Credentials returns the Morsel token and registry auth needed for a deploy.
	Credentials(ctx context.Context) (DeployCredentials, error)

	// StagingRegistry returns the staging registry URL for image push.
	StagingRegistry() string
}

// BlobStore is the object storage interface.
type BlobStore interface {
	Get(ctx context.Context, namespace, key string) (io.ReadCloser, error)
	Put(ctx context.Context, namespace, key string, body io.Reader, size int64) error
	List(ctx context.Context, namespace, prefix, cursor string, limit int) (keys []string, nextCursor string, err error)
	Delete(ctx context.Context, namespace, key string) error
	Usage(ctx context.Context, namespace string) (int64, error)
}

// DNSProvider manages DNS records for app subdomains.
type DNSProvider interface {
	CreateRecord(ctx context.Context, domain, name, recordType, value string, ttl int) error
	DeleteRecord(ctx context.Context, domain, name, recordType string) error
	RecordExists(ctx context.Context, domain, name, recordType string) (bool, error)
}

// CertProvider provisions and renews TLS certificates via ACME DNS-01.
type CertProvider interface {
	Provision(ctx context.Context, domain string) (*tls.Certificate, error)
	Renew(ctx context.Context, domain string, renewBefore time.Duration) (*tls.Certificate, error)
}

// PricingProvider fetches current list prices from the platform billing API.
type PricingProvider interface {
	Prices(ctx context.Context) (Prices, error)
}

// DeployCredentials is the result of a successful deploy token exchange.
type DeployCredentials struct {
	MorselToken  string // 10-min developer access token scoped to the repo slug
	RegistryAuth string // base64-encoded docker auth config for staging registry push
}

// Prices holds the current list prices used for cost estimation.
type Prices struct {
	ComputeCPUPerCoreHour float64
	ComputeMemPerGBHour   float64
	StoragePerGBMonth     float64
	RegistryPerGBMonth    float64
	FetchedAt             time.Time
}
