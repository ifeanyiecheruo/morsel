# Feature 01 — Repository Foundation

_Delivers: buildable binary, platform interface, project structure._

**Direct dependencies:** none

## Tasks

- [x] Initialise Go module; define top-level directory layout: `cmd/`, `internal/`, `platform/`
- [x] `platform/platform.go` — all interfaces and supporting types exactly as specced (`Platform`, `Bootstrapper`, `Deployer`, `BlobStore`, `SecretStore`, `CredentialProvider` with `DeployToken()` and `ValidateDeployToken()`, `DNSProvider`, `CertProvider`, `PricingProvider`, `Prices`, `Prompt`, `Plan`, `Resource`, `DeployCredentials`)
- [x] `platform/local/local.go` — `LocalPlatform` struct implementing `Platform`; every method compiles but returns stubs or `ErrNotImplemented`
- [x] Platform selection and DI wiring in `cmd/morsel/main.go` (`--platform` flag reads profile JSON, constructs the right implementation)
- [x] `Makefile` with `build`, `test`, `lint`, `run` targets
