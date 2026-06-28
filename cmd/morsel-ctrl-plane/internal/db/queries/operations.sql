-- name: GetOperation :one
SELECT * FROM operations
WHERE id = ?;

-- name: ListOperationsByApp :many
SELECT * FROM operations
WHERE repo_slug = ? AND app_name = ?
ORDER BY created_at DESC;

-- name: CreateOperation :one
INSERT INTO operations (id, repo_slug, app_name, kind)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: UpdateOperationStatus :exec
UPDATE operations
SET status     = ?,
    error      = ?,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = ?;
