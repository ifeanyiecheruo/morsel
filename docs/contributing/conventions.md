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
