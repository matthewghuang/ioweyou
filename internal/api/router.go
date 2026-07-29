package api

import (
	"database/sql"
	"net/http"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/matthewghuang/ioweyou/internal/auth"
	"github.com/matthewghuang/ioweyou/internal/crdt"
	"github.com/matthewghuang/ioweyou/internal/sync"
)

// Server holds the shared dependencies for all API handlers.
type Server struct {
	Store       crdt.StateStore
	HLC         *crdt.HLC
	AuthDB      *sql.DB
	Broadcaster *sync.Broadcaster
}

// NewRouter creates a chi router with all routes mounted.
func NewRouter(s *Server) http.Handler {
	r := chi.NewRouter()

	// Middleware
	r.Use(chiMiddleware.Logger)
	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.RealIP)

	// CORS
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			if r.Method == "OPTIONS" {
				w.WriteHeader(204)
				return
			}
			next.ServeHTTP(w, r)
		})
	})

	// Public routes
	r.Post("/api/auth/register", auth.RegisterHandler(s.AuthDB))

	// Authenticated routes
	r.Group(func(r chi.Router) {
		r.Use(auth.AuthMiddleware(s.AuthDB))

		// Whoami
		r.Get("/api/auth/whoami", auth.WhoamiHandler())

		// Users
		r.Get("/api/users", ListUsers(s))

		// Groups
		r.Post("/api/groups", CreateGroup(s))
		r.Get("/api/groups", ListGroups(s))
		r.Get("/api/groups/{id}", GetGroup(s))
		r.Patch("/api/groups/{id}", UpdateGroup(s))
		r.Post("/api/groups/{id}/members", AddMember(s))

		// Expenses
		r.Post("/api/groups/{id}/expenses", CreateExpense(s))
		r.Get("/api/groups/{id}/expenses", ListExpenses(s))
		r.Get("/api/expenses/{id}", GetExpense(s))
		r.Patch("/api/expenses/{id}", UpdateExpense(s))
		r.Delete("/api/expenses/{id}", DeleteExpense(s))

		// Payments
		r.Post("/api/groups/{id}/payments", CreatePayment(s))
		r.Get("/api/groups/{id}/payments", ListPayments(s))
		r.Get("/api/payments/{id}", GetPayment(s))
		r.Post("/api/payments/{id}/confirm", ConfirmPayment(s))
		r.Delete("/api/payments/{id}", CancelPayment(s))

		// Balances
		r.Get("/api/groups/{id}/balances", GetBalances(s))

		// Sync
		r.Post("/api/sync/pull", sync.HandlePull(s.AuthDB))
		r.Post("/api/sync/push", sync.HandlePush(s.AuthDB, s.HLC, s.Broadcaster))
		r.Get("/api/ws", sync.HandleWS(s.Store, s.AuthDB, s.HLC, s.Broadcaster))
	})

	return r
}
