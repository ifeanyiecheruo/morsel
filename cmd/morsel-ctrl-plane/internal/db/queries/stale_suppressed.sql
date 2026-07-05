-- name: UpsertStaleSuppressed :exec
INSERT INTO stale_suppressed (repo_slug, app_name, until)
VALUES (?, ?, ?)
ON CONFLICT (repo_slug, app_name) DO UPDATE SET until = excluded.until;

-- name: ListActiveStaleSuppressed :many
SELECT repo_slug, app_name, until
FROM stale_suppressed
WHERE until > ?;
