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
    UNIQUE(doc_id, author_id, wall_time, logical)
);
CREATE INDEX IF NOT EXISTS idx_ops_doc ON crdt_operations(doc_id);
CREATE INDEX IF NOT EXISTS idx_ops_author ON crdt_operations(author_id, wall_time, logical);

-- Group slugs (human-readable identifiers)
CREATE TABLE IF NOT EXISTS group_slugs (
    slug     TEXT PRIMARY KEY,
    group_id TEXT NOT NULL UNIQUE
);

-- Group members (replaces users + group_members)
CREATE TABLE IF NOT EXISTS members (
    member_id    TEXT PRIMARY KEY,
    group_id     TEXT NOT NULL,
    user_name    TEXT NOT NULL,
    secret_hash  TEXT NOT NULL,
    cookie_token TEXT NOT NULL UNIQUE,
    UNIQUE(group_id, user_name)
);


