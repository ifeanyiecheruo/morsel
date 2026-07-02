-- name: ListActiveExemptions :many
SELECT * FROM exemptions
WHERE expires_at IS NULL OR expires_at > strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
ORDER BY kind, repo_slug, app_name;

-- name: UpsertExemption :exec
INSERT INTO exemptions (kind, repo_slug, app_name, type, expires_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (kind, repo_slug, app_name, type) DO UPDATE
    SET expires_at = excluded.expires_at;

-- name: DeleteAppExemption :exec
DELETE FROM exemptions
WHERE kind = 'app' AND repo_slug = ? AND app_name = ? AND type = 'explicit';

-- name: DeleteRepoExemption :exec
DELETE FROM exemptions
WHERE kind = 'repo' AND repo_slug = ? AND type = 'explicit';

-- name: IsAppExempt :one
SELECT EXISTS (
    SELECT 1 FROM exemptions
    WHERE repo_slug = ?
      AND (kind = 'repo' OR (kind = 'app' AND app_name = ?))
      AND (expires_at IS NULL OR expires_at > strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
) AS is_exempt;

-- name: ExpirePeriodExemptions :exec
DELETE FROM exemptions
WHERE type = 'period' AND expires_at <= ?;
