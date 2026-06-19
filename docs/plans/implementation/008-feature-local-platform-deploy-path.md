# Feature 08 — LocalPlatform Deploy Path

_Delivers: `morsel app deploy` works end-to-end on LocalPlatform — push a change and see a running pod._

**Direct dependencies:** [F03](003-feature-authentication.md), [F05](005-feature-app-lifecycle-api.md), [F06](006-feature-kubernetes-manifest-apply.md), [F07](007-feature-bootstrap-local-platform.md)

> F04 (Lint) is a natural gate before deploy but not a hard code dependency.

## Tasks

- [ ] Local container registry provisioned during bootstrap (`registry:2` Deployment in `morsel` namespace)
- [ ] Repo slug derivation — read git root directory name; prefix with `localhost/`; sanitize to slug format
- [ ] `LocalPlatform.DeployToken()` — generate JWT signed with `local-deploy-signing-key` with `{ "repository": "localhost/{dirname}", "ref": "...", "sha": "..." }`
- [ ] `LocalPlatform.Deploy().StagingRegistry()` — return in-cluster registry URL
- [ ] Staging handshake skipped on LocalPlatform — deployer pushes directly to canonical registry
- [ ] `morsel app deploy` unified path — call `Platform.DeployToken()`; exchange at `POST /api/token/deploy`; build images; push; call sync + deploy APIs; emit annotations when in CI
- [ ] Reference GitHub Actions workflow file (`.github/workflows/morsel-deploy.yml`)
- [ ] Deploy output formatting — per-app status lines, approval warnings, failure messages
