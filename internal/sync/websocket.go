package sync

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/matthewghuang/ioweyou/internal/auth"
	"github.com/matthewghuang/ioweyou/internal/crdt"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// WSClient represents a single WebSocket connection with its associated
// member, store, and broadcast state.
type WSClient struct {
	conn   *websocket.Conn
	send   chan []byte
	done   chan struct{}
	member *auth.MemberInfo
	store  crdt.StateStore
	authDB *sql.DB
	hlc    *crdt.HLC
	bcast  *Broadcaster
}

// HandleWS is the HTTP handler for WS upgrade at /api/ws?token=<cookie_token>.
// It authenticates via query parameter, upgrades the connection, and starts
// the read and write pump goroutines.
func HandleWS(store crdt.StateStore, authDB *sql.DB, hlc *crdt.HLC, bcast *Broadcaster) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing token"})
			return
		}

		member := auth.LookupMemberByToken(authDB, token)
		if member == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("ws upgrade error: %v", err)
			return
		}

		c := &WSClient{
			conn:   conn,
			send:   make(chan []byte, 256),
			done:   make(chan struct{}),
			member: member,
			store:  store,
			authDB: authDB,
			hlc:    hlc,
			bcast:  bcast,
		}

		go c.writePump()
		go c.readPump()
	}
}

// readPump reads messages from the WebSocket connection.
// Expected client messages:
//
//	{"type":"subscribe","group_id":"..."}
//	{"type":"push","operations":[...]}
func (c *WSClient) readPump() {
	defer func() {
		if c.bcast != nil {
			c.bcast.Unsubscribe(c.send)
		}
		close(c.done)
		c.conn.Close()
	}()

	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	outer:
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("ws read error: %v", err)
			}
			break
		}

		var msg struct {
			Type       string           `json:"type"`
			GroupID    string           `json:"group_id,omitempty"`
			Operations []crdt.Operation `json:"operations,omitempty"`
		}
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "subscribe":
			// Verify membership before subscribing to a group
			var count int
			if err := c.authDB.QueryRow("SELECT COUNT(*) FROM members WHERE group_id = ? AND member_id = ?", msg.GroupID, c.member.MemberID).Scan(&count); err != nil || count == 0 {
				log.Printf("ws subscribe auth: member %s not authorized for group %s", c.member.MemberID, msg.GroupID)
				continue
			}
			c.bcast.Subscribe(msg.GroupID, c.send)

		case "push":
			// Advance HLC for causality tracking
			for _, op := range msg.Operations {
				c.hlc.Observe(op.Timestamp)
			}

			// Ops arrive with value as a JSON string; recover the raw value
			// before group resolution and insertion.
			normalizePushedOps(msg.Operations)

			// Verify group membership before inserting.
			groupSet := make(map[string]bool)
			for _, op := range msg.Operations {
				if op.OpType == crdt.OpLWW && op.Field == "name" {
					groupSet[op.DocID] = true
				}
				if op.OpType == crdt.OpLWW && op.Field == "group_id" {
					var gid string
					if err := json.Unmarshal(op.Value, &gid); err == nil && gid != "" {
						groupSet[gid] = true
					}
				}
			}
			for _, op := range msg.Operations {
				if !groupSet[op.DocID] {
					if gid := resolveDocGroup(c.authDB, op.DocID); gid != "" {
						groupSet[gid] = true
					}
				}
			}
			if len(groupSet) == 0 {
				log.Printf("ws push auth: could not determine group for operations")
				continue
			}

			for gid := range groupSet {
				var count int
				if err := c.authDB.QueryRow("SELECT COUNT(*) FROM members WHERE group_id = ? AND member_id = ?", gid, c.member.MemberID).Scan(&count); err != nil || count == 0 {
					log.Printf("ws push auth: member %s not authorized for group %s", c.member.MemberID, gid)
					continue outer
				}
			}

			// Persist operations
			for _, op := range msg.Operations {
				if err := c.store.Append(op); err != nil {
					log.Printf("append op error: %v", err)
					continue
				}
			}

			// Broadcast to subscribers of relevant groups
			if c.bcast != nil {
				groups := resolveDocGroups(msg.Operations, c.store)
				for _, gid := range groups {
					c.bcast.Broadcast(gid, msg.Operations)
				}
			}
		}
	}
}

// writePump writes messages from the send channel to the WebSocket connection.
// It also sends periodic pings to detect connection staleness.
func (c *WSClient) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				// Channel closed, connection shutting down
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}

		case <-c.done:
			return
		}
	}
}

// resolveDocGroups determines which group_ids should receive notifications
// for the given operations by inspecting each document's projected state.
func resolveDocGroups(ops []crdt.Operation, store crdt.StateStore) []string {
	seen := make(map[string]bool)
	seenGroups := make(map[string]bool)
	var groups []string

	for _, op := range ops {
		if seen[op.DocID] {
			continue
		}
		seen[op.DocID] = true

		state, err := store.GetLatestState(op.DocID)
		if err != nil || state == nil {
			continue
		}

		// If the doc itself looks like a group document (has a "name" field),
		// use its doc_id as the broadcast group.
		if _, ok := state["name"]; ok {
			if !seenGroups[op.DocID] {
				seenGroups[op.DocID] = true
				groups = append(groups, op.DocID)
			}
			continue
		}

		// If the doc has a "group_id" field, broadcast to that group.
		if gid, ok := state["group_id"].(string); ok && gid != "" {
			if !seenGroups[gid] {
				seenGroups[gid] = true
				groups = append(groups, gid)
			}
		}
	}

	return groups
}
