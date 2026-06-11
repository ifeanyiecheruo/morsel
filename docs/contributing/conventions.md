# Code Conventions

## Naming

No single-letter variable names. Use descriptive short names: `plat` over `p`, `rec` over `w`, `req`/`resp` over `r`/`w`.

## Comments

Comments explain *why*, not *what*. Don't restate what well-named identifiers already say. A comment is warranted when the code encodes a hidden constraint, a non-obvious invariant, or a workaround for a specific external behaviour.

## Layout

Constants first, public constants then private constants

Public functions next

Public structs next. Each struct followed by its methods

Private structs next. Each struct followed by its methods

Private functions last

Where possible referenced constants, structs, or functions come before their referers.
