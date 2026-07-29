package sync

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"

	"github.com/matthewghuang/ioweyou/internal/crdt"
)

// PullRequest is the JSON body for the pull endpoint.
type PullRequest struct {
	ClientID string                     `json:"client_id"`
	DocID    string                     `json:"doc_id,omitempty"`
	Cursors  map[string]crdt.Timestamp  `json:"cursors,omitempty"`
}

// PullResponse is returned by the pull endpoint.
type PullResponse struct {
	Operations []crdt.Operation           `json:"operations"`
	Cursors    map[string]crdt.Timestamp  `json:"cursors"`
}

// PushRequest is the JSON body for the push endpoint.
type PushRequest struct {
	Operations []crdt.Operation `json:"operations"`
}

// PushResponse is returned by the push endpoint.
type PushResponse struct {
	Accepted int `json:"accepted"`
}

// HandlePull handles POST /api/sync/pull.
// It reads the client's cursors (and optionally a doc_id scope), queries all
// matching operations, and returns those newer than the cursors along with
// updated per-author cursor timestamps.
func HandlePull(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req PullRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
			return
		}

		var rows *sql.Rows
		var err error
		if req.DocID != "" {
			rows, err = db.Query(
				`SELECT doc_id, op_type, field, value, item_id, prev_item_id, author_id, wall_time, logical
				 FROM crdt_operations WHERE doc_id = ?
				 ORDER BY wall_time, logical, author_id`, req.DocID)
		} else {
			rows, err = db.Query(
				`SELECT doc_id, op_type, field, value, item_id, prev_item_id, author_id, wall_time, logical
				 FROM crdt_operations
				 ORDER BY wall_time, logical, author_id`)
		}
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "query failed"})
			return
		}
		defer rows.Close()

		var allOps []crdt.Operation
		for rows.Next() {
			var op crdt.Operation
			var val string
			var opType string
			err := rows.Scan(
				&op.DocID, &opType, &op.Field, &val,
				&op.ItemID, &op.PrevItemID, &op.AuthorID,
				&op.Timestamp.WallTime, &op.Timestamp.Logical,
			)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "scan failed"})
				return
			}
			op.OpType = crdt.OpType(opType)
			op.Value = json.RawMessage(val)
			allOps = append(allOps, op)
		}
		if err := rows.Err(); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "rows error"})
			return
		}

		// Filter: keep ops where no cursor exists for the author, or the op's
		// timestamp is strictly after the cursor.
		var filtered []crdt.Operation
		for _, op := range allOps {
			cursor, ok := req.Cursors[op.AuthorID]
			if !ok {
				filtered = append(filtered, op)
				continue
			}
			if op.Timestamp.WallTime > cursor.WallTime ||
				(op.Timestamp.WallTime == cursor.WallTime && op.Timestamp.Logical > cursor.Logical) {
				filtered = append(filtered, op)
			}
		}

		// Build updated cursors (max per author from the returned operations).
		cursors := make(map[string]crdt.Timestamp)
		for _, op := range filtered {
			cur := cursors[op.AuthorID]
			if op.Timestamp.WallTime > cur.WallTime ||
				(op.Timestamp.WallTime == cur.WallTime && op.Timestamp.Logical > cur.Logical) {
				cursors[op.AuthorID] = op.Timestamp
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PullResponse{Operations: filtered, Cursors: cursors})
	}
}

// HandlePush handles POST /api/sync/push.
// It appends each incoming operation (deduped by the UNIQUE constraint on
// author_id, wall_time, logical), then broadcasts newly accepted operations
// to WebSocket subscribers of the relevant groups.
func HandlePush(db *sql.DB, hlc *crdt.HLC, bcast *Broadcaster) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req PushRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
			return
		}

		accepted := 0
		docIDs := make(map[string]bool)
		for _, op := range req.Operations {
			hlc.Observe(op.Timestamp)

			_, err := db.Exec(
				`INSERT OR IGNORE INTO crdt_operations
				 (doc_id, op_type, field, value, item_id, prev_item_id, author_id, wall_time, logical)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				op.DocID, string(op.OpType), op.Field, string(op.Value),
				op.ItemID, op.PrevItemID, op.AuthorID,
				op.Timestamp.WallTime, op.Timestamp.Logical,
			)
			if err != nil {
				log.Printf("push append error: %v", err)
				continue
			}
			docIDs[op.DocID] = true
			accepted++
		}

		// Broadcast accepted operations to subscribers of relevant groups.
		if accepted > 0 {
			groups := resolvePushGroups(db, docIDs)
			for _, gid := range groups {
				bcast.Broadcast(gid, req.Operations)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PushResponse{Accepted: accepted})
	}
}

// resolvePushGroups determines which group_ids should receive notifications
// by querying the DB for each document's group association.
func resolvePushGroups(db *sql.DB, docIDs map[string]bool) []string {
	seen := make(map[string]bool)
	var groups []string

	for docID := range docIDs {
		// Check if this doc is a group document (has a "name" field)
		var hasName int
		db.QueryRow(
			`SELECT COUNT(*) FROM crdt_operations
			 WHERE doc_id = ? AND op_type = 'lww' AND field = 'name'`, docID).Scan(&hasName)
		if hasName > 0 {
			if !seen[docID] {
				seen[docID] = true
				groups = append(groups, docID)
			}
			continue
		}

		// For child documents (expenses, payments), look up the "group_id"
		// field from the latest LWW operation that sets it.
		var rawValue string
		err := db.QueryRow(
			`SELECT value FROM crdt_operations
			 WHERE doc_id = ? AND op_type = 'lww' AND field = 'group_id'
			 ORDER BY wall_time DESC, logical DESC
			 LIMIT 1`, docID).Scan(&rawValue)
		if err != nil {
			continue
		}

		var gid string
		if err := json.Unmarshal([]byte(rawValue), &gid); err != nil {
			continue
		}
		if gid != "" && !seen[gid] {
			seen[gid] = true
			groups = append(groups, gid)
		}
	}

	return groups
}
