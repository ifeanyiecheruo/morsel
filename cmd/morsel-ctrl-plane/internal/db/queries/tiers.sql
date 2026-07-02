-- name: ListTiers :many
SELECT * FROM tiers
ORDER BY name;

-- name: GetTier :one
SELECT * FROM tiers
WHERE name = ?;

-- name: GetDefaultTier :one
SELECT * FROM tiers
WHERE is_default = 1
LIMIT 1;

-- name: InsertTier :one
INSERT INTO tiers (name, max_apps, cpu_milli, memory_mb, blob_gb, database_gb, queue_gb, hibernate_after)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateTier :one
UPDATE tiers
SET max_apps        = ?,
    cpu_milli       = ?,
    memory_mb       = ?,
    blob_gb         = ?,
    database_gb     = ?,
    queue_gb        = ?,
    hibernate_after = ?,
    updated_at      = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE name = ?
RETURNING *;

-- name: DeleteTier :exec
DELETE FROM tiers
WHERE name = ?;

-- name: SetDefaultTier :exec
UPDATE tiers
SET is_default = CASE WHEN name = ? THEN 1 ELSE 0 END,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now');

-- name: CountReposByTier :one
SELECT COUNT(*) FROM repos
WHERE tier = ?;

-- name: ListReposByTier :many
SELECT * FROM repos
WHERE tier = ?
ORDER BY slug;
