-- +goose Up

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

-- +goose Down

DROP TABLE price_snapshots;
DROP INDEX idx_scale_events_lookup;
DROP TABLE scale_events;
