CREATE TABLE repos (
    slug       TEXT    NOT NULL PRIMARY KEY,
    tier       TEXT    NOT NULL DEFAULT 'small',
    created_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE apps (
    id                 INTEGER  NOT NULL PRIMARY KEY AUTOINCREMENT,
    repo_slug          TEXT     NOT NULL REFERENCES repos(slug),
    name               TEXT     NOT NULL,
    type               TEXT     NOT NULL, -- http | worker | cronjob
    status             TEXT     NOT NULL DEFAULT 'pending',
    namespace          TEXT,
    image_current      TEXT,
    image_last_healthy TEXT,
    permanent          INTEGER  NOT NULL DEFAULT 0,
    deletion_pending   INTEGER  NOT NULL DEFAULT 0,
    deleted_at         DATETIME,
    created_at         DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at         DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
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
