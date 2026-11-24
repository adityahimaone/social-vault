CREATE TABLE IF NOT EXISTS tag_review_queue (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    bookmark_id INTEGER NOT NULL,
    suggested_tags TEXT NOT NULL DEFAULT '[]',
    jev_scores TEXT NOT NULL DEFAULT '{}',
    source TEXT NOT NULL DEFAULT 'jev',
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'applied', 'dismissed')),
    created_at TEXT NOT NULL,
    resolved_at TEXT,
    FOREIGN KEY (bookmark_id) REFERENCES bookmarks(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_tag_review_queue_pending_bookmark
    ON tag_review_queue(bookmark_id)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_tag_review_queue_status_created
    ON tag_review_queue(status, created_at DESC);
