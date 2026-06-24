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

-- name: UpsertRepo :one
INSERT INTO repos (slug, tier) VALUES (?, 'small')
ON CONFLICT(slug) DO UPDATE SET slug = slug
RETURNING *;

-- name: CountAppsByRepo :one
SELECT COUNT(*) FROM apps
WHERE repo_slug = ? AND deletion_pending = 0;
