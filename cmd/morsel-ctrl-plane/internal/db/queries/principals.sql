-- name: ListPrincipals :many
SELECT username FROM principals ORDER BY username;

-- name: InsertPrincipal :exec
INSERT INTO principals (username) VALUES (@username)
ON CONFLICT (username) DO NOTHING;

-- name: DeletePrincipal :exec
DELETE FROM principals WHERE username = @username;

-- name: PrincipalExists :one
SELECT EXISTS(SELECT 1 FROM principals WHERE username = @username) AS "exists";

-- name: GetPrincipalPasswordHash :one
SELECT password_hash FROM principals WHERE username = @username;

-- name: SetPrincipalPasswordHash :exec
UPDATE principals SET password_hash = @password_hash WHERE username = @username;

-- name: ListPrincipalsWithDetails :many
SELECT username, password_reset_required, is_admin FROM principals ORDER BY username;

-- name: GetPrincipalPasswordResetRequired :one
SELECT password_reset_required FROM principals WHERE username = @username;

-- name: SetPrincipalPasswordResetRequired :exec
UPDATE principals SET password_reset_required = @password_reset_required WHERE username = @username;

-- name: GetPrincipalSecurityState :one
SELECT password_reset_required, password_changed_at FROM principals WHERE username = @username;

-- name: SetPasswordChangedAt :exec
UPDATE principals SET password_changed_at = @password_changed_at, password_reset_required = 0 WHERE username = @username;

-- name: GetPrincipalIsAdmin :one
SELECT is_admin FROM principals WHERE username = @username;

-- name: SetPrincipalIsAdmin :exec
UPDATE principals SET is_admin = @is_admin WHERE username = @username;

-- name: CountAdmins :one
SELECT COUNT(*) FROM principals WHERE is_admin = 1;

-- name: InvalidatePrincipalPassword :exec
UPDATE principals
SET password_reset_required = 1,
    password_changed_at = @password_changed_at
WHERE username = @username;
