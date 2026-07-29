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
// user, store, and broadcast state.
type WSClient struct {
	conn   *websocket.Conn
	send   chan []byte
	user   *auth.User
	store  crdt.StateStore
	authDB *sql.DB
	hlc    *crdt.HLC
	bcast  *Broadcaster
}

// HandleWS is the HTTP handler for WS upgrade at /api/ws?token=<api_key>.
// It authenticates via query parameter, upgrades the connection, and starts
// the read and write pump goroutines.
func HandleWS(store crdt.StateStore, authDB *sql.DB, hlc *crdt.HLC, bcast *Broadcaster) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "missing token"})
			return
		}

		user, err := auth.LookupUserByAPIKey(authDB, token)
		if err != nil || user == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
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
			user:   user,
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
		c.bcast.Unsubscribe(c.conn)
		c.conn.Close()
	}()

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("ws read error: %v", err)
			}
			break
		}

		var msg struct {
			Type       string            `json:"type"`
			GroupID    string            `json:"group_id,omitempty"`
			Operations []crdt.Operation  `json:"operations,omitempty"`
		}
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "subscribe":
			c.bcast.Subscribe(msg.GroupID, c.conn)

		case "push":
			// Advance HLC for causality tracking
			for _, op := range msg.Operations {
				c.hlc.Observe(op.Timestamp)
			}

			// Persist operations
			for _, op := range msg.Operations {
				if err := c.store.Append(op); err != nil {
					log.Printf("append op error: %v", err)
				}
			}

			// Broadcast to subscribers of relevant groups
			groups := resolveDocGroups(msg.Operations, c.store)
			for _, gid := range groups {
				c.bcast.Broadcast(gid, msg.Operations)
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
		}
	}
}

// resolveDocGroups determines which group_ids should receive notifications
// for the given operations by inspecting each document's projected state.
func resolveDocGroups(ops []crdt.Operation, store crdt.StateStore) []string {
	seen := make(map[string]bool)
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
			groups = append(groups, op.DocID)
			continue
		}

		// If the doc has a "group_id" field, broadcast to that group.
		if gid, ok := state["group_id"].(string); ok && gid != "" {
			groups = append(groups, gid)
		}
	}

	return groups
}
