# Feature 19 — GCPPlatform

_Delivers: full production deployment on GCP; operator runs `morsel service bootstrap --platform gcp`._

**Direct dependencies:** [F18](018-feature-admin-ui.md) — all LocalPlatform features must be stable first.

## Tasks

- [ ] `platform/gcp/platform.go` — `GCPPlatform` struct; all `Platform` methods compile
- [ ] GCP OAuth browser flow — localhost callback listener; token held in memory only
- [ ] `GCPPlatform.Bootstrap().Prompts()` — project ID, region, base domain, DNS provider (Cloud DNS / Cloudflare)
- [ ] `GCPPlatform.Bootstrap().Plan()` — list GCP resources with estimated monthly costs
- [ ] Preflight checks — billing active, required APIs enabled, IAM permissions, compute quota, DNS zone
- [ ] `GCPPlatform.Bootstrap().Provision()` — provision in dependency order: GCS state bucket → VPC → GKE Autopilot → Artifact Registry → WIF → IAM bindings → Secret Manager → control plane install → Admin UI bundle to GCS → GKE Gateway classes → IAP → smoke test
- [ ] `GCPPlatform.Blobs()` — GCS implementation (`morsel-blobs-{project-id}` bucket)
- [ ] `GCPPlatform.Secrets()` — Secret Manager implementation
- [ ] `GCPPlatform.Credentials()` — Workload Identity metadata server token
- [ ] `GCPPlatform.DNS()` — Cloud DNS implementation
- [ ] `GCPPlatform.DNS()` — Cloudflare implementation (alternate; token from SecretStore)
- [ ] `GCPPlatform.Certs()` — ACME DNS-01 via Cloud DNS or Cloudflare
- [ ] `GCPPlatform.Pricing()` — Cloud Billing Catalog API (`cloudbilling.googleapis.com`)
- [ ] `GCPPlatform.DeployToken()` — obtain GitHub OIDC token from GitHub Actions environment (`ACTIONS_ID_TOKEN_REQUEST_URL`); fails if `GITHUB_ACTIONS` not set
- [ ] `GCPPlatform.ValidateDeployToken()` — fetch GitHub JWKS (cached), validate JWT signature, extract `repository` claim, return `org/repo` slug; generate short-lived Artifact Registry staging push credentials via WIF and attach to token response
- [ ] `GCPPlatform.ValidateOperatorToken()` — verify `X-Goog-IAP-JWT-Assertion` header using Google's public JWKS (cached); return operator email as subject; `POST /api/token/oidc` already handles token issuance
- [ ] Admin UI authentication via IAP — IAP injects identity header; control plane verifies and exchanges for Morsel token
- [ ] Smoke test on bootstrap completion — deploy a test app, verify it is reachable, clean up
