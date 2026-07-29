-- CRDT operations log (append-only)
CREATE TABLE IF NOT EXISTS crdt_operations (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    doc_id      TEXT NOT NULL,
    op_type     TEXT NOT NULL,              -- 'lww' | 'rga_insert' | 'rga_delete'
    field       TEXT,
    value       TEXT NOT NULL,              -- JSON-encoded value
    item_id     TEXT,
    prev_item_id TEXT,
    author_id   TEXT NOT NULL,
    wall_time   INTEGER NOT NULL,
    logical     INTEGER NOT NULL,
    UNIQUE(author_id, wall_time, logical)
);
CREATE INDEX IF NOT EXISTS idx_ops_doc ON crdt_operations(doc_id);
CREATE INDEX IF NOT EXISTS idx_ops_author ON crdt_operations(author_id, wall_time, logical);

-- Users
CREATE TABLE IF NOT EXISTS users (
    id       TEXT PRIMARY KEY,
    name     TEXT NOT NULL,
    api_key  TEXT NOT NULL UNIQUE
);

-- Group membership (non-CRDT)
CREATE TABLE IF NOT EXISTS group_members (
    group_id TEXT NOT NULL,
    user_id  TEXT NOT NULL,
    PRIMARY KEY (group_id, user_id)
);

-- Sync state per client
CREATE TABLE IF NOT EXISTS sync_cursors (
    client_id      TEXT NOT NULL,
    author_id      TEXT NOT NULL,
    last_wall_time INTEGER NOT NULL DEFAULT 0,
    last_logical   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (client_id, author_id)
);
