-- +goose Up

CREATE TABLE principals (
    username      TEXT     NOT NULL PRIMARY KEY,
    password_hash TEXT,
    salt          TEXT,
    created_at    DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- +goose Down

DROP TABLE principals;
