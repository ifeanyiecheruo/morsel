-- name: InsertScaleEvent :exec
INSERT INTO scale_events (namespace, app, event, occurred_at)
VALUES (?, ?, ?, ?);

-- name: ListScaleEventsSince :many
SELECT * FROM scale_events
WHERE namespace = ? AND app = ? AND occurred_at >= ?
ORDER BY occurred_at;

-- name: ListAllScaleEventsSince :many
SELECT * FROM scale_events
WHERE occurred_at >= ?
ORDER BY namespace, app, occurred_at;
