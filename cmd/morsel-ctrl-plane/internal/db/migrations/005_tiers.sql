-- +goose Up

CREATE TABLE tiers (
    name            TEXT     NOT NULL PRIMARY KEY,
    max_apps        INTEGER  NOT NULL,
    cpu_milli       INTEGER  NOT NULL,
    memory_mb       INTEGER  NOT NULL,
    blob_gb         INTEGER  NOT NULL,
    database_gb     INTEGER  NOT NULL,
    queue_gb        INTEGER  NOT NULL,
    hibernate_after TEXT     NOT NULL,
    is_default      INTEGER  NOT NULL DEFAULT 0,
    created_at      DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at      DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

INSERT INTO tiers (name, max_apps, cpu_milli, memory_mb, blob_gb, database_gb, queue_gb, hibernate_after, is_default)
VALUES
    ('small',  2,  500,  512,  1,  5,   1,  '24h', 1),
    ('medium', 10, 1000, 1024, 10, 20,  10, '48h', 0),
    ('large',  25, 4000, 4096, 50, 100, 50, '72h', 0);

-- +goose Down

DROP TABLE tiers;
