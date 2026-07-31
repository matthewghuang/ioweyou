package api

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"


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
// If staticDir is non-empty and exists, frontend files are served with SPA fallback.
func NewRouter(s *Server, staticDir string) http.Handler {
	r := chi.NewRouter()

	// Middleware
	r.Use(chiMiddleware.Logger)
	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.RealIP)

	// CORS
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Group-Token, Authorization")
			if r.Method == "OPTIONS" {
				w.WriteHeader(204)
				return
			}
			next.ServeHTTP(w, r)
		})
	})

	// CSP
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Security-Policy",
				"default-src 'self'; "+
					"script-src 'self'; "+
					"style-src 'self' 'unsafe-inline'; "+
					"connect-src 'self' ws: wss:; "+
					"img-src 'self' data:; "+
					"font-src 'self'; "+
					"frame-ancestors 'none'; "+
					"base-uri 'self'")
			next.ServeHTTP(w, r)
		})
	})

	// Public routes (no auth required)
	r.Post("/api/groups", CreateGroup(s))
	r.Post("/api/groups/join", JoinGroup(s))
	r.Get("/api/groups/{slug}/info", GroupInfo(s))

	// WebSocket (auth via query param, handled inside)
	r.Get("/api/ws", sync.HandleWS(s.Store, s.AuthDB, s.HLC, s.Broadcaster))

	// Authenticated routes (require X-Group-Token)
	r.Group(func(r chi.Router) {
		r.Use(auth.AuthMiddleware(s.AuthDB))

		r.Get("/api/groups/{slug}", GetGroup(s))
		r.Patch("/api/groups/{slug}", UpdateGroup(s))
		r.Delete("/api/groups/{slug}/members/me", LeaveGroup(s))

		// Expenses
		r.Post("/api/groups/{slug}/expenses", CreateExpense(s))
		r.Get("/api/groups/{slug}/expenses", ListExpenses(s))
		r.Get("/api/expenses/{id}", GetExpense(s))
		r.Patch("/api/expenses/{id}", UpdateExpense(s))
		r.Delete("/api/expenses/{id}", DeleteExpense(s))

		// Payments
		r.Post("/api/groups/{slug}/payments", CreatePayment(s))
		r.Get("/api/groups/{slug}/payments", ListPayments(s))
		r.Get("/api/payments/{id}", GetPayment(s))
		r.Patch("/api/payments/{id}", UpdatePayment(s))
		r.Post("/api/payments/{id}/confirm", ConfirmPayment(s))
		r.Delete("/api/payments/{id}", CancelPayment(s))

		// Balances
		r.Get("/api/groups/{slug}/balances", GetBalances(s))

		// Sync
		r.Post("/api/sync/pull", sync.HandlePull(s.AuthDB))
		r.Post("/api/sync/push", sync.HandlePush(s.AuthDB, s.HLC, s.Broadcaster))
	})

	// Frontend SPA serving
	if staticDir != "" {
		if info, err := os.Stat(staticDir); err == nil && info.IsDir() {
			absDir, _ := filepath.Abs(staticDir)
			log.Printf("serving frontend from %s", absDir)

			// Serve /assets/ files directly
			r.Get("/assets/*", func(w http.ResponseWriter, r *http.Request) {
				http.StripPrefix("/assets/", http.FileServer(http.Dir(filepath.Join(absDir, "assets")))).ServeHTTP(w, r)
			})

			// All other requests (non-API, non-WS) — try real file first, then SPA fallback
			r.NotFound(func(w http.ResponseWriter, r *http.Request) {
				// Clean the request path and resolve to an absolute path
				cleanedPath := filepath.Clean(r.URL.Path)
				fullPath := filepath.Join(absDir, cleanedPath)
				// Verify the resolved path is within absDir (prevents path traversal)
				if !strings.HasPrefix(fullPath, absDir) {
					http.NotFound(w, r)
					return
				}
				if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
					http.ServeFile(w, r, fullPath)
					return
				}
				// SPA fallback for client-side routes
				http.ServeFile(w, r, filepath.Join(absDir, "index.html"))
			})
		} else {
			log.Printf("frontend directory %s not found, API-only mode", staticDir)
		}
	}

	return r
}


