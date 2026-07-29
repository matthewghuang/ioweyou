package api

import (
	"math"
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"
	"github.com/matthewghuang/ioweyou/internal/auth"
)

// BalanceEntry represents a recommended settlement between two users.
type BalanceEntry struct {
	From   string  `json:"from"`
	To     string  `json:"to"`
	Amount float64 `json:"amount"`
}

func GetBalances(s *Server) http.HandlerFunc {
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

		// Collect all expense doc_ids for this group
		rows, err := s.AuthDB.Query(
			"SELECT DISTINCT doc_id FROM crdt_operations WHERE op_type = 'lww' AND field = 'group_id' AND value = ?",
			string(mustMarshal(gid)),
		)
		if err != nil {
			respondError(w, 500, "db error")
			return
		}

		// net balance per user: positive = should receive, negative = should pay
		balances := make(map[string]float64)

		// --- Process expenses ---
		var expenseIDs []string
		for rows.Next() {
			var docID string
			if rows.Scan(&docID) == nil {
				expenseIDs = append(expenseIDs, docID)
			}
		}
		rows.Close()

		for _, docID := range expenseIDs {
			state, err := s.Store.GetLatestState(docID)
			if err != nil || len(state) == 0 {
				continue
			}
			if t, ok := state["tombstone"]; ok && t == true {
				continue
			}

			paidBy, _ := state["paid_by"].(string)
			amount, _ := state["amount"].(float64)
			if paidBy == "" || amount <= 0 {
				continue
			}

			// Payer is owed the total amount
			balances[paidBy] += amount

			// Subtract each participant's share
			splits, _ := state["splits"].([]any)
			for _, sp := range splits {
				split, ok := sp.(map[string]any)
				if !ok {
					continue
				}
				uid, _ := split["user_id"].(string)
				splitAmt, _ := split["amount"].(float64)
				if uid == "" {
					continue
				}
				balances[uid] -= splitAmt
			}
		}

		// --- Process confirmed payments ---
		payRows, err := s.AuthDB.Query(
			"SELECT DISTINCT doc_id FROM crdt_operations WHERE op_type = 'lww' AND field = 'group_id' AND value = ?",
			string(mustMarshal(gid)),
		)
		if err == nil {
			var paymentIDs []string
			for payRows.Next() {
				var docID string
				if payRows.Scan(&docID) == nil {
					paymentIDs = append(paymentIDs, docID)
				}
			}
			payRows.Close()

			for _, docID := range paymentIDs {
				state, err := s.Store.GetLatestState(docID)
				if err != nil || len(state) == 0 {
					continue
				}
				if t, ok := state["tombstone"]; ok && t == true {
					continue
				}
				status, _ := state["status"].(string)
				if status != "confirmed" {
					continue
				}

				fromUser, _ := state["from_user"].(string)
				toUser, _ := state["to_user"].(string)
				amt, _ := state["amount"].(float64)
				if fromUser == "" || toUser == "" {
					continue
				}

				balances[fromUser] += amt
				balances[toUser] -= amt
			}
		}

		// --- Build settlement recommendations ---
		var debtors, creditors []string
		for uid, bal := range balances {
			if bal < -0.01 {
				debtors = append(debtors, uid)
			} else if bal > 0.01 {
				creditors = append(creditors, uid)
			}
		}
		sort.Strings(debtors)
		sort.Strings(creditors)

		var settlements []BalanceEntry
		for _, debtor := range debtors {
			for _, creditor := range creditors {
				amt := math.Min(-balances[debtor], balances[creditor])
				if amt < 0.01 {
					continue
				}
				rounded := math.Round(amt*100) / 100
				settlements = append(settlements, BalanceEntry{
					From:   debtor,
					To:     creditor,
					Amount: rounded,
				})
				balances[debtor] += amt
				balances[creditor] -= amt
				if balances[debtor] > -0.01 {
					break
				}
			}
		}

		if settlements == nil {
			settlements = []BalanceEntry{}
		}
		respondOK(w, settlements)
	}
}
