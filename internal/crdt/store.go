package crdt

// StateStore is the persistence interface for CRDT operations and snapshots.
type StateStore interface {
	// Append persists a single operation.
	Append(op Operation) error

	// GetOps returns all operations for a document that are newer than the
	// version vector. When since is nil or empty all known ops are returned.
	GetOps(docID string, since map[string]Timestamp) ([]Operation, error)

	// GetLatestState returns the latest projected state for a document as a
	// JSON-friendly map.
	GetLatestState(docID string) (map[string]any, error)

	// GetVersionVector returns the per-author high-water-mark timestamps for
	// a document.
	GetVersionVector(docID string) (map[string]Timestamp, error)
}
