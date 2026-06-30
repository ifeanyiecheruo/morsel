CREATE TABLE IF NOT EXISTS messages (
    id               TEXT    NOT NULL PRIMARY KEY,
    body             BLOB    NOT NULL,
    body_size        INTEGER NOT NULL,
    enqueued_at      TEXT    NOT NULL,
    visibility_until TEXT
);

CREATE TABLE IF NOT EXISTS meta (
    key   TEXT NOT NULL PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS quota (
    total_bytes INTEGER NOT NULL DEFAULT 0,
    limit_bytes INTEGER NOT NULL DEFAULT 0
);
