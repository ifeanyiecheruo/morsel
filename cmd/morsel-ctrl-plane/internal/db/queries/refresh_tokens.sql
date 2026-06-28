-- name: InsertRefreshToken :exec
INSERT INTO refresh_tokens (id, token_hash, subject, role, repo_slug, expires_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: GetRefreshTokenByHash :one
SELECT id, token_hash, subject, role, repo_slug, expires_at, revoked_at, created_at
FROM refresh_tokens
WHERE token_hash = ?;

-- name: RotateRefreshToken :one
UPDATE refresh_tokens
SET token_hash = ?,
    expires_at = ?
WHERE id = ?
RETURNING id, token_hash, subject, role, repo_slug, expires_at, revoked_at, created_at;
