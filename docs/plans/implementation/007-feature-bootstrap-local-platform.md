# Feature 07 — Bootstrap: LocalPlatform

_Delivers: `morsel service bootstrap --platform local` provisions a working local Morsel instance from scratch._

**Direct dependencies:** [F02](002-feature-api-skeleton.md), [F03](003-feature-authentication.md)

> Can be developed in parallel with F05 — both depend on F03.

## Tasks

- [x] `morsel service bootstrap` command — phase runner with progress output; idempotent
- [x] `LocalPlatform.Secrets()` — filesystem implementation (`~/.morsel/local/secrets.json`)
- [x] `LocalPlatform.Bootstrap().Prompts()` — collect base domain (default `morsel.localhost`), optional config
- [x] `LocalPlatform.Bootstrap().Plan()` — describe what will be created; no estimated cost (LocalPlatform is free)
- [x] `LocalPlatform.Bootstrap().Provision()` — write bootstrap config to secret store; generate and store `local-deploy-signing-key` in SecretStore; verify cluster access when kubeconfig is supplied; provision local container registry
- [x] Bootstrap config persistence — store wizard answers in local secret store; subsequent runs skip wizard
- [x] `morsel service status` — health-check all platform components; report pass/fail per component
- [x] `morsel service delete` — tear down all platform resources; requires explicit `--confirm` flag
- [x] `morsel operator principal add/remove/list` — manage local operator principal list in secret store

## Remaining tasks — control plane deployment

- [x] `Dockerfile` for `morsel-ctrl-plane` — multi-stage Go build (`CGO_ENABLED=0`); distroless nonroot final image
- [x] kind cluster config file — `extraPortMappings` host `8080` → node `30080`; `make cluster-up` uses `hack/kind-config.yaml`
- [x] `kube.EnsureAPI(ctx, ns, image, dbPath)` — idempotent Deployment + NodePort Service; HostPath volume for SQLite DB; `--platform local --db` flags; `ImagePullPolicy: Never`
- [x] `LocalPlatform.Bootstrap().Provision()` — detect container runtime; `docker/podman build`; `kind load docker-image`; `EnsureRegistry`; `EnsureAPI`
- [x] Bootstrap health-check loop — poll `GET /healthz` on `localhost:8080` with exponential back-off up to 3 min; clear timeout error
- [x] Profile `api_url` set to `http://localhost:8080` at bootstrap
- [x] Trim `Plan()` — only registry and morsel-api listed; removed aspirational resources not yet implemented
