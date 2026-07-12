-- name: UpsertPrincipal :one
INSERT INTO principals (github_id, github_login)
VALUES (@github_id, @github_login)
ON CONFLICT (github_id) DO UPDATE SET github_login = excluded.github_login
RETURNING *;

-- name: GetPrincipalByID :one
SELECT * FROM principals WHERE github_id = @github_id;

-- name: GetPrincipalByLogin :one
SELECT * FROM principals WHERE github_login = @github_login;

-- name: ListPrincipals :many
SELECT * FROM principals ORDER BY github_login;

-- name: DeletePrincipal :exec
DELETE FROM principals WHERE github_id = @github_id;

-- name: SetPrincipalIsOperator :exec
UPDATE principals SET is_operator = @is_operator WHERE github_id = @github_id;

-- name: SetPrincipalIsAdmin :exec
UPDATE principals SET is_admin = @is_admin WHERE github_id = @github_id;

-- name: CountAdmins :one
SELECT COUNT(*) FROM principals WHERE is_admin = 1;

-- name: CountOperators :one
SELECT COUNT(*) FROM principals WHERE is_operator = 1;
