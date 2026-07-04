-- +goose Up
CREATE TABLE stale_suppressed (
    repo_slug TEXT NOT NULL,
    app_name  TEXT NOT NULL,
    until     TEXT NOT NULL,
    PRIMARY KEY (repo_slug, app_name)
);

-- +goose Down
DROP TABLE stale_suppressed;
