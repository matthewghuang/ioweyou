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

// SplitInput represents one participant's share in an expense.
type SplitInput struct {
	UserID string  `json:"user_id"`
	Amount float64 `json:"amount"`
}

func CreateExpense(s *Server) http.HandlerFunc {
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
			Description string       `json:"description"`
			Amount      float64      `json:"amount"`
			SplitType   string       `json:"split_type"`
			Splits      []SplitInput `json:"splits,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Description == "" || body.Amount <= 0 {
			respondError(w, 400, "invalid expense")
			return
		}
		if body.SplitType == "" {
			body.SplitType = "equal"
		}

		docID := uuid.New().String()
		ts := s.HLC.Now()

		// LWW fields
		if err := s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "group_id",
			Value: mustMarshal(gid), AuthorID: member.MemberID, Timestamp: ts,
		}); err != nil {
			respondError(w, 500, "failed to create expense")
			return
		}
		ts = s.HLC.Now()
		if err := s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "description",
			Value: mustMarshal(body.Description), AuthorID: member.MemberID, Timestamp: ts,
		}); err != nil {
			respondError(w, 500, "failed to create expense")
			return
		}
		ts = s.HLC.Now()
		if err := s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "amount",
			Value: mustMarshal(body.Amount), AuthorID: member.MemberID, Timestamp: ts,
		}); err != nil {
			respondError(w, 500, "failed to create expense")
			return
		}
		ts = s.HLC.Now()
		if err := s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "paid_by",
			Value: mustMarshal(member.MemberID), AuthorID: member.MemberID, Timestamp: ts,
		}); err != nil {
			respondError(w, 500, "failed to create expense")
			return
		}
		ts = s.HLC.Now()
		if err := s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "split_type",
			Value: mustMarshal(body.SplitType), AuthorID: member.MemberID, Timestamp: ts,
		}); err != nil {
			respondError(w, 500, "failed to create expense")
			return
		}
		ts = s.HLC.Now()
		if err := s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "created_at",
			Value: mustMarshal(time.Now().UnixMilli()), AuthorID: member.MemberID, Timestamp: ts,
		}); err != nil {
			respondError(w, 500, "failed to create expense")
			return
		}

		// RGA inserts for splits
		var prevItemID string
		if body.SplitType == "custom" {
			for _, sp := range body.Splits {
				ts = s.HLC.Now()
				itemID := uuid.New().String()
				spVal := mustMarshal(sp)
				if err := s.Store.Append(crdt.Operation{
					DocID: docID, OpType: crdt.OpRGAInsert, Field: "splits",
					Value: spVal, ItemID: itemID, PrevItemID: prevItemID,
					AuthorID: member.MemberID, Timestamp: ts,
				}); err != nil {
					respondError(w, 500, "failed to create expense")
					return
				}
				prevItemID = itemID
			}
		} else {
			// equal split: create a split for each group member
			members := getGroupMembers(s.AuthDB, gid)
			if len(members) == 0 {
				respondError(w, 500, "no group members")
				return
			}
			splitAmt := body.Amount / float64(len(members))
			for _, m := range members {
				ts = s.HLC.Now()
				itemID := uuid.New().String()
				spVal := mustMarshal(SplitInput{UserID: m["id"], Amount: splitAmt})
				if err := s.Store.Append(crdt.Operation{
					DocID: docID, OpType: crdt.OpRGAInsert, Field: "splits",
					Value: spVal, ItemID: itemID, PrevItemID: prevItemID,
					AuthorID: member.MemberID, Timestamp: ts,
				}); err != nil {
					respondError(w, 500, "failed to create expense")
					return
				}
				prevItemID = itemID
			}
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

func ListExpenses(s *Server) http.HandlerFunc {
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

		// Query ops table for expense doc_ids that belong to this group
		rows, err := s.AuthDB.Query(
			"SELECT DISTINCT doc_id FROM crdt_operations WHERE op_type = 'lww' AND field = 'group_id' AND value = ?",
			string(mustMarshal(gid)),
		)
		if err != nil {
			respondError(w, 500, "db error")
			return
		}
		defer rows.Close()

		var expenses []map[string]any
		for rows.Next() {
			var docID string
			if err := rows.Scan(&docID); err != nil {
				continue
			}
			state, err := s.Store.GetLatestState(docID)
			if err != nil || len(state) == 0 {
				continue
			}
			// Skip non-expense docs (payments also have group_id)
			if _, ok := state["description"]; !ok {
				continue
			}
			// Skip tombstoned expenses
			if t, ok := state["tombstone"]; ok && t == true {
				continue
			}
			state["id"] = docID
			expenses = append(expenses, state)
		}
		if err := rows.Err(); err != nil {
			respondError(w, 500, "db error")
			return
		}
		if expenses == nil {
			expenses = []map[string]any{}
		}
		respondOK(w, expenses)
	}
}

func GetExpense(s *Server) http.HandlerFunc {
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

func UpdateExpense(s *Server) http.HandlerFunc {
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

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondError(w, 400, "invalid body")
			return
		}

		// Handle LWW field updates (whitelist only mutable expense fields)
		allowedFields := map[string]bool{
			"description": true,
			"amount":      true,
			"split_type":  true,
		}
		for k, v := range body {
			if !allowedFields[k] {
				continue
			}
			if err := s.Store.Append(crdt.Operation{
				DocID: docID, OpType: crdt.OpLWW, Field: k,
				Value: mustMarshal(v), AuthorID: member.MemberID, Timestamp: s.HLC.Now(),
			}); err != nil {
				respondError(w, 500, "failed to update expense")
				return
			}
		}

		// Handle split replacement via RGA delete + insert
		if splitsRaw, ok := body["splits"]; ok {
			newSplits, ok := splitsRaw.([]any)
			if !ok {
				respondError(w, 400, "splits must be an array")
				return
			}

			// Delete existing splits
			existingOps, err := s.Store.GetOps(docID, nil)
			if err != nil {
				respondError(w, 500, "failed to fetch existing ops")
				return
			}
			for _, op := range existingOps {
				if op.OpType == crdt.OpRGAInsert && op.Field == "splits" {
					if err := s.Store.Append(crdt.Operation{
						DocID: docID, OpType: crdt.OpRGADelete, Field: "splits",
						Value: mustMarshal(op.ItemID), ItemID: op.ItemID,
						AuthorID: member.MemberID, Timestamp: s.HLC.Now(),
					}); err != nil {
						respondError(w, 500, "failed to update expense")
						return
					}
				}
			}

			// Insert new splits
			var prevItemID string
			for _, sp := range newSplits {
				spMap, ok := sp.(map[string]any)
				if !ok {
					continue
				}
				itemID := uuid.New().String()
				spVal := mustMarshal(spMap)
				if err := s.Store.Append(crdt.Operation{
					DocID: docID, OpType: crdt.OpRGAInsert, Field: "splits",
					Value: spVal, ItemID: itemID, PrevItemID: prevItemID,
					AuthorID: member.MemberID, Timestamp: s.HLC.Now(),
				}); err != nil {
					respondError(w, 500, "failed to update expense")
					return
				}
				prevItemID = itemID
			}
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

func DeleteExpense(s *Server) http.HandlerFunc {
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

		if err := s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "tombstone",
			Value: mustMarshal(true), AuthorID: member.MemberID, Timestamp: s.HLC.Now(),
		}); err != nil {
			respondError(w, 500, "failed to delete expense")
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
