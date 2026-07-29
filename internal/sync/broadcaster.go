package sync

import (
	"encoding/json"
	"sync"

	"github.com/matthewghuang/ioweyou/internal/crdt"
)

// Broadcaster manages WebSocket subscriptions by group_id.
// Messages are sent to each subscriber's send channel, which the
// WebSocket write pump reads from — this avoids concurrent writes
// to a single websocket.Conn.
type Broadcaster struct {
	mu   sync.RWMutex
	subs map[string]map[chan []byte]bool // group_id -> set of send channels
}

// NewBroadcaster creates a new Broadcaster.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{subs: make(map[string]map[chan []byte]bool)}
}

// Subscribe registers a send channel to receive broadcasts for a group.
func (b *Broadcaster) Subscribe(groupID string, sendCh chan []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.subs[groupID] == nil {
		b.subs[groupID] = make(map[chan []byte]bool)
	}
	b.subs[groupID][sendCh] = true
}

// Unsubscribe removes a send channel from all groups.
func (b *Broadcaster) Unsubscribe(sendCh chan []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for gid, chs := range b.subs {
		delete(chs, sendCh)
		if len(chs) == 0 {
			delete(b.subs, gid)
		}
	}
}

// Broadcast sends an operation change notification to all subscribers of a group.
// The message is {"type":"change","operations":[...]}.
// Messages are sent to each subscriber's send channel (non-blocking). The
// receiver's write pump is responsible for writing to the WebSocket connection.
func (b *Broadcaster) Broadcast(groupID string, ops []crdt.Operation) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	chs := b.subs[groupID]

	if len(chs) == 0 {
		return
	}

	msg, err := json.Marshal(map[string]any{
		"type":       "change",
		"operations": ops,
	})
	if err != nil {
		return
	}

	for ch := range chs {
		select {
		case ch <- msg:
		default:
			// Channel full — subscriber is too slow, drop message
		}
	}
}
