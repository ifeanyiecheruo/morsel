-- name: GetApp :one
SELECT * FROM apps
WHERE repo_slug = ? AND name = ?;

-- name: ListAppsByRepo :many
SELECT * FROM apps
WHERE repo_slug = ? AND deletion_pending = 0
ORDER BY name;

-- name: CreateApp :one
INSERT INTO apps (repo_slug, name, type, namespace)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: UpdateAppStatus :exec
UPDATE apps
SET status     = ?,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = ?;

-- name: UpdateAppImages :exec
UPDATE apps
SET image_current      = ?,
    image_last_healthy = ?,
    updated_at         = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = ?;

-- name: MarkAppDeletionPending :exec
UPDATE apps
SET deletion_pending = 1,
    deleted_at       = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'),
    updated_at       = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = ?;

-- name: UpsertApp :one
INSERT INTO apps (repo_slug, name, type, namespace, image_current, idle_after)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(repo_slug, name) DO UPDATE SET
    type               = excluded.type,
    namespace          = excluded.namespace,
    image_last_healthy = apps.image_current,
    image_current      = excluded.image_current,
    idle_after         = excluded.idle_after,
    status             = 'pending',
    deletion_pending   = 0,
    deleted_at         = NULL,
    updated_at         = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
RETURNING *;

-- name: SetAppHibernated :exec
UPDATE apps
SET hibernated          = 1,
    hibernated_at       = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'),
    hibernation_reason  = ?,
    status              = 'hibernated',
    updated_at          = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = ?;

-- name: SetAppAwake :exec
UPDATE apps
SET hibernated          = 0,
    hibernated_at       = NULL,
    hibernation_reason  = NULL,
    status              = 'running',
    updated_at          = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = ?;

-- name: UpdateLastActiveAt :exec
UPDATE apps
SET last_active_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'),
    updated_at     = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = ?;

-- name: ListAllApps :many
SELECT * FROM apps
WHERE deletion_pending = 0
ORDER BY repo_slug, name;

-- name: GetAppByNamespace :one
SELECT * FROM apps
WHERE namespace = ? AND deletion_pending = 0
LIMIT 1;
