package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/matthewghuang/ioweyou/internal/crdt"
)

// MemberInfo represents an authenticated group member (injected into context).
type MemberInfo struct {
	MemberID string `json:"member_id"`
	GroupID  string `json:"group_id"`
	UserName string `json:"user_name"`
}

// Sentinel errors for JoinGroup / CreateGroup flows.
var (
	ErrGroupNotFound = fmt.Errorf("group not found")
	ErrInvalidSecret = fmt.Errorf("invalid secret")
)

// Context key for authenticated member.
type contextKey string

const MemberContextKey contextKey = "member"

// HashSecret returns SHA-256 hex of the secret.
func HashSecret(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(h[:])
}

// GenerateToken creates a 32-byte random hex string (replaces GenerateAPIKey).
func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// GenerateSlug creates a human-readable slug from a group name.
// Lowercases, replaces spaces with hyphens, strips non-alnum, appends 8 random hex chars.
func GenerateSlug(name string) (string, error) {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else if r == ' ' || r == '-' {
			b.WriteRune('-')
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "group"
	}
	randSuffix := make([]byte, 4) // 8 hex chars
	if _, err := rand.Read(randSuffix); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	return fmt.Sprintf("%s-%x", slug, randSuffix), nil
}

// CreateGroup creates group slug, group CRDT doc, and creator member entry.
// CRDT appends happen first (append-only, harmless if orphaned), then
// relational DB inserts are wrapped in a transaction for atomicity.
// Returns (slug, memberID, cookieToken, groupID, error).
func CreateGroup(db *sql.DB, store crdt.StateStore, hlc *crdt.HLC, groupName, creatorName, secret string) (slug, memberID, cookieToken, groupID string, err error) {
	groupID = uuid.New().String()
	slug, err = GenerateSlug(groupName)
	if err != nil {
		return "", "", "", "", fmt.Errorf("generate slug: %w", err)
	}

	// Generate member UUID first so it can be used as AuthorID in CRDT ops
	memberID = uuid.New().String()

	// Create group CRDT doc with member as author (before the DB transaction;
	// these are append-only and harmless if orphaned by a later rollback).
	ts := hlc.Now()
	if err := store.Append(crdt.Operation{
		DocID: groupID, OpType: crdt.OpLWW, Field: "name",
		Value: mustMarshal(groupName), AuthorID: memberID, Timestamp: ts,
	}); err != nil {
		return "", "", "", "", fmt.Errorf("store append: %w", err)
	}
	ts = hlc.Now()
	if err := store.Append(crdt.Operation{
		DocID: groupID, OpType: crdt.OpLWW, Field: "created_by",
		Value: mustMarshal(creatorName), AuthorID: memberID, Timestamp: ts,
	}); err != nil {
		return "", "", "", "", fmt.Errorf("store append: %w", err)
	}
	ts = hlc.Now()
	if err := store.Append(crdt.Operation{
		DocID: groupID, OpType: crdt.OpLWW, Field: "created_at",
		Value: mustMarshal(ts.WallTime / 1e6), AuthorID: memberID, Timestamp: ts,
	}); err != nil {
		return "", "", "", "", fmt.Errorf("store append: %w", err)
	}

	// Prepare relational data
	secretHash := HashSecret(secret)
	cookieToken, err = GenerateToken()
	if err != nil {
		return "", "", "", "", fmt.Errorf("generate token: %w", err)
	}

	// Wrap slug + member inserts in a SQL transaction for atomicity.
	tx, err := db.Begin()
	if err != nil {
		return "", "", "", "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	if _, err = tx.Exec("INSERT INTO group_slugs (slug, group_id) VALUES (?, ?)", slug, groupID); err != nil {
		return "", "", "", "", fmt.Errorf("insert slug: %w", err)
	}

	if _, err = tx.Exec(
		"INSERT INTO members (member_id, group_id, user_name, secret_hash, cookie_token) VALUES (?, ?, ?, ?, ?)",
		memberID, groupID, creatorName, secretHash, cookieToken,
	); err != nil {
		return "", "", "", "", fmt.Errorf("insert member: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return "", "", "", "", fmt.Errorf("commit tx: %w", err)
	}

	return slug, memberID, cookieToken, groupID, nil
}

// JoinGroup adds a new member to an existing group, or re-authenticates an existing member.
// If the user_name already exists in the group, the secret must match the stored hash
// (re-issues a fresh cookie_token). Otherwise, a new member is created.
// Returns (memberID, cookieToken, groupID, error).
func JoinGroup(db *sql.DB, slug, userName, secret string) (memberID, cookieToken, groupID string, err error) {
	// Look up group_id from slug
	err = db.QueryRow("SELECT group_id FROM group_slugs WHERE slug = ?", slug).Scan(&groupID)
	if err == sql.ErrNoRows {
		return "", "", "", ErrGroupNotFound
	}
	if err != nil {
		return "", "", "", fmt.Errorf("lookup slug: %w", err)
	}

	// Check if this user already exists in the group
	var existingID, existingHash string
	err = db.QueryRow("SELECT member_id, secret_hash FROM members WHERE group_id = ? AND user_name = ?", groupID, userName).Scan(&existingID, &existingHash)
	if err == nil {
		// Member exists — verify secret and re-issue token
		if HashSecret(secret) != existingHash {
			return "", "", "", ErrInvalidSecret
		}
		cookieToken, err = GenerateToken()
		if err != nil {
			return "", "", "", fmt.Errorf("generate token: %w", err)
		}
		if _, err = db.Exec("UPDATE members SET cookie_token = ? WHERE member_id = ?", cookieToken, existingID); err != nil {
			return "", "", "", fmt.Errorf("update token: %w", err)
		}
		return existingID, cookieToken, groupID, nil
	}
	if err != sql.ErrNoRows {
		return "", "", "", fmt.Errorf("lookup member: %w", err)
	}

	// New member — create
	memberID = uuid.New().String()
	secretHash := HashSecret(secret)
	cookieToken, err = GenerateToken()
	if err != nil {
		return "", "", "", fmt.Errorf("generate token: %w", err)
	}

	// Wrap member insert in a transaction.
	tx, err := db.Begin()
	if err != nil {
		return "", "", "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	if _, err = tx.Exec(
		"INSERT INTO members (member_id, group_id, user_name, secret_hash, cookie_token) VALUES (?, ?, ?, ?, ?)",
		memberID, groupID, userName, secretHash, cookieToken,
	); err != nil {
		return "", "", "", fmt.Errorf("insert member: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return "", "", "", fmt.Errorf("commit tx: %w", err)
	}

	return memberID, cookieToken, groupID, nil
}

// LookupMemberByToken looks up a member by cookie_token.
// Returns MemberInfo or nil.
func LookupMemberByToken(db *sql.DB, token string) *MemberInfo {
	row := db.QueryRow("SELECT member_id, group_id, user_name FROM members WHERE cookie_token = ?", token)
	var m MemberInfo
	err := row.Scan(&m.MemberID, &m.GroupID, &m.UserName)
	if err != nil {
		return nil
	}
	return &m
}

// AuthMiddleware extracts X-Group-Token header, looks up member, injects MemberInfo into context.
// On failure returns 401.
func AuthMiddleware(db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get("X-Group-Token")
			if token == "" {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}

			member := LookupMemberByToken(db, token)
			if member == nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}

			ctx := context.WithValue(r.Context(), MemberContextKey, member)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// MemberFromContext extracts MemberInfo from request context.
func MemberFromContext(ctx context.Context) *MemberInfo {
	member, _ := ctx.Value(MemberContextKey).(*MemberInfo)
	return member
}

func mustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic("mustMarshal: " + err.Error())
	}
	return b
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// client connection gone — nothing to do
	}
}
