package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/matthewghuang/ioweyou/internal/auth"
	"github.com/matthewghuang/ioweyou/internal/crdt"
)

func mustMarshal(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func getGroupMembers(db *sql.DB, groupID string) []string {
	rows, err := db.Query("SELECT user_id FROM group_members WHERE group_id = ?", groupID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var members []string
	for rows.Next() {
		var uid string
		if rows.Scan(&uid) == nil {
			members = append(members, uid)
		}
	}
	return members
}

func isGroupMember(db *sql.DB, groupID, userID string) bool {
	var count int
	db.QueryRow("SELECT COUNT(*) FROM group_members WHERE group_id = ? AND user_id = ?", groupID, userID).Scan(&count)
	return count > 0
}

func CreateGroup(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())
		if user == nil {
			respondError(w, 401, "unauthorized")
			return
		}

		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
			respondError(w, 400, "name required")
			return
		}

		docID := uuid.New().String()
		now := time.Now().UnixNano()
		ts := s.HLC.Now()

		s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "name",
			Value: mustMarshal(body.Name), AuthorID: user.ID, Timestamp: ts,
		})
		ts = s.HLC.Now()
		s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "created_by",
			Value: mustMarshal(user.ID), AuthorID: user.ID, Timestamp: ts,
		})
		ts = s.HLC.Now()
		s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "created_at",
			Value: mustMarshal(now), AuthorID: user.ID, Timestamp: ts,
		})

		// Add creator to group_members (non-CRDT)
		s.AuthDB.Exec("INSERT OR IGNORE INTO group_members (group_id, user_id) VALUES (?, ?)", docID, user.ID)

		state, _ := s.Store.GetLatestState(docID)
		state["id"] = docID
		state["members"] = getGroupMembers(s.AuthDB, docID)

		respondJSON(w, 201, state)
	}
}

func ListGroups(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())
		if user == nil {
			respondError(w, 401, "unauthorized")
			return
		}

		rows, err := s.AuthDB.Query("SELECT group_id FROM group_members WHERE user_id = ?", user.ID)
		if err != nil {
			respondError(w, 500, "db error")
			return
		}
		defer rows.Close()

		var groups []map[string]any
		for rows.Next() {
			var gid string
			if err := rows.Scan(&gid); err != nil {
				continue
			}
			state, err := s.Store.GetLatestState(gid)
			if err != nil || len(state) == 0 {
				continue
			}
			state["id"] = gid
			state["members"] = getGroupMembers(s.AuthDB, gid)
			groups = append(groups, state)
		}
		if groups == nil {
			groups = []map[string]any{}
		}
		respondOK(w, groups)
	}
}

func GetGroup(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gid := chi.URLParam(r, "id")
		user := auth.UserFromContext(r.Context())
		if user == nil {
			respondError(w, 401, "unauthorized")
			return
		}
		if !isGroupMember(s.AuthDB, gid, user.ID) {
			respondError(w, 403, "not a member")
			return
		}

		state, err := s.Store.GetLatestState(gid)
		if err != nil || len(state) == 0 {
			respondError(w, 404, "not found")
			return
		}
		state["id"] = gid
		state["members"] = getGroupMembers(s.AuthDB, gid)
		respondOK(w, state)
	}
}

func UpdateGroup(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gid := chi.URLParam(r, "id")
		user := auth.UserFromContext(r.Context())
		if user == nil {
			respondError(w, 401, "unauthorized")
			return
		}
		if !isGroupMember(s.AuthDB, gid, user.ID) {
			respondError(w, 403, "not a member")
			return
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondError(w, 400, "invalid body")
			return
		}
		for k, v := range body {
			s.Store.Append(crdt.Operation{
				DocID: gid, OpType: crdt.OpLWW, Field: k,
				Value: mustMarshal(v), AuthorID: user.ID, Timestamp: s.HLC.Now(),
			})
		}

		state, _ := s.Store.GetLatestState(gid)
		state["id"] = gid
		state["members"] = getGroupMembers(s.AuthDB, gid)
		respondOK(w, state)
	}
}
