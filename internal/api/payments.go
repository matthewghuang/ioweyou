package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/matthewghuang/ioweyou/internal/auth"
	"github.com/matthewghuang/ioweyou/internal/crdt"
)

func CreatePayment(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		member := auth.MemberFromContext(r.Context())
		if member == nil {
			respondError(w, 401, "unauthorized")
			return
		}

		slug := chi.URLParam(r, "slug")
		gid, err := resolveGroup(s.AuthDB, slug)
		if errors.Is(err, errNotFound) {
			respondError(w, 404, "group not found")
			return
		}
		if err != nil {
			respondError(w, 500, "db error")
			return
		}
		if !isGroupMember(s.AuthDB, gid, member.MemberID) {
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
		if body.FromUser != member.MemberID {
			respondError(w, 403, "you can only record payments from yourself")
			return
		}

		docID := uuid.New().String()
		ts := s.HLC.Now()

		// Store group_id
		if err := s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "group_id",
			Value: mustMarshal(gid), AuthorID: member.MemberID, Timestamp: ts,
		}); err != nil {
			respondError(w, 500, "failed to create payment")
			return
		}
		ts = s.HLC.Now()
		if err := s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "from_user",
			Value: mustMarshal(body.FromUser), AuthorID: member.MemberID, Timestamp: ts,
		}); err != nil {
			respondError(w, 500, "failed to create payment")
			return
		}
		ts = s.HLC.Now()
		if err := s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "to_user",
			Value: mustMarshal(body.ToUser), AuthorID: member.MemberID, Timestamp: ts,
		}); err != nil {
			respondError(w, 500, "failed to create payment")
			return
		}
		ts = s.HLC.Now()
		if err := s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "amount",
			Value: mustMarshal(body.Amount), AuthorID: member.MemberID, Timestamp: ts,
		}); err != nil {
			respondError(w, 500, "failed to create payment")
			return
		}
		ts = s.HLC.Now()
		if err := s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "method",
			Value: mustMarshal(body.Method), AuthorID: member.MemberID, Timestamp: ts,
		}); err != nil {
			respondError(w, 500, "failed to create payment")
			return
		}
		ts = s.HLC.Now()
		if err := s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "status",
			Value: mustMarshal("pending"), AuthorID: member.MemberID, Timestamp: ts,
		}); err != nil {
			respondError(w, 500, "failed to create payment")
			return
		}
		ts = s.HLC.Now()
		if err := s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "created_at",
			Value: mustMarshal(time.Now().UnixMilli()), AuthorID: member.MemberID, Timestamp: ts,
		}); err != nil {
			respondError(w, 500, "failed to create payment")
			return
		}

		state, err := s.Store.GetLatestState(docID)
		if err != nil {
			respondError(w, 500, "failed to read state")
			return
		}
		state["id"] = docID
		respondJSON(w, 201, state)

		// Broadcast to group subscribers
		if s.Broadcaster != nil {
			if ops, err := s.Store.GetOps(docID, nil); err == nil && len(ops) > 0 {
				s.Broadcaster.Broadcast(gid, ops)
			}
		}
	}
}

func ListPayments(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		member := auth.MemberFromContext(r.Context())
		if member == nil {
			respondError(w, 401, "unauthorized")
			return
		}
		slug := chi.URLParam(r, "slug")
		gid, err := resolveGroup(s.AuthDB, slug)
		if errors.Is(err, errNotFound) {
			respondError(w, 404, "group not found")
			return
		}
		if err != nil {
			respondError(w, 500, "db error")
			return
		}
		if !isGroupMember(s.AuthDB, gid, member.MemberID) {
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
			// Skip non-payment docs (expenses also have group_id)
			if _, ok := state["from_user"]; !ok {
				continue
			}
			state["id"] = docID
			payments = append(payments, state)
		}
		if err := rows.Err(); err != nil {
			respondError(w, 500, "db error")
			return
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
		member := auth.MemberFromContext(r.Context())
		if member == nil {
			respondError(w, 401, "unauthorized")
			return
		}

		state, err := s.Store.GetLatestState(docID)
		if err != nil {
			respondError(w, 500, "db error")
			return
		}
		if len(state) == 0 {
			respondError(w, 404, "not found")
			return
		}
		if t, ok := state["tombstone"]; ok && t == true {
			respondError(w, 404, "not found")
			return
		}

		gid, _ := state["group_id"].(string)
		if !isGroupMember(s.AuthDB, gid, member.MemberID) {
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
		member := auth.MemberFromContext(r.Context())
		if member == nil {
			respondError(w, 401, "unauthorized")
			return
		}

		state, err := s.Store.GetLatestState(docID)
		if err != nil {
			respondError(w, 500, "db error")
			return
		}
		if len(state) == 0 {
			respondError(w, 404, "not found")
			return
		}
		if t, ok := state["tombstone"]; ok && t == true {
			respondError(w, 404, "not found")
			return
		}

		gid, _ := state["group_id"].(string)
		if !isGroupMember(s.AuthDB, gid, member.MemberID) {
			respondError(w, 403, "not a member")
			return
		}

		// Only the recipient can confirm
		toUser, _ := state["to_user"].(string)
		if toUser != member.MemberID {
			respondError(w, 403, "only the recipient can confirm")
			return
		}

		status, _ := state["status"].(string)
		if status == "confirmed" {
			respondError(w, 400, "already confirmed")
			return
		}

		ts := s.HLC.Now()
		if err := s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "status",
			Value: mustMarshal("confirmed"), AuthorID: member.MemberID, Timestamp: ts,
		}); err != nil {
			respondError(w, 500, "failed to confirm payment")
			return
		}
		ts = s.HLC.Now()
		if err := s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "confirmed_at",
			Value: mustMarshal(time.Now().UnixMilli()), AuthorID: member.MemberID, Timestamp: ts,
		}); err != nil {
			respondError(w, 500, "failed to confirm payment")
			return
		}
		ts = s.HLC.Now()
		if err := s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "confirmed_by",
			Value: mustMarshal(member.MemberID), AuthorID: member.MemberID, Timestamp: ts,
		}); err != nil {
			respondError(w, 500, "failed to confirm payment")
			return
		}

		state, err = s.Store.GetLatestState(docID)
		if err != nil {
			respondError(w, 500, "failed to read state")
			return
		}
		state["id"] = docID
		respondOK(w, state)

		// Broadcast to group subscribers
		if s.Broadcaster != nil {
			if ops, err := s.Store.GetOps(docID, nil); err == nil && len(ops) > 0 {
				gid, _ := state["group_id"].(string)
				if gid != "" {
					s.Broadcaster.Broadcast(gid, ops)
				}
			}
		}
	}
}

func CancelPayment(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docID := chi.URLParam(r, "id")
		member := auth.MemberFromContext(r.Context())
		if member == nil {
			respondError(w, 401, "unauthorized")
			return
		}

		state, err := s.Store.GetLatestState(docID)
		if err != nil {
			respondError(w, 500, "db error")
			return
		}
		if len(state) == 0 {
			respondError(w, 404, "not found")
			return
		}
		if t, ok := state["tombstone"]; ok && t == true {
			respondError(w, 404, "not found")
			return
		}

		gid, _ := state["group_id"].(string)
		if !isGroupMember(s.AuthDB, gid, member.MemberID) {
			respondError(w, 403, "not a member")
			return
		}

		// Only the sender or recipient can cancel
		fromUser, _ := state["from_user"].(string)
		toUser, _ := state["to_user"].(string)
		if member.MemberID != fromUser && member.MemberID != toUser {
			respondError(w, 403, "only the sender or recipient can cancel")
			return
		}

		if err := s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "tombstone",
			Value: mustMarshal(true), AuthorID: member.MemberID, Timestamp: s.HLC.Now(),
		}); err != nil {
			respondError(w, 500, "failed to cancel payment")
			return
		}

		respondOK(w, nil)

		// Broadcast to group subscribers
		if s.Broadcaster != nil {
			if ops, err := s.Store.GetOps(docID, nil); err == nil && len(ops) > 0 {
				gid, _ := state["group_id"].(string)
				if gid != "" {
					s.Broadcaster.Broadcast(gid, ops)
				}
			}
		}
	}
}
