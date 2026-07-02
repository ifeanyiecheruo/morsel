-- name: InsertPriceSnapshot :one
INSERT INTO price_snapshots (compute_cpu_per_core_hour, compute_mem_per_gb_hour, storage_per_gb_month, registry_per_gb_month, fetched_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetLatestPriceSnapshot :one
SELECT * FROM price_snapshots
ORDER BY fetched_at DESC
LIMIT 1;

-- name: ListPriceSnapshots :many
SELECT * FROM price_snapshots
ORDER BY fetched_at DESC;
