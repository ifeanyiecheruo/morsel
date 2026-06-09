-- name: GetRepo :one
SELECT * FROM repos
WHERE slug = ?;

-- name: ListRepos :many
SELECT * FROM repos
ORDER BY slug;

-- name: CreateRepo :one
INSERT INTO repos (slug, tier)
VALUES (?, ?)
RETURNING *;

-- name: UpdateRepoTier :one
UPDATE repos
SET tier = ?
WHERE slug = ?
RETURNING *;
