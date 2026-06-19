# Feature 07 — Bootstrap: LocalPlatform

_Delivers: `morsel service bootstrap --platform local` provisions a working local Morsel instance from scratch._

**Direct dependencies:** [F02](002-feature-api-skeleton.md), [F03](003-feature-authentication.md)

> Can be developed in parallel with F05 — both depend on F03.

## Tasks

- [ ] `morsel service bootstrap` command — phase runner with progress output; idempotent
- [ ] `LocalPlatform.Secrets()` — filesystem implementation (`~/.morsel/local/secrets.json`)
- [ ] `LocalPlatform.Bootstrap().Prompts()` — collect base domain (default `morsel.localhost`), optional config
- [ ] `LocalPlatform.Bootstrap().Plan()` — describe what will be created; no estimated cost (LocalPlatform is free)
- [ ] `LocalPlatform.Bootstrap().Provision()` — install Morsel API, blob service, queue service, database service, local registry into cluster; write bootstrap config to secret store; generate and store `local-deploy-signing-key` in SecretStore
- [ ] Bootstrap config persistence — store wizard answers in local secret store; subsequent runs skip wizard
- [ ] `morsel service status` — health-check all platform components; report pass/fail per component
- [ ] `morsel service delete` — tear down all platform resources; requires explicit `--confirm` flag
- [ ] `morsel operator principal add/remove/list` — manage local operator principal list in secret store
