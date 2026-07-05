-- +goose Up
ALTER TABLE principals ADD COLUMN password_changed_at DATETIME;

-- +goose Down
ALTER TABLE principals DROP COLUMN password_changed_at;
