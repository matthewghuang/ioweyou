package api

import (
	"net/http"

	"github.com/matthewghuang/ioweyou/internal/auth"
)

// ListUsers handles GET /api/users?q=name
// Searches users by name prefix (case-insensitive).
// Returns at most 20 matching users.
func ListUsers(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())
		if user == nil {
			respondError(w, 401, "unauthorized")
			return
		}

		q := r.URL.Query().Get("q")
		if q == "" {
			respondJSON(w, 200, []map[string]string{})
			return
		}

		rows, err := s.AuthDB.Query(
			"SELECT id, name FROM users WHERE name LIKE ? ORDER BY name LIMIT 20",
			"%"+q+"%",
		)
		if err != nil {
			respondError(w, 500, "db error")
			return
		}
		defer rows.Close()

		var users []map[string]string
		for rows.Next() {
			var id, name string
			if rows.Scan(&id, &name) == nil {
				users = append(users, map[string]string{"id": id, "name": name})
			}
		}
		if users == nil {
			users = []map[string]string{}
		}
		respondOK(w, users)
	}
}
