package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/matthewghuang/ioweyou/internal/auth"
	"github.com/matthewghuang/ioweyou/internal/crdt"
)

func getGroupMembers(db *sql.DB, groupID string) []map[string]string {
	rows, err := db.Query(`SELECT member_id, user_name FROM members WHERE group_id = ?`, groupID)
	if err != nil {
		return []map[string]string{}
	}
	defer rows.Close()
	var members []map[string]string
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}
		members = append(members, map[string]string{"id": id, "name": name})
	}
	if err := rows.Err(); err != nil {
		return []map[string]string{}
	}
	if members == nil {
		members = []map[string]string{}
	}
	return members
}

func isGroupMember(db *sql.DB, groupID, memberID string) bool {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM members WHERE group_id = ? AND member_id = ?", groupID, memberID).Scan(&count); err != nil {
		return false
	}
	return count > 0
}

// errNotFound is a sentinel for "item not found" in resolveGroup.
var errNotFound = errors.New("not found")

// resolveGroup checks if identifier is a UUID (direct group_id) or a slug
// (lookup in group_slugs). Returns the UUID.
// Returns errNotFound when the identifier is valid but no record exists.
func resolveGroup(db *sql.DB, identifier string) (string, error) {
	// Try as UUID first (direct group_id)
	if uuid.Validate(identifier) == nil {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM group_slugs WHERE group_id = ?", identifier).Scan(&count); err != nil {
			return "", err
		}
		if count > 0 {
			return identifier, nil
		}
		return "", errNotFound
	}

	// Lookup by slug
	var groupID string
	err := db.QueryRow("SELECT group_id FROM group_slugs WHERE slug = ?", identifier).Scan(&groupID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errNotFound
	}
	if err != nil {
		return "", err
	}
	return groupID, nil
}

func getGroupInfo(db *sql.DB, store crdt.StateStore, slug string) (map[string]any, error) {
	groupID, err := resolveGroup(db, slug)
	if err != nil {
		return nil, err
	}
	state, err := store.GetLatestState(groupID)
	if err != nil {
		return nil, err
	}
	if len(state) == 0 {
		return nil, nil
	}
	members := getGroupMembers(db, groupID)
	return map[string]any{
		"name":         state["name"],
		"slug":         slug,
		"internal_id":  groupID,
		"member_count": len(members),
		"members":      members,
	}, nil
}

// CreateGroup handles POST /api/groups (public, no auth required).
func CreateGroup(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name        string `json:"name"`
			CreatorName string `json:"creator_name"`
			Secret      string `json:"secret"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" || body.CreatorName == "" || body.Secret == "" {
			respondError(w, 400, "name, creator_name, and secret are required")
			return
		}

		slug, memberID, cookieToken, groupID, err := auth.CreateGroup(s.AuthDB, s.Store, s.HLC, body.Name, body.CreatorName, body.Secret)
		if err != nil {
			respondError(w, 500, err.Error())
			return
		}

		respondJSON(w, 201, map[string]any{
			"id":           slug,
			"internal_id":  groupID,
			"member_id":    memberID,
			"cookie_token": cookieToken,
		})
	}
}

// JoinGroup handles POST /api/groups/join (public, no auth required).
func JoinGroup(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Slug   string `json:"slug"`
			Name   string `json:"name"`
			Secret string `json:"secret"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Slug == "" || body.Name == "" || body.Secret == "" {
			respondError(w, 400, "slug, name, and secret are required")
			return
		}

		memberID, cookieToken, groupID, err := auth.JoinGroup(s.AuthDB, body.Slug, body.Name, body.Secret)
		if err != nil {
			if errors.Is(err, auth.ErrGroupNotFound) {
				respondError(w, 404, "group not found")
				return
			}
			if errors.Is(err, auth.ErrInvalidSecret) {
				respondError(w, 401, "invalid secret")
				return
			}
			respondError(w, 500, err.Error())
			return
		}

		state, err := s.Store.GetLatestState(groupID)
		groupName := ""
		if err == nil {
			groupName, _ = state["name"].(string)
		}

		respondJSON(w, 201, map[string]any{
			"cookie_token": cookieToken,
			"member_id":    memberID,
			"group_name":   groupName,
			"internal_id":  groupID,
		})
	}
}

// GroupInfo handles GET /api/groups/{slug}/info (public, no auth required).
func GroupInfo(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		info, err := getGroupInfo(s.AuthDB, s.Store, slug)
		if errors.Is(err, errNotFound) {
			respondError(w, 404, "group not found")
			return
		}
		if err != nil {
			respondError(w, 500, "db error")
			return
		}
		if info == nil {
			respondError(w, 404, "group not found")
			return
		}
		respondOK(w, info)
	}
}

// GetGroup handles GET /api/groups/{slug} (authenticated).
func GetGroup(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		member := auth.MemberFromContext(r.Context())
		if member == nil {
			respondError(w, 401, "unauthorized")
			return
		}

		slug := chi.URLParam(r, "slug")
		groupID, err := resolveGroup(s.AuthDB, slug)
		if errors.Is(err, errNotFound) {
			respondError(w, 404, "group not found")
			return
		}
		if err != nil {
			respondError(w, 500, "db error")
			return
		}

		if !isGroupMember(s.AuthDB, groupID, member.MemberID) {
			respondError(w, 403, "not a member")
			return
		}

		state, err := s.Store.GetLatestState(groupID)
		if err != nil {
			respondError(w, 500, "db error")
			return
		}
		if len(state) == 0 {
			respondError(w, 404, "not found")
			return
		}
		state["id"] = slug
		state["internal_id"] = groupID
		state["members"] = getGroupMembers(s.AuthDB, groupID)
		respondOK(w, state)
	}
}

// LeaveGroup handles DELETE /api/groups/{slug}/members/me
// Removes the authenticated member from the group's members table.
func LeaveGroup(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		member := auth.MemberFromContext(r.Context())
		if member == nil {
			respondError(w, 401, "unauthorized")
			return
		}

		slug := chi.URLParam(r, "slug")
		gid, err := resolveGroup(s.AuthDB, slug)
		if err != nil {
			respondError(w, 404, "group not found")
			return
		}

		// Verify the member belongs to this group
		if member.GroupID != gid {
			respondError(w, 403, "not a member of this group")
			return
		}

		// Delete the member
		result, err := s.AuthDB.Exec(
			`DELETE FROM members WHERE member_id = ? AND group_id = ?`,
			member.MemberID, gid,
		)
		if err != nil {
			respondError(w, 500, "failed to remove member")
			return
		}

		rows, _ := result.RowsAffected()
		if rows == 0 {
			respondError(w, 404, "member not found")
			return
		}

		respondOK(w, map[string]string{"status": "ok"})
	}
}

// UpdateGroup handles PATCH /api/groups/{slug} (authenticated).
func UpdateGroup(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		member := auth.MemberFromContext(r.Context())
		if member == nil {
			respondError(w, 401, "unauthorized")
			return
		}

		slug := chi.URLParam(r, "slug")
		groupID, err := resolveGroup(s.AuthDB, slug)
		if errors.Is(err, errNotFound) {
			respondError(w, 404, "group not found")
			return
		}
		if err != nil {
			respondError(w, 500, "db error")
			return
		}

		if !isGroupMember(s.AuthDB, groupID, member.MemberID) {
			respondError(w, 403, "not a member")
			return
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondError(w, 400, "invalid body")
			return
		}
		// Only allow updating the group name
		allowedFields := map[string]bool{"name": true}
		for k, v := range body {
			if !allowedFields[k] {
				continue
			}
			if err := s.Store.Append(crdt.Operation{
				DocID: groupID, OpType: crdt.OpLWW, Field: k,
				Value: mustMarshal(v), AuthorID: member.MemberID, Timestamp: s.HLC.Now(),
			}); err != nil {
				respondError(w, 500, "update failed")
				return
			}
		}

		state, err := s.Store.GetLatestState(groupID)
		if err != nil {
			respondError(w, 500, "failed to read state")
			return
		}
		state["id"] = slug
		state["internal_id"] = groupID
		state["members"] = getGroupMembers(s.AuthDB, groupID)
		respondOK(w, state)

		// Broadcast to group subscribers
		if s.Broadcaster != nil {
			if ops, err := s.Store.GetOps(groupID, nil); err == nil && len(ops) > 0 {
				s.Broadcaster.Broadcast(groupID, ops)
			}
		}
	}
}
