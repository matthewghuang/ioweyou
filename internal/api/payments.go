package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/matthewghuang/ioweyou/internal/auth"
	"github.com/matthewghuang/ioweyou/internal/crdt"
)

func CreatePayment(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())
		if user == nil {
			respondError(w, 401, "unauthorized")
			return
		}

		gid := chi.URLParam(r, "id")
		if !isGroupMember(s.AuthDB, gid, user.ID) {
			respondError(w, 403, "not a member")
			return
		}

		var body struct {
			FromUser string  `json:"from_user"`
			ToUser   string  `json:"to_user"`
			Amount   float64 `json:"amount"`
			Method   string  `json:"method,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.FromUser == "" || body.ToUser == "" || body.Amount <= 0 {
			respondError(w, 400, "invalid payment")
			return
		}

		docID := uuid.New().String()
		ts := s.HLC.Now()

		// Store group_id
		s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "group_id",
			Value: mustMarshal(gid), AuthorID: user.ID, Timestamp: ts,
		})
		ts = s.HLC.Now()
		s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "from_user",
			Value: mustMarshal(body.FromUser), AuthorID: user.ID, Timestamp: ts,
		})
		ts = s.HLC.Now()
		s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "to_user",
			Value: mustMarshal(body.ToUser), AuthorID: user.ID, Timestamp: ts,
		})
		ts = s.HLC.Now()
		s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "amount",
			Value: mustMarshal(body.Amount), AuthorID: user.ID, Timestamp: ts,
		})
		ts = s.HLC.Now()
		s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "method",
			Value: mustMarshal(body.Method), AuthorID: user.ID, Timestamp: ts,
		})
		ts = s.HLC.Now()
		s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "status",
			Value: mustMarshal("pending"), AuthorID: user.ID, Timestamp: ts,
		})
		ts = s.HLC.Now()
		s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "created_at",
			Value: mustMarshal(time.Now().UnixNano()), AuthorID: user.ID, Timestamp: ts,
		})

		state, _ := s.Store.GetLatestState(docID)
		state["id"] = docID
		respondJSON(w, 201, state)
	}
}

func ListPayments(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())
		if user == nil {
			respondError(w, 401, "unauthorized")
			return
		}
		gid := chi.URLParam(r, "id")
		if !isGroupMember(s.AuthDB, gid, user.ID) {
			respondError(w, 403, "not a member")
			return
		}

		rows, err := s.AuthDB.Query(
			"SELECT DISTINCT doc_id FROM crdt_operations WHERE op_type = 'lww' AND field = 'group_id' AND value = ?",
			string(mustMarshal(gid)),
		)
		if err != nil {
			respondError(w, 500, "db error")
			return
		}
		defer rows.Close()

		var payments []map[string]any
		for rows.Next() {
			var docID string
			if err := rows.Scan(&docID); err != nil {
				continue
			}
			state, err := s.Store.GetLatestState(docID)
			if err != nil || len(state) == 0 {
				continue
			}
			if t, ok := state["tombstone"]; ok && t == true {
				continue
			}
			state["id"] = docID
			payments = append(payments, state)
		}
		if payments == nil {
			payments = []map[string]any{}
		}
		respondOK(w, payments)
	}
}

func GetPayment(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docID := chi.URLParam(r, "id")
		user := auth.UserFromContext(r.Context())
		if user == nil {
			respondError(w, 401, "unauthorized")
			return
		}

		state, err := s.Store.GetLatestState(docID)
		if err != nil || len(state) == 0 {
			respondError(w, 404, "not found")
			return
		}
		if t, ok := state["tombstone"]; ok && t == true {
			respondError(w, 404, "not found")
			return
		}

		gid, _ := state["group_id"].(string)
		if !isGroupMember(s.AuthDB, gid, user.ID) {
			respondError(w, 403, "not a member")
			return
		}

		state["id"] = docID
		respondOK(w, state)
	}
}

func ConfirmPayment(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docID := chi.URLParam(r, "id")
		user := auth.UserFromContext(r.Context())
		if user == nil {
			respondError(w, 401, "unauthorized")
			return
		}

		state, err := s.Store.GetLatestState(docID)
		if err != nil || len(state) == 0 {
			respondError(w, 404, "not found")
			return
		}
		if t, ok := state["tombstone"]; ok && t == true {
			respondError(w, 404, "not found")
			return
		}

		// Only the recipient can confirm
		toUser, _ := state["to_user"].(string)
		if toUser != user.ID {
			respondError(w, 403, "only the recipient can confirm")
			return
		}

		status, _ := state["status"].(string)
		if status == "confirmed" {
			respondError(w, 400, "already confirmed")
			return
		}

		ts := s.HLC.Now()
		s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "status",
			Value: mustMarshal("confirmed"), AuthorID: user.ID, Timestamp: ts,
		})
		ts = s.HLC.Now()
		s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "confirmed_at",
			Value: mustMarshal(time.Now().UnixNano()), AuthorID: user.ID, Timestamp: ts,
		})
		ts = s.HLC.Now()
		s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "confirmed_by",
			Value: mustMarshal(user.ID), AuthorID: user.ID, Timestamp: ts,
		})

		state, _ = s.Store.GetLatestState(docID)
		state["id"] = docID
		respondOK(w, state)
	}
}

func CancelPayment(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docID := chi.URLParam(r, "id")
		user := auth.UserFromContext(r.Context())
		if user == nil {
			respondError(w, 401, "unauthorized")
			return
		}

		state, err := s.Store.GetLatestState(docID)
		if err != nil || len(state) == 0 {
			respondError(w, 404, "not found")
			return
		}
		if t, ok := state["tombstone"]; ok && t == true {
			respondError(w, 404, "not found")
			return
		}

		gid, _ := state["group_id"].(string)
		if !isGroupMember(s.AuthDB, gid, user.ID) {
			respondError(w, 403, "not a member")
			return
		}

		s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "tombstone",
			Value: mustMarshal(true), AuthorID: user.ID, Timestamp: s.HLC.Now(),
		})

		respondOK(w, nil)
	}
}
