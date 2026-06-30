CREATE TABLE IF NOT EXISTS quota (
	total_bytes INTEGER NOT NULL DEFAULT 0,
	limit_bytes INTEGER NOT NULL DEFAULT 0
);

INSERT OR IGNORE INTO quota(total_bytes, limit_bytes) VALUES (0, 0);
