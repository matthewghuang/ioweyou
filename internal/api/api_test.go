package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/matthewghuang/ioweyou/internal/auth"
	"github.com/matthewghuang/ioweyou/internal/crdt"
	"github.com/matthewghuang/ioweyou/internal/store"
	"github.com/matthewghuang/ioweyou/internal/sync"
)

// testServer creates a Server with a temporary SQLite database and returns
// the server, the temp db path, and a cleanup function.
func testServer(t *testing.T) (*Server, string, func()) {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "ioweyou-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpFile.Close()
	dbPath := tmpFile.Name()

	st, err := store.NewSQLiteStateStore(dbPath)
	if err != nil {
		os.Remove(dbPath)
		t.Fatalf("failed to open store: %v", err)
	}

	hlc := crdt.NewHLC()
	bcast := sync.NewBroadcaster()

	srv := &Server{
		Store:       st,
		HLC:         hlc,
		AuthDB:      st.DB(),
		Broadcaster: bcast,
	}

	cleanup := func() {
		st.Close()
		os.Remove(dbPath)
	}

	return srv, dbPath, cleanup
}

// request is a helper to make JSON HTTP requests and parse responses.
// chiParams are optional route parameters (e.g. {"slug": "my-group"}).
func request(t *testing.T, handler http.Handler, method, path string, body any, token string, chiParams ...map[string]string) *http.Response {
	t.Helper()

	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("failed to marshal body: %v", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req := httptest.NewRequest(method, path, bodyReader)

	// Inject chi route params if provided
	if len(chiParams) > 0 && chiParams[0] != nil {
		rctx := chi.NewRouteContext()
		for k, v := range chiParams[0] {
			rctx.URLParams.Add(k, v)
		}
		ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
		req = req.WithContext(ctx)
	}

	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Group-Token", token)
	}

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w.Result()
}

// parseBody reads the response body and unmarshals it into dest.
func parseBody(t *testing.T, resp *http.Response, dest any) {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	resp.Body.Close()
	if err := json.Unmarshal(body, dest); err != nil {
		t.Fatalf("failed to parse response body (%s): %v", string(body), err)
	}
}

// authedRequest wraps handler with auth middleware then calls request.
func authedRequest(t *testing.T, srv *Server, handler http.HandlerFunc, method, path string, body any, token string, chiParams ...map[string]string) *http.Response {
	t.Helper()
	// Wrap with auth middleware; it reads X-Group-Token from the header
	// (already set by request) and injects MemberInfo into context.
	wrapped := auth.AuthMiddleware(srv.AuthDB)(handler)
	return request(t, wrapped, method, path, body, token, chiParams...)
}

func TestCreateAndJoinGroup(t *testing.T) {
	srv, _, cleanup := testServer(t)
	defer cleanup()

	// Create a group
	resp := request(t, CreateGroup(srv), "POST", "/api/groups", map[string]string{
		"name":         "Test Group",
		"creator_name": "Alice",
		"secret":       "secret123",
	}, "")

	if resp.StatusCode != 201 {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var createResult map[string]any
	parseBody(t, resp, &createResult)

	slug, ok := createResult["id"].(string)
	if !ok || slug == "" {
		t.Fatal("expected non-empty group id/slug")
	}
	aliceToken, ok := createResult["cookie_token"].(string)
	if !ok || aliceToken == "" {
		t.Fatal("expected cookie_token")
	}
	groupID, ok := createResult["internal_id"].(string)
	if !ok || groupID == "" {
		t.Fatal("expected internal_id")
	}

	// Verify group exists via info endpoint (public)
	resp = request(t, GroupInfo(srv), "GET", "/api/groups/"+slug+"/info", nil, "", map[string]string{"slug": slug})
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var infoResult map[string]any
	parseBody(t, resp, &infoResult)
	if infoResult["name"] != "Test Group" {
		t.Fatalf("expected 'Test Group', got %v", infoResult["name"])
	}

	// Join as Bob
	resp = request(t, JoinGroup(srv), "POST", "/api/groups/join", map[string]string{
		"slug":   slug,
		"name":   "Bob",
		"secret": "secret456",
	}, "")
	if resp.StatusCode != 201 {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var joinResult map[string]any
	parseBody(t, resp, &joinResult)
	bobToken, ok := joinResult["cookie_token"].(string)
	if !ok || bobToken == "" {
		t.Fatal("expected cookie_token for Bob")
	}

	// Verify group detail shows both members (authenticated as Alice)
	resp = authedRequest(t, srv, GetGroup(srv), "GET", "/api/groups/"+slug, nil, aliceToken, map[string]string{"slug": slug})
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var groupResult map[string]any
	parseBody(t, resp, &groupResult)
	members, ok := groupResult["members"].([]any)
	if !ok || len(members) != 2 {
		t.Fatalf("expected 2 members, got %v", members)
	}
}

func TestLeaveGroup(t *testing.T) {
	srv, _, cleanup := testServer(t)
	defer cleanup()

	// Create group
	resp := request(t, CreateGroup(srv), "POST", "/api/groups", map[string]string{
		"name":         "Test Group",
		"creator_name": "Alice",
		"secret":       "secret123",
	}, "")
	var createResult map[string]any
	parseBody(t, resp, &createResult)
	slug := createResult["id"].(string)
	aliceToken := createResult["cookie_token"].(string)

	// Join as Bob
	resp = request(t, JoinGroup(srv), "POST", "/api/groups/join", map[string]string{
		"slug":   slug,
		"name":   "Bob",
		"secret": "secret456",
	}, "")
	var joinResult map[string]any
	parseBody(t, resp, &joinResult)
	bobToken := joinResult["cookie_token"].(string)

	// Bob leaves the group
	resp = authedRequest(t, srv, LeaveGroup(srv), "DELETE", "/api/groups/"+slug+"/members/me", nil, bobToken, map[string]string{"slug": slug})
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Verify only Alice remains
	resp = authedRequest(t, srv, GetGroup(srv), "GET", "/api/groups/"+slug, nil, aliceToken, map[string]string{"slug": slug})
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var groupResult map[string]any
	parseBody(t, resp, &groupResult)
	members, ok := groupResult["members"].([]any)
	if !ok || len(members) != 1 {
		t.Fatalf("expected 1 member after Bob leaves, got %v", members)
	}

	// Verify Bob can't access the group anymore (member row deleted, token invalid)
	resp = authedRequest(t, srv, GetGroup(srv), "GET", "/api/groups/"+slug, nil, bobToken, map[string]string{"slug": slug})
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401 for Bob after leaving (token invalidated), got %d", resp.StatusCode)
	}
}
