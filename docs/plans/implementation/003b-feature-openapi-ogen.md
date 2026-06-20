# Feature 03b — OpenAPI Spec & ogen Code Generation

_Delivers: a machine-readable OpenAPI 3.x spec is the single source of truth for the API; ogen generates the server handler interface and a typed HTTP client; the CLI has a ready-to-use API client package._

**Direct dependencies:** [F03](003-feature-authentication.md)

> Must complete before F05. All future server endpoints implement the ogen-generated `Handler` interface rather than hand-wiring routes into the `ServeMux`. All CLI commands that make API calls use the generated client.

---

## Design

### Why ogen

ogen generates both the server handler interface and a typed client from a single OpenAPI spec, validated against the spec at compile time on both sides. Unlike oapi-codegen, ogen validates request and response bodies automatically, generates its own JSON codec (no `encoding/json` reflection), and uses union return types that force exhaustive handling of every documented response code. The tradeoff is a stricter spec requirement and a generated router that replaces the existing `ServeMux` — both acceptable given that most routes are still stubs.

### Code layout after this feature

```
api/
  openapi/                        ← source of truth (checked in, multi-file)
    openapi.yaml                  ← root document; $refs paths/, schemas/, parameters, responses
    parameters.yaml               ← shared path parameters (OrgParam, RepoParam, …)
    responses.yaml                ← shared error responses (ErrorBadRequest, …)
    schemas/                      ← one file per schema (App.yaml, Repo.yaml, …)
    paths/                        ← one file per path item (repos-org-repo.yaml, …)
    embed.go                      ← embeds all spec files into the binary via go:embed

internal/api/
  oas/                            ← ogen output (checked in, excluded from linting and diffs)
    *_gen.go
  handler/                        ← hand-written, implements oas.Handler + oas.SecurityHandler
    handler.go                    ← Handler struct; constructor
    security.go                   ← SecurityHandler: JWT validation + RBAC
    token.go                      ← token route logic (deploy, OIDC, refresh)
  wellknown/                      ← serves /.well-known/ discovery documents
    bundle.go                     ← recursive $ref resolver: YAML multi-file → single JSON
    handlers.go                   ← HTTP handlers for /openapi, /api-catalog, /ai-plugin.json
  server.go                       ← constructs ogen Server; mounts wellknown under /.well-known/

internal/apiclient/
  client.go                       ← thin wrapper over oas.Client; injects bearer token; handles refresh
```

### Auth moves from middleware to SecurityHandler

ogen calls the `SecurityHandler` before every operation handler, before any business logic runs. The three existing middleware functions (`RequireAuth`, `RequireRepo`, `RequireOperator`) map cleanly:

| Current middleware | ogen equivalent |
|---|---|
| `RequireAuth` | `SecurityHandler.HandleBearerAuth` — validates JWT signature, extracts claims |
| `RequireRepo` | `SecurityHandler.HandleBearerAuth` — checks `repo` claim against `{org}/{repo}` params for developer tokens |
| `RequireOperator` | `SecurityHandler.HandleBearerAuth` — checks role claim is `operator` |

The SecurityHandler injects parsed JWT claims into `context.Context`. Handler methods read claims from context — same pattern as today, but claims are put there by ogen's security layer instead of middleware.

ogen calls SecurityHandler for every operation that declares a security requirement in the spec. Operations are tagged with their auth level:

- No `security:` entry → token exchange endpoints (public)
- `security: [bearerAuth: []]` with `x-morsel-auth: repo` → repo-scoped endpoints
- `security: [bearerAuth: []]` with `x-morsel-auth: operator` → operator-only endpoints

The `x-morsel-auth` extension is a spec-level annotation — SecurityHandler reads the operation name to determine which RBAC check to apply.

### Error model integration

The morsel error shape (`{error: {code, message, remedy, context?}}`) is defined as a reusable `Error` component in the spec and referenced by all 4xx/5xx responses. ogen's `WithErrorHandler` option intercepts decode and validation errors from ogen's own pipeline and translates them to this shape before the response is written.

### Async 202 responses

ogen's union return types model multi-status responses naturally. An endpoint like `POST /api/repos/{org}/{repo}/apps` returns `PostRepoAppsRes`, which is satisfied by either `*PostRepoApps202Response` (success) or `*ErrorResponse`. The 202 variant carries both the response body and the `Location`/`Retry-After` headers — ogen generates the header-setting code from the spec.

### net/http middleware is unchanged

ogen's generated `Server` implements `http.Handler`. The existing `InjectLogger` and `LogRequests` middleware wrap it unchanged — the outer middleware layer is not affected by the switch from `ServeMux` to the ogen router.

### Well-known discovery endpoints

The `internal/api/wellknown` package serves three standard discovery documents under `/.well-known/`:

- `/openapi` — fully resolved OpenAPI spec as a single JSON document (`application/openapi+json`). The multi-file YAML spec is bundled at server startup by recursively resolving `$ref` pointers.
- `/api-catalog` — RFC 9727 Linkset JSON (`application/linkset+json`) pointing to the OpenAPI description.
- `/ai-plugin.json` — OpenAI plugin manifest for AI agent discovery.

`server.go` mounts `wellknown.New("/.well-known")` as a subtree handler; the wellknown package owns all three path registrations internally.

---

## Tasks

- [x] Write `api/openapi/openapi.yaml` — full multi-file spec for all ~30 endpoints: security scheme (`bearerAuth`), shared path parameters, shared error responses, per-schema files under `schemas/`, per-path files under `paths/`, description fields on all operations and schemas
- [x] Add ogen to build pipeline — `go get github.com/ogen-go/ogen`; `internal/api/generate.go` with `//go:generate` directive; exclude `internal/api/oas/` from golangci-lint and diffs via `.golangci.yml` and `.gitattributes`
- [x] Validate codegen — run `go generate` and confirm the generated package compiles cleanly against the spec
- [x] Implement `oas.SecurityHandler` in `internal/api/handler/security.go` — JWT validation, claims extraction, RBAC check keyed on operation name; injects claims into context
- [x] Implement `oas.Handler` stub in `internal/api/handler/handler.go` — all operations return the ogen equivalent of "not yet implemented"; tests confirm the server starts and stubs return the expected error shape
- [x] Migrate token handlers — move logic from `internal/api/routes/token*.go` into the corresponding `oas.Handler` methods in `internal/api/handler/token.go`; delete the now-unused `internal/api/routes/` files
- [x] Rebuild `internal/api/server.go` — construct ogen `Server` (passing Handler + SecurityHandler + `WithErrorHandler` for morsel error shape); mount `wellknown.Handlers` at `/.well-known/`; wrap with `InjectLogger`/`LogRequests`
- [x] Implement well-known discovery — `internal/api/wellknown/bundle.go` bundles the multi-file YAML spec into a single JSON document; `handlers.go` serves `/openapi`, `/api-catalog` (RFC 9727), and `/ai-plugin.json`
- [x] Add `internal/apiclient/` — wraps `oas.Client`; constructor accepts a `Profile`; injects `Authorization: Bearer` on every request; on 401 calls the refresh endpoint, updates the profile on disk, and retries once
- [x] Wire `apiclient.Client` into `internal/cli/handler.go` — `cliHandler` holds a `*apiclient.Client`; constructed in `cli.Execute()` after profile is loaded
