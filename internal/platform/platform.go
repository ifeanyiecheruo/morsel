// Package platform defines the server-facing platform interface and all
// supporting types. The exported public platform package re-exports a
// CLI-facing subset (CliPlatform) via type aliases.
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

	// ErrSecretNotFound is returned by SecretStore.Get when the named secret does not exist.
	ErrSecretNotFound = errors.New("secret not found")

	// ErrPrincipalNotAuthorized is returned by CredentialProvider.ValidateOperatorToken
	// when the identity is not an authorised operator. Callers map this to 401.
	ErrPrincipalNotAuthorized = errors.New("principal not authorized")
)

// Seeder is an optional interface implemented by platforms that need to
// install default configuration on first startup.
type Seeder interface {
	SeedDefaults(ctx context.Context) error
}

// Platform is the full server-facing interface consumed by the REST API server.
type Platform interface {
	Bootstrap() Bootstrapper
	Deploy() Deployer
	Blobs() BlobStore
	Secrets() SecretStore
	Credentials() CredentialProvider
	DNS() DNSProvider
	Certs() CertProvider
	Pricing() PricingProvider
}

// CliPlatform is the CLI-facing subset. Re-exported from the public platform
// package as platform.Platform so CLI commands only see Bootstrap and Deploy.
type CliPlatform interface {
	Bootstrap() Bootstrapper
	Deploy() Deployer
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

// BlobStore is the object storage interface.
type BlobStore interface {
	Get(ctx context.Context, namespace, key string) (io.ReadCloser, error)
	Put(ctx context.Context, namespace, key string, body io.Reader, size int64) error
	List(ctx context.Context, namespace, prefix, cursor string, limit int) (keys []string, nextCursor string, err error)
	Delete(ctx context.Context, namespace, key string) error
	Usage(ctx context.Context, namespace string) (int64, error)
}

// SecretStore is the low-level platform secret read/write interface.
// Use secrets.Manager for strongly-typed access in business logic.
type SecretStore interface {
	Get(ctx context.Context, name string) ([]byte, error)
	Set(ctx context.Context, name string, value []byte) error
	Delete(ctx context.Context, name string) error
}

// CredentialProvider handles service identity and operator authentication.
type CredentialProvider interface {
	// AmbientToken returns a short-lived platform access token for ambient service identity.
	AmbientToken(ctx context.Context) (string, error)

	// DeployToken generates a deploy identity token for the current repo.
	DeployToken(ctx context.Context) (string, error)

	// ValidateDeployToken validates a deploy identity token and returns the repo slug.
	ValidateDeployToken(ctx context.Context, token string) (slug string, err error)

	// ValidateOperatorToken validates an operator login and returns the subject.
	// Returns ErrPrincipalNotAuthorized for any auth failure; other errors are infrastructure failures.
	ValidateOperatorToken(ctx context.Context, username, password string) (subject string, err error)
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
