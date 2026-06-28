-- +goose Up

CREATE TABLE refresh_tokens (
    id         TEXT     NOT NULL PRIMARY KEY,
    token_hash TEXT     NOT NULL UNIQUE,
    subject    TEXT     NOT NULL,
    role       TEXT     NOT NULL,
    repo_slug  TEXT,
    expires_at DATETIME NOT NULL,
    revoked_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX refresh_tokens_hash_idx ON refresh_tokens (token_hash);

-- +goose Down

DROP INDEX refresh_tokens_hash_idx;
DROP TABLE refresh_tokens;
