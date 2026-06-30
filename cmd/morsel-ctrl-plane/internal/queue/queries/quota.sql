-- name: GetQuota :one
SELECT total_bytes, limit_bytes FROM quota;

-- name: SetLimitBytes :exec
UPDATE quota SET limit_bytes = ?;

-- name: AddBytes :exec
UPDATE quota SET total_bytes = total_bytes + ?;
