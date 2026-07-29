package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// User represents a registered user.
type User struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	APIKey string `json:"api_key,omitempty"`
}

// Context key for authenticated user.
type contextKey string

const UserContextKey contextKey = "user"

// GenerateAPIKey creates a 32-byte random hex string.
func GenerateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// RegisterUser inserts a new user into the users table. Returns the created User.
func RegisterUser(db *sql.DB, name string) (*User, error) {
	id := uuid.New().String()
	apiKey, err := GenerateAPIKey()
	if err != nil {
		return nil, err
	}

	_, err = db.Exec("INSERT INTO users (id, name, api_key) VALUES (?, ?, ?)", id, name, apiKey)
	if err != nil {
		return nil, err
	}

	return &User{ID: id, Name: name, APIKey: apiKey}, nil
}

// LookupUserByAPIKey queries the users table by api_key. Returns nil if not found.
func LookupUserByAPIKey(db *sql.DB, apiKey string) (*User, error) {
	row := db.QueryRow("SELECT id, name, api_key FROM users WHERE api_key = ?", apiKey)

	var u User
	err := row.Scan(&u.ID, &u.Name, &u.APIKey)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &u, nil
}

// AuthMiddleware returns an HTTP middleware that extracts the Bearer token from
// the Authorization header, looks up the user, and injects the User into the request context.
// On failure, returns 401.
func AuthMiddleware(db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
				return
			}

			token := strings.TrimPrefix(auth, "Bearer ")
			user, err := LookupUserByAPIKey(db, token)
			if err != nil || user == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
				return
			}

			ctx := context.WithValue(r.Context(), UserContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserFromContext extracts the User from a request context.
func UserFromContext(ctx context.Context) *User {
	user, _ := ctx.Value(UserContextKey).(*User)
	return user
}

// RegisterHandler handles POST /api/auth/register
// Request body: {"name": "Alice"}
// Response: {"id": "...", "name": "Alice", "api_key": "..."}
// WhoamiHandler handles GET /api/auth/whoami
// Returns the current authenticated user's id and name.
func WhoamiHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		if user == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"id":   user.ID,
			"name": user.Name,
		})
	}
}

// RegisterHandler handles POST /api/auth/register
// Request body: {"name": "Alice"}
// Response: {"id": "...", "name": "Alice", "api_key": "..."}
func RegisterHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
			return
		}

		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
			return
		}

		if body.Name == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "name is required"})
			return
		}

		user, err := RegisterUser(db, body.Name)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to register user"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(user)
	}
}
