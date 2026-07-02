-- name: GetPlatformConfig :one
SELECT * FROM platform_config WHERE id = 1;

-- name: UpdatePlatformConfig :one
UPDATE platform_config
SET budget_ceiling_monthly   = ?,
    soft_limit_pct           = ?,
    hard_limit_pct           = ?,
    default_idle_after       = ?,
    budget_soft_limit_active = ?,
    budget_hard_limit_active = ?,
    billing_period           = ?
WHERE id = 1
RETURNING *;
