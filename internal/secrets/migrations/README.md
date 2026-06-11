# Secret migrations

Each file in this folder is a migration script for secrets applied on every server startup. Migrations are idempotent — safe to re-run.

## Naming

Files must be named `NNN_description.secrets.txt` where `NNN` is a zero-padded integer. They are applied in lexicographic filename order, so the numeric prefix determines sequence.

```
001_initial.secrets.txt
002_rename_signing_key.secrets.txt
003_delete_legacy_deploy_key.secrets.txt
```

## Script format

Each line is either a blank line, a comment, or a directive.

```
# This is a comment — ignored by the parser.

rename "old-secret-name" "new-secret-name"
delete "stale-secret-name"
```

### `rename "src" "dst"`

Copies the value of `src` to `dst`, then deletes `src`. If `src` does not exist the directive is a no-op (safe to re-run after partial failure).

### `delete "name"`

Removes `name` from the secret store. If `name` does not exist the directive is a no-op.

## Rules

- Secret names are double-quoted strings.
- Each migration file is append-only once merged — never edit or delete a committed file.
- Add new migrations in new files with the next sequence number.
- A failed migration aborts startup; subsequent files are not applied.
