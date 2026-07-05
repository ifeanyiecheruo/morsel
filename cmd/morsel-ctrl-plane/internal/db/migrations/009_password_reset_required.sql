-- +goose Up

ALTER TABLE principals ADD COLUMN password_reset_required INTEGER NOT NULL DEFAULT 0;

-- +goose Down

ALTER TABLE principals DROP COLUMN password_reset_required;
