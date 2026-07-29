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
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	// Enable WAL mode and foreign keys
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("pragma: %w", err)
		}
	}
	if err := InitSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("schema: %w", err)
	}
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
	return crdt.Snapshot(ops), nil
}

func (s *SQLiteStateStore) GetVersionVector(docID string) (map[string]crdt.Timestamp, error) {
	rows, err := s.db.Query(
		`SELECT author_id, MAX(wall_time), MAX(logical)
		 FROM crdt_operations WHERE doc_id = ?
		 GROUP BY author_id`, docID)
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
		vv[authorID] = ts
	}
	return vv, rows.Err()
}
