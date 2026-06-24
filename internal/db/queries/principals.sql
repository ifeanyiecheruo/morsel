-- name: ListPrincipals :many
SELECT username FROM principals ORDER BY username;

-- name: InsertPrincipal :exec
INSERT INTO principals (username) VALUES (@username)
ON CONFLICT (username) DO NOTHING;

-- name: DeletePrincipal :exec
DELETE FROM principals WHERE username = @username;

-- name: PrincipalExists :one
SELECT EXISTS(SELECT 1 FROM principals WHERE username = @username) AS "exists";
