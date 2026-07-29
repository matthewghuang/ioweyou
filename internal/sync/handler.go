package sync

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"

	"github.com/matthewghuang/ioweyou/internal/auth"
	"github.com/matthewghuang/ioweyou/internal/crdt"
)

// PullRequest is the JSON body for the pull endpoint.
type PullRequest struct {
	ClientID string                    `json:"client_id"` // reserved for future use
	DocID    string                    `json:"doc_id,omitempty"`
	Cursors  map[string]crdt.Timestamp `json:"cursors,omitempty"`
}

// PullResponse is returned by the pull endpoint.
type PullResponse struct {
	Operations []crdt.Operation          `json:"operations"`
	Cursors    map[string]crdt.Timestamp `json:"cursors"`
}

// PushRequest is the JSON body for the push endpoint.
type PushRequest struct {
	Operations []crdt.Operation `json:"operations"`
}

// PushResponse is returned by the push endpoint.
type PushResponse struct {
	Accepted int `json:"accepted"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// client connection gone — nothing to do
	}
}

// HandlePull handles POST /api/sync/pull.
// It reads the client's cursors (and optionally a doc_id scope), queries all
// matching operations, and returns those newer than the cursors along with
// updated per-author cursor timestamps.
func HandlePull(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req PullRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		member := auth.MemberFromContext(r.Context())
		if member == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		if req.DocID == "" {
			writeJSON(w, http.StatusOK, PullResponse{Operations: nil, Cursors: make(map[string]crdt.Timestamp)})
			return
		}

		// Resolve the group that owns this document and verify membership.
		groupID := resolveDocGroup(db, req.DocID)
		if groupID == "" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM members WHERE group_id = ? AND member_id = ?", groupID, member.MemberID).Scan(&count); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "membership check failed"})
			return
		}
		if count == 0 {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}

		var rows *sql.Rows
		var err error
		rows, err = db.Query(
			`SELECT doc_id, op_type, field, value, item_id, prev_item_id, author_id, wall_time, logical
			 FROM crdt_operations WHERE doc_id = ?
			 ORDER BY wall_time, logical, author_id`, req.DocID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
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
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
				return
			}
			op.OpType = crdt.OpType(opType)
			op.Value = json.RawMessage(val)
			allOps = append(allOps, op)
		}
		if err := rows.Err(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "rows error"})
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

		writeJSON(w, http.StatusOK, PullResponse{Operations: filtered, Cursors: cursors})
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
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		member := auth.MemberFromContext(r.Context())
		if member == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		// ---- Auth check BEFORE inserting any data ----
		// Determine affected groups from the operations themselves (handles new
		// documents whose group_id is set in the same push batch) and from the
		// DB (for existing documents).
		groupSet := make(map[string]bool)
		for _, op := range req.Operations {
			if op.OpType == crdt.OpLWW && op.Field == "name" {
				groupSet[op.DocID] = true
			}
			if op.OpType == crdt.OpLWW && op.Field == "group_id" {
				var gid string
				if err := json.Unmarshal(op.Value, &gid); err == nil && gid != "" {
					groupSet[gid] = true
				}
			}
		}
		// Also check DB for any existing docs not resolved from operations.
		for _, op := range req.Operations {
			if !groupSet[op.DocID] {
				if gid := resolveDocGroup(db, op.DocID); gid != "" {
					groupSet[gid] = true
				}
			}
		}
		if len(groupSet) == 0 {
			log.Printf("push auth: could not determine group for operations")
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "could not determine group"})
			return
		}

		for gid := range groupSet {
			var count int
			if err := db.QueryRow("SELECT COUNT(*) FROM members WHERE group_id = ? AND member_id = ?", gid, member.MemberID).Scan(&count); err != nil {
				log.Printf("push auth check error: %v", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "membership check failed"})
				return
			}
			if count == 0 {
				log.Printf("push auth: member %s not in group %s", member.MemberID, gid)
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
				return
			}
		}

		var accepted []crdt.Operation
		docIDs := make(map[string]bool)
		for _, op := range req.Operations {
			hlc.Observe(op.Timestamp)

			res, err := db.Exec(
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
			n, err := res.RowsAffected()
			if err != nil {
				log.Printf("push rows affected error: %v", err)
				continue
			}
			if n == 0 {
				continue // duplicate, skip
			}
			docIDs[op.DocID] = true
			accepted = append(accepted, op)
		}

		// Broadcast accepted operations to subscribers of relevant groups.
		if len(accepted) > 0 && bcast != nil {
			groups := resolvePushGroups(db, docIDs)
			for _, gid := range groups {
				bcast.Broadcast(gid, accepted)
			}
		}

		writeJSON(w, http.StatusOK, PushResponse{Accepted: len(accepted)})
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
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM crdt_operations
			 WHERE doc_id = ? AND op_type = 'lww' AND field = 'name'`, docID).Scan(&hasName); err != nil {
			continue
		}
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

// resolveDocGroup resolves the group that owns a single document.
// Returns the group ID, or empty string if it cannot be determined.
func resolveDocGroup(db *sql.DB, docID string) string {
	// Check if this doc is a group document (has a "name" field).
	var hasName int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM crdt_operations
		 WHERE doc_id = ? AND op_type = 'lww' AND field = 'name'`, docID).Scan(&hasName); err != nil {
		return ""
	}
	if hasName > 0 {
		return docID
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
		return ""
	}

	var gid string
	if err := json.Unmarshal([]byte(rawValue), &gid); err != nil {
		return ""
	}
	return gid
}
