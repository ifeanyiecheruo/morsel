-- +goose Up

ALTER TABLE principals ADD COLUMN is_admin INTEGER NOT NULL DEFAULT 0;

-- +goose Down

ALTER TABLE principals DROP COLUMN is_admin;
