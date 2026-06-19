# Feature 04 — App Lint and Schema Validation

_Delivers: `morsel lint` works; developers catch schema errors before pushing._

**Direct dependencies:** [F01](001-feature-repository-foundation.md)

> Can be developed in parallel with F02 and F03 — requires only the CLI infrastructure, not the API server.

## Tasks

- [ ] `morsel lint` command — find and validate all `*.morsel.json` files in `.morsel/`; validate against `morsel.schema.json`
- [ ] `morsel lint --staged` — validate only git-staged files; suitable for pre-commit hook
- [ ] `morsel lint --fix` — auto-correct safe issues (field ordering, whitespace)
- [ ] Lint checks: schema validity, valid `type` value, `schedule`+`timeout` present for `cronjob`, unique `name` within `.morsel/`, `permanent: true` removal warning, type-incompatible field warnings
