# Feature 04 — App Lint and Schema Validation

_Delivers: `morsel lint` works; developers catch schema errors before pushing._

**Direct dependencies:** [F03a](003a-feature-cli-scaffold.md)

> Can be developed in parallel with F03 — requires the CLI scaffold (F03a) but not the API server.

## Tasks

- [x] `morsel lint` command — find and validate all `*.morsel.json` files in `.morsel/`; validate against `morsel.schema.json`
- [x] `morsel lint --staged` — validate only git-staged files; suitable for pre-commit hook
- [x] `morsel lint --fix` — auto-correct safe issues (field ordering, whitespace)
- [x] Lint checks: schema validity, valid `type` value, `schedule`+`timeout` present for `cronjob`, unique `name` within `.morsel/`, `permanent: true` removal warning, type-incompatible field warnings
