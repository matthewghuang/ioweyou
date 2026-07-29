package sync

import (
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/matthewghuang/ioweyou/internal/crdt"
)

// Broadcaster manages WebSocket subscriptions by group_id.
type Broadcaster struct {
	mu   sync.RWMutex
	subs map[string]map[*websocket.Conn]bool // group_id -> set of connections
}

// NewBroadcaster creates a new Broadcaster.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{subs: make(map[string]map[*websocket.Conn]bool)}
}

// Subscribe adds a connection to a group's broadcast list.
func (b *Broadcaster) Subscribe(groupID string, conn *websocket.Conn) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.subs[groupID] == nil {
		b.subs[groupID] = make(map[*websocket.Conn]bool)
	}
	b.subs[groupID][conn] = true
}

// Unsubscribe removes a connection from all groups. If a group becomes empty,
// the group key is deleted from the map.
func (b *Broadcaster) Unsubscribe(conn *websocket.Conn) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for gid, conns := range b.subs {
		delete(conns, conn)
		if len(conns) == 0 {
			delete(b.subs, gid)
		}
	}
}

// Broadcast sends an operation change notification to all subscribers of a group.
// The message is {"type":"change","operations":[...]}.
// Each connection is written to in a separate goroutine to avoid blocking. On
// write error the connection is unsubscribed and closed.
func (b *Broadcaster) Broadcast(groupID string, ops []crdt.Operation) {
	b.mu.RLock()
	conns := b.subs[groupID]
	b.mu.RUnlock()

	if len(conns) == 0 {
		return
	}

	msg, err := json.Marshal(map[string]any{
		"type":       "change",
		"operations": ops,
	})
	if err != nil {
		return
	}

	for conn := range conns {
		go func(c *websocket.Conn) {
			if err := c.WriteMessage(websocket.TextMessage, msg); err != nil {
				b.Unsubscribe(c)
				c.Close()
			}
		}(conn)
	}
}
