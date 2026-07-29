package crdt

import (
	"sync"
	"time"
)

// Timestamp represents a Hybrid Logical Clock timestamp.
type Timestamp struct {
	WallTime int64  `json:"wall_time"` // nanosecond precision
	Logical  uint32 `json:"logical"`
}

// HLC is a thread-safe Hybrid Logical Clock.
type HLC struct {
	mu       sync.Mutex
	wallTime int64
	logical  uint32
}

// NewHLC returns a new HLC initialised to the current wall time.
func NewHLC() *HLC {
	return &HLC{
		wallTime: time.Now().UnixNano(),
	}
}

// Now returns the current timestamp and advances the clock.
func (h *HLC) Now() Timestamp {
	h.mu.Lock()
	defer h.mu.Unlock()

	wt := time.Now().UnixNano()
	if wt > h.wallTime {
		h.wallTime = wt
	}
	h.logical++

	return Timestamp{WallTime: h.wallTime, Logical: h.logical}
}

// Observe merges a received timestamp into this clock to maintain
// causal ordering across nodes.
func (h *HLC) Observe(ts Timestamp) {
	h.mu.Lock()
	defer h.mu.Unlock()

	switch {
	case ts.WallTime > h.wallTime:
		h.wallTime = ts.WallTime
		h.logical = ts.Logical + 1
	case ts.WallTime == h.wallTime:
		if ts.Logical > h.logical {
			h.logical = ts.Logical
		}
		h.logical++
	default: // ts.WallTime < h.wallTime
		h.logical++
	}
}
