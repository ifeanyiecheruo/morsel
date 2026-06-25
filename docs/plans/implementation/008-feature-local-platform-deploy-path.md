# Feature 08 — LocalPlatform Deploy Path

_Delivers: `morsel app deploy` works end-to-end on LocalPlatform — push a change and see a running pod._

**Direct dependencies:** [F03](003-feature-authentication.md), [F05](005-feature-app-lifecycle-api.md), [F06](006-feature-kubernetes-manifest-apply.md), [F07](007-feature-bootstrap-local-platform.md)

> F04 (Lint) is a natural gate before deploy but not a hard code dependency.

## Tasks

- [x] Local container registry provisioned during bootstrap (`registry:2` Deployment in `morsel` namespace)
- [x] Repo slug derivation — read git root directory name; prefix with `localhost/`; sanitize to slug format
- [x] `LocalPlatform.Tokens().CreateDeployToken()` — generate JWT signed with `local-deploy-signing-key` with derived `"repository": "localhost/{dirname}"`
- [x] `LocalPlatform.Deploy().StagingRegistry()` — return in-cluster registry URL (`localhost:5000`)
- [x] Staging handshake skipped on LocalPlatform — deployer pushes directly to canonical registry
- [x] `morsel app deploy` unified path — optional `--image`; builds from `dockerfile` field in `.morsel.json` when omitted; `registry_url` stored in profile at bootstrap
- [x] Reference GitHub Actions workflow file (`.github/workflows/morsel-deploy.yml`)
- [x] Deploy output formatting — image ref printed on deploy, "Done." on completion
