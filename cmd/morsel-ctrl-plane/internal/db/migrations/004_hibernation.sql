-- +goose Up

ALTER TABLE apps ADD COLUMN hibernated         INTEGER  NOT NULL DEFAULT 0;
ALTER TABLE apps ADD COLUMN hibernated_at      DATETIME;
ALTER TABLE apps ADD COLUMN hibernation_reason TEXT;
ALTER TABLE apps ADD COLUMN last_active_at     DATETIME;
ALTER TABLE apps ADD COLUMN idle_after         TEXT;

-- +goose Down

ALTER TABLE apps DROP COLUMN idle_after;
ALTER TABLE apps DROP COLUMN last_active_at;
ALTER TABLE apps DROP COLUMN hibernation_reason;
ALTER TABLE apps DROP COLUMN hibernated_at;
ALTER TABLE apps DROP COLUMN hibernated;
