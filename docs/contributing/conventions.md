# Code Conventions

## Naming

No single-letter variable names. Use descriptive short names: `plat` over `p`, `rec` over `w`, `req`/`resp` over `r`/`w`.

## Comments

Comments explain *why* and possibly how at a high level, not *what*.
Don't restate what well-named identifiers already imply.
Don't place implementation details that can easily be changed in comments.
A comment is warranted when the code encodes a hidden constraint, a non-obvious invariant, or a workaround for a specific external behaviour.

## Layout

The order for code should be...

1) Public constants
2) Private constants
3) Public functions
4) Public structs, each struct followed by its methods
5) Private structs, each struct followed by its methods
6) Private functions

Where possible referenced constants, structs, or functions come after their referers.

## Initialization

Never use `init()`. It runs invisibly, cannot return errors, cannot be tested in isolation, and makes startup order implicit and hard to reason about. Instead, use an explicit constructor function (e.g. `Create()`) that the caller invokes. Panics on startup failure belong in the outermost entry point (`main` or equivalent), not scattered through package-level side effects.

## Context

Only use context.Background() in main.go and tests, never use it anywhere else in the codebase. Thread the context down to where it is needed

## Logging

Always get a logger from the contex, never log with the default logger or a privately created logger.
Alway log any error that does not need to be checked and is not returned to the caller.
Do not directly print, always write to the context logger
