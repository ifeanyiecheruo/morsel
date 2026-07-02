-- +goose Up

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
    kind       TEXT     NOT NULL,                 -- 'app' | 'repo'
    repo_slug  TEXT     NOT NULL,
    app_name   TEXT     NOT NULL DEFAULT '',      -- '' for repo-level exemptions
    type       TEXT     NOT NULL,                 -- 'explicit' | 'period'
    expires_at DATETIME,                          -- NULL for explicit; end of billing period for period
    created_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE (kind, repo_slug, app_name, type)
);

-- +goose Down

DROP TABLE exemptions;
DELETE FROM platform_config;
DROP TABLE platform_config;
