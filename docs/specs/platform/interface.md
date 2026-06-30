Up: [Index](../README.md) · Prev: [Admin UI](../components/admin-ui.md) · Next: [GCP Platform](gcp.md)

---

# Platform — Interface

> **Status:** Draft · **Date:** May 2026

---

## Overview

All infrastructure concerns in Morsel are accessed through a single `Platform` interface. Business logic never imports a cloud SDK directly. Platform-specific implementations live in isolated packages and are injected at startup via dependency injection.

This means:
- The control plane, bootstrap binary, blob service, and queue service all depend on `Platform`, not on GCP
- Adding a new cloud target means implementing the interface in a new package — no business logic changes
- Testing uses a `LocalPlatform` or a mock implementation — no cloud account required

---

## Interface Definition

```go
// platform/platform.go — no cloud SDK imports

type Platform interface {
    Bootstrap()   Bootstrapper
    Deploy()      Deployer
    Blobs()       BlobStore
    Queues()      QueueStore
    Secrets()     SecretStore
    Credentials() CredentialProvider
    DNS()         DNSProvider
    Certs()       CertProvider
    Pricing()     PricingProvider
}
```

---

## Supporting Types

```go
type Prompt struct {
    Key         string
    Label       string
    Description string
    Default     string
    Required    bool
    Secret      bool               // mask input in terminal
    Validate    func(string) error // optional inline validation
}

type Plan struct {
    Summary   string     // human-readable description of what will be created
    Resources []Resource // list of resources with estimated costs
}

type Resource struct {
    Name            string
    Description     string
    EstimatedCostMo float64
}

type DeployCredentials struct {
    MorselToken  string // exchanged from platform-specific identity token
    RegistryAuth string // base64-encoded docker auth config for registry push
}

type Prices struct {
    ComputeCPUPerCoreHour float64
    ComputeMemPerGBHour   float64
    StoragePerGBMonth     float64
    RegistryPerGBMonth    float64
    FetchedAt             time.Time
}
```

---

## Sub-Interfaces

### Bootstrapper

```go
type Bootstrapper interface {
    // Prompts returns the wizard questions the platform needs answered before provisioning.
    Prompts() []Prompt

    // Plan returns a human-readable summary of what will be created and estimated costs,
    // given the operator's answers. Called after Prompts() for operator confirmation.
    Plan(answers map[string]string) Plan

    // Provision performs full platform provisioning idempotently. Safe to re-run on upgrade.
    Provision(ctx context.Context, answers map[string]string) error
}
```

### Deployer

```go
type Deployer interface {
    // Credentials returns the Morsel token and registry auth needed for a deploy.
    // In CI, exchanges the platform identity token for deploy credentials.
    // Locally, reads from the stored CLI profile.
    Credentials(ctx context.Context) (DeployCredentials, error)

    // StagingRegistry returns the staging registry URL for image push.
    StagingRegistry() string
}
```

### BlobStore

```go
type BlobStore interface {
    Get(ctx context.Context, namespace, key string) (io.ReadCloser, error)
    Put(ctx context.Context, namespace, key string, body io.Reader, size int64) error
    List(ctx context.Context, namespace, prefix, cursor string, limit int) ([]string, string, error)
    Delete(ctx context.Context, namespace, key string) error
    Usage(ctx context.Context, namespace string) (int64, error)
}
```

### QueueStore

The backing store for the queue service. `namespace` is the Kubernetes namespace of the app that owns the queues (e.g. `alice-myrepo--worker`). `LocalQueueStore` stores one SQLite file per queue; `GCPQueueStore` would back this with Cloud Tasks or Firestore.

```go
type QueueStore interface {
    CreateQueue(ctx context.Context, namespace, name string) error
    DeleteQueue(ctx context.Context, namespace, name string) error
    ListQueues(ctx context.Context, namespace string, idleAfter time.Duration) ([]QueueInfo, error)
    Enqueue(ctx context.Context, namespace, name string, body []byte, senderID, ownerID string) error
    Dequeue(ctx context.Context, namespace, name string, visibilityTimeout time.Duration) (*QueueMessage, error)
    Ack(ctx context.Context, namespace, name, id string) error
    Depth(ctx context.Context, namespace, name string) (int64, error)
    SetQuota(ctx context.Context, namespace string, limitBytes int64) error
    Usage(ctx context.Context, namespace string) (int64, error)
    IdleStatus(ctx context.Context, namespace string, idleAfter time.Duration) ([]QueueInfo, error)
}

type QueueInfo struct {
    Name  string
    Depth int64
    Idle  bool
}

type QueueMessage struct {
    ID         string
    Body       []byte
    EnqueuedAt time.Time
}
```

`senderID` and `ownerID` in `Enqueue` are Kubernetes namespace strings. When they differ the enqueue is treated as external and updates `last_external_enqueue_at`, which drives idle detection.

`IdleStatus` is the internal endpoint consumed by the control plane hibernation watcher.

### SecretStore

```go
type SecretStore interface {
    Get(ctx context.Context, name string) ([]byte, error)
    Set(ctx context.Context, name string, value []byte) error
    Delete(ctx context.Context, name string) error
}
```

### CredentialProvider

```go
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

    // ValidateOperatorToken validates the operator identity from the incoming request
    // and returns the operator subject (e.g. "alice@example.com").
    // Called server-side by the POST /api/token/oidc handler, which passes the raw
    // request so each implementation can read from wherever it expects its credential:
    // the request body (LocalPlatform) or a platform-injected header (GCPPlatform).
    ValidateOperatorToken(ctx context.Context, r *http.Request) (subject string, err error)
}
```

### DNSProvider

```go
type DNSProvider interface {
    CreateRecord(ctx context.Context, domain, name, recordType, value string, ttl int) error
    DeleteRecord(ctx context.Context, domain, name, recordType string) error
    RecordExists(ctx context.Context, domain, name, recordType string) (bool, error)
}
```

### CertProvider

```go
type CertProvider interface {
    // Provision obtains a TLS certificate for the given domain.
    // Uses ACME DNS-01 challenge via the platform's DNSProvider.
    Provision(ctx context.Context, domain string) (*tls.Certificate, error)

    // Renew renews an existing certificate if it expires within the given threshold.
    Renew(ctx context.Context, domain string, renewBefore time.Duration) (*tls.Certificate, error)
}
```

### PricingProvider

```go
type PricingProvider interface {
    // Prices fetches current list prices from the platform's pricing API.
    // Results are used for cost estimation and stored as daily snapshots by the control plane.
    Prices(ctx context.Context) (Prices, error)
}
```

---

## Method Responsibilities

| Accessor / Interface | Responsibility |
|---|---|
| `Bootstrap()` | Returns the `Bootstrapper` for the platform. Used only by the CLI during `service bootstrap`. |
| `Bootstrapper.Prompts()` | Returns the wizard questions the platform needs answered before provisioning. |
| `Bootstrapper.Plan()` | Returns a human-readable plan of what will be created, with estimated costs. Shown before operator confirms. |
| `Bootstrapper.Provision()` | Performs full platform provisioning idempotently. Safe to re-run on upgrade. |
| `Deploy()` | Returns the `Deployer` for the platform. Used by the CLI during `app deploy`. |
| `Deployer.Credentials()` | Returns the Morsel token and registry auth needed for a deploy. In CI: exchanges the platform identity token. Locally: reads from stored profile. |
| `Deployer.StagingRegistry()` | Returns the staging registry URL for image push. |
| `Blobs()` | Returns the `BlobStore`. Object storage — get, put, list, delete, usage. |
| `Queues()` | Returns the `QueueStore`. Message queue backing store — create/delete queues, enqueue/dequeue/ack messages, quota, idle status. Used by the queue service and control plane queue lifecycle management. |
| `Secrets()` | Returns the `SecretStore`. Platform secret read and write. |
| `Credentials()` | Returns the `CredentialProvider`. Service authentication token for platform API calls. |
| `CredentialProvider.ValidateOperatorToken()` | Validates the operator identity from an incoming `POST /api/token/oidc` request. Reads credential from wherever the platform expects it: request body (Local) or injected header (GCP). Returns the operator subject. |
| `DNS()` | Returns the `DNSProvider`. DNS record create, delete, and existence check. |
| `Certs()` | Returns the `CertProvider`. TLS certificate provisioning and renewal via ACME DNS-01. |
| `Pricing()` | Returns the `PricingProvider`. Fetches current list prices from the platform pricing API. |

---

## Package Structure

```
morsel/
  platform/
    platform.go        # Interface definitions only — zero cloud SDK imports
    gcp/
      platform.go      # GCPPlatform — imports GCP Go SDKs
      bootstrap.go
      deploy.go
      blobs.go
      secrets.go
      credentials.go
      dns_clouddns.go
      dns_cloudflare.go
      certs.go
      pricing.go
    local/
      platform.go      # LocalPlatform — no external dependencies
      bootstrap.go
      deploy.go
      blobs.go
      queues.go
      secrets.go
      credentials.go
      dns.go
      certs.go
      pricing.go       # no-op — returns zeros; LocalPlatform has no billing
    aws/               # Future — AWSPlatform stub
    azure/             # Future — AzurePlatform stub
```

The `platform/platform.go` file imports nothing from cloud SDKs. Each implementation package imports only what it needs. The main binary and all services import only `platform/platform.go` for types, plus the concrete implementation they are configured to use.

---

## Platform Selection

The active platform is determined from the profile config at startup:

```json
{ "platform": "gcp" }
{ "platform": "local" }
```

`--platform` is required on `morsel service bootstrap`. The value is written to the profile and used on all subsequent commands. `gcp` and `local` are the supported values; additional platforms can be added without changes to business logic.

No business logic code path branches on the platform type. All platform-specific behaviour is encapsulated in the implementation packages.

---

## Implementations

| Implementation | Status | Description |
|---|---|---|
| `GCPPlatform` | Production | GCS, Secret Manager, Workload Identity, Cloud DNS / Cloudflare, ACME/Let's Encrypt |
| `LocalPlatform` | Production | Filesystem blobs, local JSON secrets, no-op credentials, `*.morsel.localhost`, self-signed certs |
| `AWSPlatform` | Future | S3, Secrets Manager, IRSA, Route 53, ACME/Let's Encrypt |
| `AzurePlatform` | Future | Azure Blob Storage, Key Vault, Azure Workload Identity, Azure DNS, ACME/Let's Encrypt |

See [platform/gcp.md](gcp.md) and [platform/local.md](local.md) for implementation details.

---

## Adding a New Platform

Implementing a new cloud platform requires:

1. Create `platform/{cloud}/platform.go` implementing the `Platform` interface
2. Implement `Bootstrapper` — wizard prompts, plan, and idempotent provisioning
3. Implement `Deployer` — credential exchange and staging registry URL
4. Implement `BlobStore`, `SecretStore`, `CredentialProvider`, `DNSProvider`, `CertProvider` for the target cloud
5. Implement `PricingProvider` — fetch list prices from the cloud's billing/pricing API
6. Wire the new platform into the platform selection logic in the main binary
7. Add a profile JSON entry (`"platform": "{cloud}"`)

No changes to business logic, API handlers, or the bootstrap wizard itself. The wizard presents whatever prompts the platform returns and passes the answers to `Bootstrap()`.

The admin UI auth mechanism (currently GCP IAP) is the most platform-specific concern not covered by the `Platform` interface. Cloudflare Access is a cloud-agnostic alternative worth considering if portability becomes a priority.

---

Up: [Index](../README.md) · Prev: [Admin UI](../components/admin-ui.md) · Next: [GCP Platform](gcp.md)
