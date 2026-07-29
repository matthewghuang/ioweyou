package crdt

import "encoding/json"

// OpType identifies the kind of CRDT operation.
type OpType string

const (
	OpLWW       OpType = "lww"
	OpRGAInsert OpType = "rga_insert"
	OpRGADelete OpType = "rga_delete"
)

// Operation represents a single atomic mutation on a CRDT document.
type Operation struct {
	DocID      string          `json:"doc_id"`
	OpType     OpType          `json:"op_type"`
	Field      string          `json:"field,omitempty"`
	Value      json.RawMessage `json:"value"`
	ItemID     string          `json:"item_id,omitempty"`
	PrevItemID string          `json:"prev_item_id,omitempty"`
	AuthorID   string          `json:"author_id"`
	Timestamp  Timestamp       `json:"timestamp"`
}
