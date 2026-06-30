-- name: InsertMessage :exec
INSERT INTO messages(id, body, body_size, enqueued_at) VALUES (?, ?, ?, ?);

-- name: SelectNextMessage :one
SELECT id, body, enqueued_at FROM messages
WHERE visibility_until IS NULL
   OR visibility_until < strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
ORDER BY enqueued_at ASC LIMIT 1;

-- name: SetVisibility :exec
UPDATE messages SET visibility_until = ? WHERE id = ?;

-- name: GetMessageBodySize :one
SELECT body_size FROM messages WHERE id = ?;

-- name: DeleteMessage :exec
DELETE FROM messages WHERE id = ?;

-- name: CountMessages :one
SELECT COUNT(*) FROM messages;
