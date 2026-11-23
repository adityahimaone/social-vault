CREATE TABLE IF NOT EXISTS bookmarks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    url TEXT NOT NULL UNIQUE,
    title TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    favicon_url TEXT NOT NULL DEFAULT '',
    site_name TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'manual',
    archived INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS tags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE COLLATE NOCASE,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS bookmark_tags (
    bookmark_id INTEGER NOT NULL,
    tag_id INTEGER NOT NULL,
    PRIMARY KEY (bookmark_id, tag_id),
    FOREIGN KEY (bookmark_id) REFERENCES bookmarks(id) ON DELETE CASCADE,
    FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS lists (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE COLLATE NOCASE,
    description TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS bookmark_lists (
    bookmark_id INTEGER NOT NULL,
    list_id INTEGER NOT NULL,
    PRIMARY KEY (bookmark_id, list_id),
    FOREIGN KEY (bookmark_id) REFERENCES bookmarks(id) ON DELETE CASCADE,
    FOREIGN KEY (list_id) REFERENCES lists(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS sessions (
    token_hash TEXT PRIMARY KEY,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL
);

CREATE VIRTUAL TABLE IF NOT EXISTS bookmarks_fts USING fts5(
    title,
    description,
    notes,
    url,
    content='bookmarks',
    content_rowid='id',
    tokenize='unicode61'
);

CREATE TRIGGER IF NOT EXISTS bookmarks_ai AFTER INSERT ON bookmarks BEGIN
    INSERT INTO bookmarks_fts(rowid, title, description, notes, url)
    VALUES (new.id, new.title, new.description, new.notes, new.url);
END;

CREATE TRIGGER IF NOT EXISTS bookmarks_ad AFTER DELETE ON bookmarks BEGIN
    INSERT INTO bookmarks_fts(bookmarks_fts, rowid, title, description, notes, url)
    VALUES ('delete', old.id, old.title, old.description, old.notes, old.url);
END;

CREATE TRIGGER IF NOT EXISTS bookmarks_au AFTER UPDATE ON bookmarks BEGIN
    INSERT INTO bookmarks_fts(bookmarks_fts, rowid, title, description, notes, url)
    VALUES ('delete', old.id, old.title, old.description, old.notes, old.url);
    INSERT INTO bookmarks_fts(rowid, title, description, notes, url)
    VALUES (new.id, new.title, new.description, new.notes, new.url);
END;

CREATE INDEX IF NOT EXISTS idx_bookmarks_archived_created ON bookmarks(archived, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_bookmark_tags_tag ON bookmark_tags(tag_id);
CREATE INDEX IF NOT EXISTS idx_bookmark_lists_list ON bookmark_lists(list_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);
