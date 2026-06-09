# Implementation Plan

## Approach

**Local-first, platform-abstracted.** All features are built against `LocalPlatform` first. `GCPPlatform` is a separate later feature — a complete Morsel instance runs with no cloud account. This keeps the inner development loop fast and validates the platform abstraction.

**Visible progress first.** Each feature delivers something a user can observe: a running endpoint, a working CLI command, an app that deploys. Stubs and fake implementations are acceptable whenever they unblock the end-to-end visible path.

**Task sizing.** Each task is one PR — a focused, reviewable unit of work with passing tests. Tasks within a feature are ordered by dependency.

**Tech stack.** Go (Morsel API, CLI, Blob Service, Queue Service), React + TypeScript (Admin UI), SQLite (Morsel API state), PostgreSQL (Database Service + Queue Service backing store), Kubernetes Gateway API (ingress).

---

## Feature 1 — Repository Foundation

_Delivers: buildable binary, platform interface, project structure._

- [x] Initialise Go module; define top-level directory layout: `cmd/`, `internal/`, `platform/`
- [x] `platform/platform.go` — all interfaces and supporting types exactly as specced (`Platform`, `Bootstrapper`, `Deployer`, `BlobStore`, `SecretStore`, `CredentialProvider` with `DeployToken()` and `ValidateDeployToken()`, `DNSProvider`, `CertProvider`, `PricingProvider`, `Prices`, `Prompt`, `Plan`, `Resource`, `DeployCredentials`)
- [x] `platform/local/local.go` — `LocalPlatform` struct implementing `Platform`; every method compiles but returns stubs or `ErrNotImplemented`
- [x] Platform selection and DI wiring in `cmd/morsel/main.go` (`--platform` flag reads profile JSON, constructs the right implementation)
- [x] `Makefile` with `build`, `test`, `lint`, `run` targets

---

## Feature 2 — Morsel API: HTTP Server Skeleton

_Delivers: running API binary; `curl /healthz` returns 200._

- [x] `cmd/morsel-api/main.go` — HTTP server with graceful shutdown on SIGTERM
- [x] Structured error middleware — all responses follow `{"error": "code", "message": "...", "details": {...}}` shape per `conventions/rest.md`
- [x] `GET /healthz` — returns `{"status": "ok"}`
- [ ] SQLite connection pool with WAL mode enabled
- [ ] Migration runner — versioned SQL files applied at startup; idempotent
- [ ] Initial schema: `repos`, `apps`, `operations` tables
- [ ] Structured request logging (method, path, status, latency)

---

## Feature 3 — Authentication

_Delivers: deploy identity token exchange works; operator can `morsel operator login` on LocalPlatform._

- [ ] JWT signing key: load from `SecretStore` at startup; generate and persist on first run if absent
- [ ] `POST /api/token/deploy` — call `Platform.ValidateDeployToken(token)` → repo slug; issue 10-min developer access token; no platform-specific logic in the handler
- [ ] `LocalPlatform.ValidateDeployToken()` — validate JWT signature against `local-deploy-signing-key`, extract `repository` claim, return `localhost/{dirname}` slug
- [ ] Auth middleware — verify JWT signature, parse role + repo claims, attach to request context
- [ ] `repos` ownership enforcement — 403 if token `repo` claim doesn't match `:slug`
- [ ] SQLite schema: `refresh_tokens` table
- [ ] `POST /api/token/refresh` — validate refresh token, issue new access token + rotated refresh token
- [ ] `POST /api/token/local-oidc` — LocalPlatform only; validate principal against local principal list, issue 15-min operator access token + 90-day refresh token
- [ ] `morsel operator login` CLI command — LocalPlatform path; POST to `/api/token/local-oidc`
- [ ] Profile file write (`~/.config/morsel/<profile>.profile.json`, mode 0600)
- [ ] CLI pre-command hook — load profile, silent refresh if access token expired, re-prompt login if refresh token expired

---

## Feature 4 — App Lint and Schema Validation

_Delivers: `morsel lint` works; developers catch schema errors before pushing._

- [ ] `morsel lint` command — find and validate all `*.morsel.json` files in `.morsel/`; validate against `morsel.schema.json`
- [ ] `morsel lint --staged` — validate only git-staged files; suitable for pre-commit hook
- [ ] `morsel lint --fix` — auto-correct safe issues (field ordering, whitespace)
- [ ] Lint checks: schema validity, valid `type` value, `schedule`+`timeout` present for `cronjob`, unique `name` within `.morsel/`, `permanent: true` removal warning, type-incompatible field warnings

---

## Feature 5 — App Lifecycle: API Layer

_Delivers: apps can be created, listed, and deleted through the API (SQLite only — no Kubernetes yet)._

- [ ] `POST /api/repos/:slug/sync` — upsert declared app list; mark apps absent from list as `deletion_pending`
- [ ] `POST /api/repos/:slug/apps` — upsert app record; validate fields; write SQLite row; return `202 Accepted` with operation location
- [ ] Async operation model — `operations` table; status polling at `GET /api/repos/:slug/apps/:name/operations/:id`
- [ ] `GET /api/repos/:slug/apps` — list apps with status
- [ ] `GET /api/repos/:slug/apps/:name` — app detail
- [ ] `GET /api/repos/:slug/apps/:name/status` — current runtime state (stub: `unknown` until K8s integration)
- [ ] `GET /api/repos/:slug/apps/:name/history` — deploy history from operations log
- [ ] `DELETE /api/repos/:slug/apps/:name` — soft-delete; begin 30-day grace period
- [ ] `GET /api/repos` — list repos (developer: own only; operator: all with `?all=true`)
- [ ] `GET /api/repos/:slug` — repo detail
- [ ] Namespace naming function: `{org-slug}-{repo-slug}--{app-name}` (slash → hyphen, preserve existing hyphens)

---

## Feature 6 — Kubernetes Manifest Apply

_Delivers: upserted apps actually run as pods in the cluster._

- [ ] `client-go` integration — kubeconfig detection (in-cluster via service account; local via `~/.kube/config`)
- [ ] Namespace create-or-ensure
- [ ] `ResourceQuota` + `LimitRange` apply (hardcoded `small` tier defaults until Feature 14)
- [ ] `NetworkPolicy` apply — allow ingress from load balancer and other app pods; deny cross-sidecar access
- [ ] `ServiceAccount` apply
- [ ] `Deployment` apply for `type: http` and `type: worker`
- [ ] `CronJob` apply for `type: cronjob`
- [ ] Rollout watch — poll deployment rollout via `client-go`; timeout from `health_check.timeout`
- [ ] Rollback on failed rollout — re-apply `last-healthy` image digest
- [ ] Image digest tracking in SQLite (`current`, `last-healthy` per app)
- [ ] `GET /api/repos/:slug/apps/:name/status` — reflect actual Kubernetes pod state

---

## Feature 7 — Bootstrap: LocalPlatform

_Delivers: `morsel service bootstrap --platform local` provisions a working local Morsel instance from scratch._

- [ ] `morsel service bootstrap` command — phase runner with progress output; idempotent
- [ ] `LocalPlatform.Secrets()` — filesystem implementation (`~/.morsel/local/secrets.json`)
- [ ] `LocalPlatform.Bootstrap().Prompts()` — collect base domain (default `morsel.localhost`), optional config
- [ ] `LocalPlatform.Bootstrap().Plan()` — describe what will be created; no estimated cost (LocalPlatform is free)
- [ ] `LocalPlatform.Bootstrap().Provision()` — install Morsel API, blob service, queue service, database service, local registry into cluster; write bootstrap config to secret store; generate and store `local-deploy-signing-key` in SecretStore
- [ ] Bootstrap config persistence — store wizard answers in local secret store; subsequent runs skip wizard
- [ ] `morsel service status` — health-check all platform components; report pass/fail per component
- [ ] `morsel service delete` — tear down all platform resources; requires explicit `--confirm` flag
- [ ] `morsel operator principal add/remove/list` — manage local operator principal list in secret store

---

## Feature 8 — LocalPlatform Deploy Path

_Delivers: `morsel app deploy` works end-to-end on LocalPlatform — push a change and see a running pod._

- [ ] Local container registry provisioned during bootstrap (`registry:2` Deployment in `morsel` namespace)
- [ ] Repo slug derivation — read git root directory name; prefix with `localhost/`; sanitize to slug format
- [ ] `LocalPlatform.DeployToken()` — generate JWT signed with `local-deploy-signing-key` with `{ "repository": "localhost/{dirname}", "ref": "...", "sha": "..." }`
- [ ] `LocalPlatform.Deploy().StagingRegistry()` — return in-cluster registry URL
- [ ] Staging handshake skipped on LocalPlatform — deployer pushes directly to canonical registry
- [ ] `morsel app deploy` unified path — call `Platform.DeployToken()`; exchange at `POST /api/token/deploy`; build images; push; call sync + deploy APIs; emit annotations when in CI
- [ ] Reference GitHub Actions workflow file (`.github/workflows/morsel-deploy.yml`)
- [ ] Deploy output formatting — per-app status lines, approval warnings, failure messages

---

## Feature 9 — Networking

_Delivers: HTTP apps get stable URLs with TLS; HTTPS works in-browser._

- [ ] Gateway class + Gateway resource provisioned during `LocalPlatform.Bootstrap().Provision()`
- [ ] `HTTPRoute` apply on deploy — route subdomain to app Service
- [ ] `HTTPRoute` routing decision — external Gateway class for `private: false`; internal for `private: true`
- [ ] `LocalPlatform.DNS()` — no-op; `*.morsel.localhost` resolves natively in modern browsers
- [ ] `LocalPlatform.Certs()` — generate self-signed wildcard cert for `*.morsel.localhost` at bootstrap; store in K8s Secret
- [ ] Cert storage helper — write `*tls.Certificate` to K8s Secret in app namespace
- [ ] Certificate renewal background goroutine — check expiry daily; renew 30 days before expiry
- [ ] Certificate alert in `GET /api/operator/status` — expiring soon, failed
- [ ] `HTTPRoute` delete on app deletion

---

## Feature 10 — Blob Service

_Delivers: apps can call `blob.morsel.internal` to store and retrieve objects._

- [ ] `cmd/blob-service/main.go` — HTTP server with graceful shutdown
- [ ] `TokenReview` caller identity resolution — map pod service account to `{repo-slug}/{app-name}`
- [ ] Key namespacing — prepend `{repo-slug}/{app-name}/` before every storage operation
- [ ] `LocalPlatform.Blobs()` — filesystem implementation; root at `~/.morsel/local/blobs/`
- [ ] SQLite quota tracking database (separate file from Morsel API)
- [ ] `GET /objects/{key}`, `PUT /objects/{key}`, `DELETE /objects/{key}` endpoints
- [ ] `GET /objects?prefix=&cursor=` — paginated key listing
- [ ] `PUT` quota check — reject writes that would exceed app's byte limit; return `429` with `blob_quota_exceeded`
- [ ] Internal quota-push endpoint — receive updated limits from Morsel API on tier change
- [ ] Blob service registration in `morsel-services` namespace during bootstrap

---

## Feature 11 — Database Service

_Delivers: apps connect to Postgres via `database.morsel.internal` with no credential management._

- [ ] Shared Postgres `StatefulSet` provisioned in `morsel-services` namespace during bootstrap
- [ ] Per-app database + user provisioning on first deploy with `persistence.database` declared
- [ ] `GRANT ALL` scoped to own database only
- [ ] Real credentials stored in K8s Secret in app namespace
- [ ] PGBouncer sidecar injection — add PGBouncer container to app pod spec; configure `morsel/morsel/morsel` → real credentials mapping
- [ ] `/etc/hosts` injection so `database.morsel.internal` resolves to `127.0.0.1` in app pod
- [ ] PGBouncer sidecar removal on hibernation (scale to 0 removes the pod + sidecar)
- [ ] Idempotent re-provisioning — re-deploy with same declaration does nothing

---

## Feature 12 — Queue Service

_Delivers: apps can enqueue and dequeue messages via `queue.morsel.internal`._

- [ ] `cmd/queue-service/main.go` — HTTP server
- [ ] `TokenReview` caller identity (same pattern as blob service)
- [ ] Postgres-backed queue tables with namespace prefix `{repo-slug}__{app-name}__{queue-name}`
- [ ] `PUT /queues/{name}` — create queue
- [ ] `DELETE /queues/{name}` — delete queue
- [ ] `GET /queues` — list caller's queues with idle status
- [ ] `POST /queues/{name}/messages` — enqueue; check storage quota
- [ ] `GET /queues/{name}/messages/next` — dequeue (at-most-once delivery)
- [ ] `DELETE /queues/{name}/messages/{id}` — explicit ack
- [ ] `GET /queues/{name}/depth` — message count
- [ ] Storage quota enforcement — track total bytes per app; reject enqueue at limit
- [ ] Internal quota-push endpoint — same pattern as blob service
- [ ] `GET /internal/queues/{namespace}/{app-name}` — return idle status for all queues owned by app; authenticated with `queue-internal-token`; used by Morsel API hibernation watcher

---

## Feature 13 — Hibernation

_Delivers: idle apps automatically scale to zero; first request after hibernation wakes the app transparently._

- [ ] Hibernation watcher goroutine — tick on configurable interval (`hibernation_check_interval`, default 60s)
- [ ] HTTP idle detection — read Envoy Gateway Prometheus metrics per app at each tick; compare request count to previous tick; update `last_active_at` in SQLite when count increases; hibernate when `now − last_active_at > idle_after`
- [ ] Worker idle detection — poll `GET /internal/queues/{namespace}/{app-name}` on queue service at each tick; hibernate worker when all queues return `idle: true`
- [ ] Idle threshold evaluation per app (`idle_after` from morsel.json or platform default `default_idle_after`)
- [ ] Scale-to-zero via `client-go` on idle threshold exceeded
- [ ] App hibernation state persisted in SQLite (`hibernated_at`, `hibernation_reason`)
- [ ] `HTTPRoute` update — route hibernated app's subdomain to wake-proxy Service in `morsel-services`
- [ ] Wake proxy binary — read `Host` header; call `POST /internal/wake/{namespace}/{name}` on Morsel API; forward held request to returned Service address on success; return `503 wake_timeout` on timeout
- [ ] Wake proxy Deployment + Service + NetworkPolicy in `morsel-services`; shared token Secret for Morsel API auth
- [ ] Morsel API internal wake endpoint — scale to 1, watch readiness, restore `HTTPRoute`, return Service address; cluster-internal only
- [ ] `HTTPRoute` restore — Morsel API restores subdomain to app Service as part of wake completion
- [ ] Worker hibernation — queue service idle flag polling; scale-to-zero when all queues idle
- [ ] `CronJob` suspend via `spec.suspend: true`; unsuspend on wake
- [ ] `POST /api/repos/:slug/apps/:name/hibernate` — force hibernate (synchronous)
- [ ] `POST /api/repos/:slug/apps/:name/wake` — force wake (synchronous); check soft/hard budget limit
- [ ] `GET /api/repos/:slug/apps/:name/status` — include `hibernated`, `hibernated_at`, `idle_since`

---

## Feature 14 — Quota Tiers

_Delivers: operators control resource limits per repo; Kubernetes enforces them._

- [ ] `tiers` table in SQLite; seed built-in `small`, `medium`, `large` on migration
- [ ] `GET /api/operator/tiers` — list all tiers
- [ ] `POST /api/operator/tiers` — create tier
- [ ] `PATCH /api/operator/tiers/:name` — edit tier; propagate `ResourceQuota`/`LimitRange` changes to all namespaces on that tier
- [ ] `DELETE /api/operator/tiers/:name` — reject if any repo is on it or it is the platform default
- [ ] `POST /api/operator/tiers/:name/set-default` — update platform default
- [ ] Tier assignment on repo creation (use platform default)
- [ ] `PATCH /api/operator/repos/:slug` — promote or demote tier; update all app namespaces
- [ ] App count enforcement at deploy time — `quota_exceeded` if repo is at its app limit
- [ ] Tier demotion guard — reject if current usage exceeds lower tier limits
- [ ] `morsel operator tier *` CLI commands (thin wrappers over operator API)

---

## Feature 15 — Approvals

_Delivers: protected field changes (tier, resource limits) require operator sign-off before taking effect._

- [ ] `approvals` table in SQLite (`id`, `repo`, `app`, `field`, `current_value`, `requested_value`, `requested_at`, `expires_at`, `status`)
- [ ] Protected field detection in deploy handler — create approval record when a protected field changes from its current approved value
- [ ] Approval coalescing — if an approval for the same field already exists, update `requested_value` in-place; do not reset `expires_at`
- [ ] Deploy proceeds at current approved config; changed field reverts until approved
- [ ] `GET /api/repos/:slug/approvals` — list pending approvals for repo
- [ ] `GET /api/operator/approvals` — list all pending approvals
- [ ] `GET /api/operator/approvals/:id` — single approval detail
- [ ] `POST /api/operator/approvals/batch` — approve / reject (with reason) / ignore
- [ ] Approved field reconciliation — on approve, redeploy app with the now-approved value
- [ ] Approval expiry background goroutine — runs daily; marks expired approvals as `expired`; reverts field to current approved value
- [ ] `morsel app deploy` CI warning annotations for pending approvals

---

## Feature 16 — Cost Estimation

_Delivers: operators can see estimated monthly spend per app and platform-wide._

- [ ] `scale_events` table in SQLite — columns: `id`, `namespace`, `app`, `event` (`scale_to_1` / `scale_to_0`), `occurred_at`; written on every hibernation and wake transition
- [ ] Daily price-fetch goroutine in Morsel API — call `Platform.Pricing().Prices()` once per day
- [ ] `LocalPlatform.Pricing()` — returns `Prices{}` with all-zero fields (LocalPlatform has no billing)
- [ ] `price_snapshots` table in SQLite — one immutable row per fetch; columns match `Prices` struct fields + `fetched_at`
- [ ] 48-hour staleness check — emit `prices_stale` warning in `GET /api/operator/status` if last snapshot is older than 48h
- [ ] Cost estimation function — compute `running_hours_this_period` from `scale_events` log; multiply by resource requests × latest snapshot prices
- [ ] `GET /api/repos/:slug/apps/:name/utilisation` — resource usage + `estimated_cost_month`
- [ ] `GET /api/repos/:slug` — include per-repo `estimated_cost_month` (sum of apps)
- [ ] `GET /api/operator/cost` — `estimated_total_month`, `prices_fetched_at`, per-repo breakdown
- [ ] `GET /api/operator/prices/history` — full snapshot list for debugging

---

## Feature 17 — Budget Enforcement

_Delivers: platform automatically enforces spend limits; no app can inadvertently blow the budget._

- [ ] `platform_config` table in SQLite — `budget_ceiling`, `soft_limit_pct`, `hard_limit_pct`, `default_idle_after`
- [ ] `GET /api/operator/config` + `PATCH /api/operator/config`
- [ ] Cost enforcement watcher goroutine — runs on configurable tick interval (default 5 min)
- [ ] Soft limit — set `budget_soft_limit_active` flag; wake-on-request proxy returns `503` with `Retry-After`; explicit wake returns `budget_soft_limit`
- [ ] Hard limit — force-hibernate all running non-exempt apps; wake blocked
- [ ] Billing period rollover — first tick after calendar month rollover clears flags; expires period exemptions
- [ ] Operator wake override — wake during active limit grants period exemption for remainder of billing period
- [ ] `exemptions` table in SQLite — app-level and repo-level; explicit vs period
- [ ] `POST /api/operator/app-exemptions` + `DELETE` — explicit exemption add/remove
- [ ] `POST /api/operator/repo-exemptions` + `DELETE` — repo-level exemption
- [ ] `GET /api/operator/exemptions` — list all active exemptions
- [ ] `morsel operator app exempt / repo exempt` CLI commands

---

## Feature 18 — Admin UI

_Delivers: operators have a browser interface for day-to-day management._

- [ ] React + TypeScript SPA scaffold (Vite); production build outputs a static bundle
- [ ] Operator token exchange on page load — LocalPlatform: POST to local-oidc; calls Morsel API
- [ ] Token storage — in-memory only (no localStorage, no cookies)
- [ ] App management view — list all apps; filter by repo/status/tier; per-app hibernate/wake/delete actions
- [ ] Repo management view — list repos; tier promotion button
- [ ] Approvals view — pending approvals table with batch approve/reject/ignore UI
- [ ] Cost dashboard — spend vs ceiling progress bar; per-repo breakdown table; hibernate candidates
- [ ] Platform status view — component health, cert alerts, failed deploys, pending approvals count
- [ ] Stale apps view — apps sorted by last deploy date; suppress-for-30-days per entry
- [ ] Morsel API serve static bundle on LocalPlatform (`GET /admin/*` route)

---

## Feature 19 — GCPPlatform

_Delivers: full production deployment on GCP; operator runs `morsel service bootstrap --platform gcp`._

- [ ] `platform/gcp/platform.go` — `GCPPlatform` struct; all `Platform` methods compile
- [ ] GCP OAuth browser flow — localhost callback listener; token held in memory only
- [ ] `GCPPlatform.Bootstrap().Prompts()` — project ID, region, base domain, DNS provider (Cloud DNS / Cloudflare)
- [ ] `GCPPlatform.Bootstrap().Plan()` — list GCP resources with estimated monthly costs
- [ ] Preflight checks — billing active, required APIs enabled, IAM permissions, compute quota, DNS zone
- [ ] `GCPPlatform.Bootstrap().Provision()` — provision in dependency order: GCS state bucket → VPC → GKE Autopilot → Artifact Registry → WIF → IAM bindings → Secret Manager → Morsel API install → Admin UI bundle to GCS → GKE Gateway classes → IAP → smoke test
- [ ] `GCPPlatform.Blobs()` — GCS implementation (`morsel-blobs-{project-id}` bucket)
- [ ] `GCPPlatform.Secrets()` — Secret Manager implementation
- [ ] `GCPPlatform.Credentials()` — Workload Identity metadata server token
- [ ] `GCPPlatform.DNS()` — Cloud DNS implementation
- [ ] `GCPPlatform.DNS()` — Cloudflare implementation (alternate; token from SecretStore)
- [ ] `GCPPlatform.Certs()` — ACME DNS-01 via Cloud DNS or Cloudflare
- [ ] `GCPPlatform.Pricing()` — Cloud Billing Catalog API (`cloudbilling.googleapis.com`)
- [ ] `GCPPlatform.DeployToken()` — obtain GitHub OIDC token from GitHub Actions environment (`ACTIONS_ID_TOKEN_REQUEST_URL`); fails if `GITHUB_ACTIONS` not set
- [ ] `GCPPlatform.ValidateDeployToken()` — fetch GitHub JWKS (cached), validate JWT signature, extract `repository` claim, return `org/repo` slug; generate short-lived Artifact Registry staging push credentials via WIF and attach to token response
- [ ] `POST /api/token/gcp-oidc` in Morsel API — validate GCP identity token (IAP-issued); issue operator token
- [ ] Admin UI authentication via IAP — IAP injects identity header; Morsel API verifies and exchanges for Morsel token
- [ ] Smoke test on bootstrap completion — deploy a test app, verify it is reachable, clean up
