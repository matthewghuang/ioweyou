package api

import (
	"errors"
	"math"
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"
	"github.com/matthewghuang/ioweyou/internal/auth"
)

// BreakdownItem shows how one expense contributes to a balance entry.
type BreakdownItem struct {
	ExpenseName string  `json:"expense_name"`
	Amount      float64 `json:"amount"`
}

// BalanceEntry represents a recommended transfer between two users.
type BalanceEntry struct {
	From      string          `json:"from"`
	To        string          `json:"to"`
	Amount    float64         `json:"amount"`
	Breakdown []BreakdownItem `json:"breakdown"`
}

func GetBalances(s *Server) http.HandlerFunc {
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

		// Collect all doc_ids for this group
		rows, err := s.AuthDB.Query(
			"SELECT DISTINCT doc_id FROM crdt_operations WHERE op_type = 'lww' AND field = 'group_id' AND value = ?",
			string(mustMarshal(gid)),
		)
		if err != nil {
			respondError(w, 500, "db error")
			return
		}
		defer rows.Close()

		// net balance per user: positive = should receive, negative = should pay
		balances := make(map[string]float64)

		var docIDs []string
		for rows.Next() {
			var docID string
			if rows.Scan(&docID) == nil {
				docIDs = append(docIDs, docID)
			}
		}
		if err := rows.Err(); err != nil {
			respondError(w, 500, "rows error")
			return
		}

		// Track debt edges between users for per-expense breakdown
		type debtEdge struct {
			fromUser    string
			toUser      string
			amount      float64
			expenseName string
		}
		var allDebts []debtEdge

		for _, docID := range docIDs {
			state, err := s.Store.GetLatestState(docID)
			if err != nil || len(state) == 0 {
				continue
			}
			if t, ok := state["tombstone"]; ok && t == true {
				continue
			}

			// Process as expense if it has paid_by and splits
			paidBy, _ := state["paid_by"].(string)
			description, _ := state["description"].(string)
			amount, _ := state["amount"].(float64)
			if paidBy != "" && amount > 0 {
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

					// Track edge for breakdown (skip self-edges)
					if uid != paidBy && description != "" {
						allDebts = append(allDebts, debtEdge{
							fromUser:    uid,
							toUser:      paidBy,
							amount:      splitAmt,
							expenseName: description,
						})
					}
				}
			}

			// Process as confirmed payment if it has from_user, to_user, and status confirmed
			status, _ := state["status"].(string)
			if status == "confirmed" {
				fromUser, _ := state["from_user"].(string)
				toUser, _ := state["to_user"].(string)
				amt, _ := state["amount"].(float64)
				if fromUser != "" && toUser != "" && amt > 0 {
					// fromUser paid toUser, so fromUser's net balance increases
					// (reduces their debt) and toUser's net balance decreases.
					balances[fromUser] += amt
					balances[toUser] -= amt
				}
			}
		}

		// --- Build balance / settlement recommendations ---
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

		computeBreakdown := func(debtor, creditor string, capAmount float64) []BreakdownItem {
			remaining := capAmount
			var items []BreakdownItem
			for _, d := range allDebts {
				if remaining < 0.01 {
					break
				}
				if d.fromUser == debtor && d.toUser == creditor {
					used := math.Min(d.amount, remaining)
					items = append(items, BreakdownItem{
						ExpenseName: d.expenseName,
						Amount:      math.Round(used*100) / 100,
					})
					remaining -= used
				} else if d.fromUser == creditor && d.toUser == debtor {
					// reverse direction: reduces what debtor owes
					used := math.Min(d.amount, remaining)
					items = append(items, BreakdownItem{
						ExpenseName: d.expenseName,
						Amount:      -math.Round(used*100) / 100,
					})
					remaining -= used
				}
			}
			return items
		}

		var settlements []BalanceEntry
		for _, debtor := range debtors {
			for _, creditor := range creditors {
				amt := math.Min(-balances[debtor], balances[creditor])
				if amt < 0.01 {
					continue
				}
				rounded := math.Round(amt*100) / 100
				breakdown := computeBreakdown(debtor, creditor, rounded)
				if breakdown == nil {
					breakdown = []BreakdownItem{}
				}
				// Absorb any rounding remainder into the last item so breakdown sums exactly to amount
				if len(breakdown) > 0 {
					sum := 0.0
					for _, b := range breakdown {
						sum += b.Amount
					}
					diff := math.Round((rounded-sum)*100) / 100
					if math.Abs(diff) > 0.001 {
						breakdown[len(breakdown)-1].Amount += diff
						breakdown[len(breakdown)-1].Amount = math.Round(breakdown[len(breakdown)-1].Amount*100) / 100
					}
				}
				settlements = append(settlements, BalanceEntry{
					From:      debtor,
					To:        creditor,
					Amount:    rounded,
					Breakdown: breakdown,
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
