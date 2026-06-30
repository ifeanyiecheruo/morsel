-- name: GetMeta :one
SELECT value FROM meta WHERE key = ?;

-- name: UpsertMeta :exec
INSERT OR REPLACE INTO meta(key, value) VALUES (?, ?);
