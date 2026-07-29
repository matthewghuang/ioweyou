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

// SplitInput represents one participant's share in an expense.
type SplitInput struct {
	UserID string  `json:"user_id"`
	Amount float64 `json:"amount"`
}

func CreateExpense(s *Server) http.HandlerFunc {
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
		s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "group_id",
			Value: mustMarshal(gid), AuthorID: user.ID, Timestamp: ts,
		})
		ts = s.HLC.Now()
		s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "description",
			Value: mustMarshal(body.Description), AuthorID: user.ID, Timestamp: ts,
		})
		ts = s.HLC.Now()
		s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "amount",
			Value: mustMarshal(body.Amount), AuthorID: user.ID, Timestamp: ts,
		})
		ts = s.HLC.Now()
		s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "paid_by",
			Value: mustMarshal(user.ID), AuthorID: user.ID, Timestamp: ts,
		})
		ts = s.HLC.Now()
		s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "split_type",
			Value: mustMarshal(body.SplitType), AuthorID: user.ID, Timestamp: ts,
		})
		ts = s.HLC.Now()
		s.Store.Append(crdt.Operation{
			DocID: docID, OpType: crdt.OpLWW, Field: "created_at",
			Value: mustMarshal(time.Now().UnixNano()), AuthorID: user.ID, Timestamp: ts,
		})

		// RGA inserts for splits
		var prevItemID string
		if body.SplitType == "custom" {
			for _, sp := range body.Splits {
				ts = s.HLC.Now()
				itemID := uuid.New().String()
				spVal, _ := json.Marshal(sp)
				s.Store.Append(crdt.Operation{
					DocID: docID, OpType: crdt.OpRGAInsert, Field: "splits",
					Value: spVal, ItemID: itemID, PrevItemID: prevItemID,
					AuthorID: user.ID, Timestamp: ts,
				})
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
				spVal, _ := json.Marshal(SplitInput{UserID: m, Amount: splitAmt})
				s.Store.Append(crdt.Operation{
					DocID: docID, OpType: crdt.OpRGAInsert, Field: "splits",
					Value: spVal, ItemID: itemID, PrevItemID: prevItemID,
					AuthorID: user.ID, Timestamp: ts,
				})
				prevItemID = itemID
			}
		}

		state, _ := s.Store.GetLatestState(docID)
		state["id"] = docID
		respondJSON(w, 201, state)
	}
}

func ListExpenses(s *Server) http.HandlerFunc {
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
			// Skip tombstoned expenses
			if t, ok := state["tombstone"]; ok && t == true {
				continue
			}
			state["id"] = docID
			expenses = append(expenses, state)
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

func UpdateExpense(s *Server) http.HandlerFunc {
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

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondError(w, 400, "invalid body")
			return
		}

		// Handle LWW field updates
		for k, v := range body {
			if k == "splits" {
				continue // handled below
			}
			s.Store.Append(crdt.Operation{
				DocID: docID, OpType: crdt.OpLWW, Field: k,
				Value: mustMarshal(v), AuthorID: user.ID, Timestamp: s.HLC.Now(),
			})
		}

		// Handle split replacement via RGA delete + insert
		if splitsRaw, ok := body["splits"]; ok {
			newSplits, ok := splitsRaw.([]any)
			if !ok {
				respondError(w, 400, "splits must be an array")
				return
			}

			// Delete existing splits
			existingOps, _ := s.Store.GetOps(docID, nil)
			for _, op := range existingOps {
				if op.OpType == crdt.OpRGAInsert && op.Field == "splits" {
					s.Store.Append(crdt.Operation{
						DocID: docID, OpType: crdt.OpRGADelete, Field: "splits",
						Value: mustMarshal(op.ItemID), ItemID: op.ItemID,
						AuthorID: user.ID, Timestamp: s.HLC.Now(),
					})
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
				spVal, _ := json.Marshal(spMap)
				s.Store.Append(crdt.Operation{
					DocID: docID, OpType: crdt.OpRGAInsert, Field: "splits",
					Value: spVal, ItemID: itemID, PrevItemID: prevItemID,
					AuthorID: user.ID, Timestamp: s.HLC.Now(),
				})
				prevItemID = itemID
			}
		}

		state, _ = s.Store.GetLatestState(docID)
		state["id"] = docID
		respondOK(w, state)
	}
}

func DeleteExpense(s *Server) http.HandlerFunc {
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
