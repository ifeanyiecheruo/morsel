# Feature 03 — Authentication

_Delivers: deploy identity token exchange works; operator can `morsel operator login` on LocalPlatform._

**Direct dependencies:** [F02](002-feature-api-skeleton.md)

## Tasks

- [x] JWT signing key: load from `SecretStore` at startup; generate and persist on first run if absent
- [x] `POST /api/token/deploy` — call `Platform.ValidateDeployToken(token)` → repo slug; issue 10-min developer access token; no platform-specific logic in the handler
- [x] `LocalPlatform.ValidateDeployToken()` — validate JWT signature against `local-deploy-signing-key`, extract `repository` claim, return `localhost/{dirname}` slug
- [x] Auth middleware — verify JWT signature, parse role + repo claims, attach to request context
- [x] `repos` ownership enforcement — 403 if token `repo` claim doesn't match `:slug`
- [x] SQLite schema: `refresh_tokens` table
- [x] `POST /api/token/refresh` — validate refresh token, issue new access token + rotated refresh token
- [ ] `POST /api/token/oidc` — call `Platform.ValidateOperatorToken(ctx, r)` → operator subject; issue 15-min operator access token + 90-day refresh token; add `ValidateOperatorToken` to `CredentialProvider` interface and implement on `LocalPlatform` (reads email from request body, checks against principal list in SecretStore)
- [ ] `morsel operator login` CLI command — LocalPlatform path; POST to `/api/token/local-oidc`
- [ ] Profile file write (`~/.config/morsel/<profile>.profile.json`, mode 0600)
- [ ] CLI pre-command hook — load profile, silent refresh if access token expired, re-prompt login if refresh token expired
