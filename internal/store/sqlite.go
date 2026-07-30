package store

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/matthewghuang/ioweyou/internal/crdt"

	_ "modernc.org/sqlite"
)

type SQLiteStateStore struct {
	db *sql.DB
}

func NewSQLiteStateStore(dbPath string) (*SQLiteStateStore, error) {
	dsn := dbPath + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := InitSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("schema: %w", err)
	}
	// SQLite only supports one writer at a time
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	return &SQLiteStateStore{db: db}, nil
}

func (s *SQLiteStateStore) Close() error {
	return s.db.Close()
}

// DB returns the underlying *sql.DB for direct queries (sync, auth).
func (s *SQLiteStateStore) DB() *sql.DB {
	return s.db
}

func (s *SQLiteStateStore) Append(op crdt.Operation) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO crdt_operations
		(doc_id, op_type, field, value, item_id, prev_item_id, author_id, wall_time, logical)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		op.DocID, string(op.OpType), op.Field, string(op.Value),
		op.ItemID, op.PrevItemID, op.AuthorID,
		op.Timestamp.WallTime, op.Timestamp.Logical,
	)
	return err
}

func (s *SQLiteStateStore) GetOps(docID string, since map[string]crdt.Timestamp) ([]crdt.Operation, error) {
	rows, err := s.db.Query(
		`SELECT doc_id, op_type, field, value, item_id, prev_item_id, author_id, wall_time, logical
		 FROM crdt_operations WHERE doc_id = ?
		 ORDER BY wall_time, logical, author_id`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ops []crdt.Operation
	for rows.Next() {
		var op crdt.Operation
		var val string
		var opType string
		err := rows.Scan(&op.DocID, &opType, &op.Field, &val, &op.ItemID, &op.PrevItemID,
			&op.AuthorID, &op.Timestamp.WallTime, &op.Timestamp.Logical)
		if err != nil {
			return nil, err
		}
		op.OpType = crdt.OpType(opType)
		op.Value = json.RawMessage(val)
		ops = append(ops, op)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if since == nil {
		return ops, nil
	}

	// Filter: keep ops where no cursor exists for the author, or timestamp > cursor
	var filtered []crdt.Operation
	for _, op := range ops {
		cursor, ok := since[op.AuthorID]
		if !ok {
			filtered = append(filtered, op)
			continue
		}
		// op is "after" cursor if (wall_time, logical) > (cursor.WallTime, cursor.Logical)
		if op.Timestamp.WallTime > cursor.WallTime ||
			(op.Timestamp.WallTime == cursor.WallTime && op.Timestamp.Logical > cursor.Logical) {
			filtered = append(filtered, op)
		}
	}
	return filtered, nil
}

func (s *SQLiteStateStore) GetLatestState(docID string) (map[string]any, error) {
	ops, err := s.GetOps(docID, nil)
	if err != nil {
		return nil, err
	}
	merged := crdt.MergeState(ops, nil)
	return crdt.Snapshot(merged), nil
}

func (s *SQLiteStateStore) GetVersionVector(docID string) (map[string]crdt.Timestamp, error) {
	rows, err := s.db.Query(
		`SELECT author_id, wall_time, logical FROM crdt_operations
		 WHERE doc_id = ?
		 AND (wall_time > 0 OR logical > 0)
		 ORDER BY wall_time DESC, logical DESC`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	vv := make(map[string]crdt.Timestamp)
	for rows.Next() {
		var authorID string
		var ts crdt.Timestamp
		if err := rows.Scan(&authorID, &ts.WallTime, &ts.Logical); err != nil {
			return nil, err
		}
		// Only take the first (highest timestamp) per author
		if _, exists := vv[authorID]; !exists {
			vv[authorID] = ts
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return vv, nil
}
