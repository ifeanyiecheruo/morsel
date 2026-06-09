# Code Conventions

## Naming

No single-letter variable names. Use descriptive short names: `plat` over `p`, `rec` over `w`, `req`/`resp` over `r`/`w`.

## Comments

Comments explain *why*, not *what*. Don't restate what well-named identifiers already say. A comment is warranted when the code encodes a hidden constraint, a non-obvious invariant, or a workaround for a specific external behaviour.

## Error handling

No error handling for scenarios that cannot happen inside the module. No defensive fallbacks, no feature flags, no backwards-compatibility shims. Validate at system boundaries (user input, external APIs) and trust internal code.

## Platform abstraction

All platform-specific logic belongs in a `Platform` implementation (`platform/local/`, `platform/gcp/`), never in a handler or business logic function. Handlers receive a `platform.Platform` and call its interfaces. They never import cloud SDKs or make decisions based on which platform is active.

## Error shape

All API errors follow the structured shape defined in [docs/specs/conventions/rest.md](../specs/conventions/rest.md): `{"error": {"code", "message", "remedy", "context"}}`. Return `*api.APIError` from handlers; the `ErrorHandlerFunc` middleware serialises it.
