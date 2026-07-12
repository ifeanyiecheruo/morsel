-- +goose Up

CREATE TABLE repos (
    slug       TEXT     NOT NULL PRIMARY KEY,
    tier       TEXT     NOT NULL DEFAULT 'small',
    created_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE apps (
    id                 INTEGER  NOT NULL PRIMARY KEY AUTOINCREMENT,
    repo_slug          TEXT     NOT NULL REFERENCES repos(slug),
    name               TEXT     NOT NULL,
    type               TEXT     NOT NULL, -- http | worker | cron
    status             TEXT     NOT NULL DEFAULT 'pending',
    namespace          TEXT,
    image_current      TEXT,
    image_last_healthy TEXT,
    permanent          INTEGER  NOT NULL DEFAULT 0,
    deletion_pending   INTEGER  NOT NULL DEFAULT 0,
    deleted_at         DATETIME,
    created_at         DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at         DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    hibernated         INTEGER  NOT NULL DEFAULT 0,
    hibernated_at      DATETIME,
    hibernation_reason TEXT,
    last_active_at     DATETIME,
    idle_after         TEXT,
    UNIQUE (repo_slug, name)
);

CREATE TABLE operations (
    id         TEXT     NOT NULL PRIMARY KEY, -- UUID
    repo_slug  TEXT     NOT NULL,
    app_name   TEXT     NOT NULL,
    kind       TEXT     NOT NULL, -- deploy | delete | hibernate | wake
    status     TEXT     NOT NULL DEFAULT 'pending', -- pending | running | succeeded | failed
    error      TEXT,
    created_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

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

-- Principals are GitHub-authenticated users. is_operator and is_admin are
-- orthogonal: operators do platform work; admins manage who the operators are.
CREATE TABLE principals (
    github_id    INTEGER  NOT NULL PRIMARY KEY,
    github_login TEXT     NOT NULL UNIQUE,
    is_operator  INTEGER  NOT NULL DEFAULT 0,
    is_admin     INTEGER  NOT NULL DEFAULT 0,
    created_at   DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

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

CREATE TABLE scale_events (
    id          INTEGER  NOT NULL PRIMARY KEY AUTOINCREMENT,
    namespace   TEXT     NOT NULL,
    app         TEXT     NOT NULL,
    event       TEXT     NOT NULL, -- scale_to_1 | scale_to_0
    occurred_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_scale_events_lookup ON scale_events (namespace, app, occurred_at);

CREATE TABLE price_snapshots (
    id                        INTEGER  NOT NULL PRIMARY KEY AUTOINCREMENT,
    compute_cpu_per_core_hour REAL     NOT NULL,
    compute_mem_per_gb_hour   REAL     NOT NULL,
    storage_per_gb_month      REAL     NOT NULL,
    registry_per_gb_month     REAL     NOT NULL,
    fetched_at                DATETIME NOT NULL
);

CREATE TABLE platform_config (
    id                       INTEGER NOT NULL PRIMARY KEY CHECK (id = 1),
    budget_ceiling_monthly   REAL    NOT NULL DEFAULT 500.0,
    soft_limit_pct           REAL    NOT NULL DEFAULT 0.9,
    hard_limit_pct           REAL    NOT NULL DEFAULT 1.0,
    default_idle_after       TEXT    NOT NULL DEFAULT '24h',
    budget_soft_limit_active INTEGER NOT NULL DEFAULT 0,
    budget_hard_limit_active INTEGER NOT NULL DEFAULT 0,
    billing_period           TEXT    NOT NULL DEFAULT ''
);

INSERT OR IGNORE INTO platform_config (id) VALUES (1);

CREATE TABLE exemptions (
    id         INTEGER  NOT NULL PRIMARY KEY AUTOINCREMENT,
    kind       TEXT     NOT NULL,            -- 'app' | 'repo'
    repo_slug  TEXT     NOT NULL,
    app_name   TEXT     NOT NULL DEFAULT '', -- '' for repo-level exemptions
    type       TEXT     NOT NULL,            -- 'explicit' | 'period'
    expires_at DATETIME,                     -- NULL for explicit; end of billing period for period
    created_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE (kind, repo_slug, app_name, type)
);

CREATE TABLE stale_suppressed (
    repo_slug TEXT NOT NULL,
    app_name  TEXT NOT NULL,
    until     TEXT NOT NULL,
    PRIMARY KEY (repo_slug, app_name)
);

-- +goose Down

DROP TABLE stale_suppressed;
DROP TABLE exemptions;
DROP TABLE platform_config;
DROP TABLE price_snapshots;
DROP INDEX idx_scale_events_lookup;
DROP TABLE scale_events;
DROP TABLE tiers;
DROP TABLE principals;
DROP INDEX refresh_tokens_hash_idx;
DROP TABLE refresh_tokens;
DROP TABLE operations;
DROP TABLE apps;
DROP TABLE repos;
