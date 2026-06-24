# Morsel JSON Schemas

Canonical schema files for files authored by Morsel users. These are embedded in the `morsel` CLI binary (via `schemas/schema.go`) and also published for IDE validation.

## Files

| File | Description |
|---|---|
| `morsel.schema.json` | Validates `.morsel/*.morsel.json` app declaration files |

## IDE validation

Add `"$schema"` to any `.morsel.json` file to get in-editor validation and autocompletion:

```json
{
  "$schema": "https://raw.githubusercontent.com/ifeanyiecheruo/morsel/v0.1.0/schemas/morsel.schema.json",
  "type": "http",
  "dockerfile": "Dockerfile"
}
```

VS Code and JetBrains IDEs also pick up the schema automatically via the SchemaStore registration (no `$schema` field needed).

## SchemaStore registration

The schema is registered at [SchemaStore](https://www.schemastore.org/json/) so IDEs load it automatically for `*.morsel.json` files. The catalog entry (in `src/api/json/catalog.json` in the SchemaStore repo) is:

```json
{
  "name": "Morsel app declaration",
  "description": "Declares a Morsel app within a repository (.morsel/*.morsel.json)",
  "fileMatch": [".morsel/*.morsel.json"],
  "url": "https://raw.githubusercontent.com/ifeanyiecheruo/morsel/v0.1.0/schemas/morsel.schema.json"
}
```

## Versioning and release process

Schema versions are pinned to git tags. The `$id` in each schema file is the raw GitHub URL for that tag.

When making a breaking schema change:

1. Update `schemas/morsel.schema.json` with the new fields/constraints.
2. Update the `$id` to the next version tag (e.g. `v0.2.0`).
3. Commit, tag the release (`git tag v0.2.0`), push the tag.
4. Update the `url` in the SchemaStore catalog entry to the new tag URL and open a PR.

Non-breaking changes (new optional fields) do not require a version bump — the existing tag URL continues to work for users who have pinned to it.
