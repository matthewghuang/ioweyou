package crdt

import (
	"encoding/json"
	"fmt"
	"sort"
)

// opOrigin records whether an operation originated from the local or remote slice.
type opOrigin int

const (
	originLocal  opOrigin = 1
	originRemote opOrigin = 2
)

// MergeState takes local and remote operation slices, deduplicates by
// (AuthorID, WallTime, Logical), resolves LWW field conflicts by keeping
// the highest-timestamp operation per (doc_id, field), and reconstructs RGA
// lists by ordering surviving inserts and removing deleted items.
//
// The returned slice is the canonical merged operation set.
func MergeState(local, remote []Operation) []Operation {
	// ---- deduplicate and track origins ----
	type dedupKey struct {
		docID    string
		authorID string
		wallTime int64
		logical  uint32
	}
	seen := make(map[dedupKey]struct{})
	opOriginStr := make(map[string]opOrigin)

	var deduped []Operation
	add := func(op Operation, org opOrigin) {
		k := dedupKey{docID: op.DocID, authorID: op.AuthorID, wallTime: op.Timestamp.WallTime, logical: op.Timestamp.Logical}
		if _, ok := seen[k]; !ok {
			seen[k] = struct{}{}
			opOriginStr[opKey(op)] = org
			deduped = append(deduped, op)
		}
	}
	for _, op := range local {
		add(op, originLocal)
	}
	for _, op := range remote {
		add(op, originRemote)
	}

	// ---- split by op type ----
	type docField struct {
		docID string
		field string
	}

	lwwByField := make(map[docField][]Operation)
	var rgaInserts []Operation
	var rgaDeletes []Operation
	// Track origin of RGA operations for speculative-delete semantics.
	// A delete from local only applies to inserts that were also present in
	// the local slice (the deleting node knew about them).
	isLocalOp := make(map[string]bool)

	for _, op := range deduped {
		switch op.OpType {
		case OpLWW:
			df := docField{docID: op.DocID, field: op.Field}
			lwwByField[df] = append(lwwByField[df], op)
		case OpRGAInsert:
			rgaInserts = append(rgaInserts, op)
			if opOriginStr[opKey(op)] == originLocal {
				isLocalOp[opKey(op)] = true
			}
		case OpRGADelete:
			rgaDeletes = append(rgaDeletes, op)
			if opOriginStr[opKey(op)] == originLocal {
				isLocalOp[opKey(op)] = true
			}
		}
	}

	// ---- LWW: keep winner per (doc_id, field) ----
	var result []Operation
	for _, ops := range lwwByField {
		winner := ops[0]
		for i := 1; i < len(ops); i++ {
			if compareOpTimestamp(ops[i], winner) > 0 {
				winner = ops[i]
			}
		}
		result = append(result, winner)
	}

	// ---- RGA: resolve deletions and order surviving inserts ----
	rgaInsertByField := make(map[docField][]Operation)
	for _, ins := range rgaInserts {
		k := docField{docID: ins.DocID, field: ins.Field}
		rgaInsertByField[k] = append(rgaInsertByField[k], ins)
	}

	// Build delete-after-insert set.
	deletedSet := buildDeleteSet(rgaInserts, rgaDeletes, isLocalOp)

	for _, inserts := range rgaInsertByField {
		// Sort inserts by (WallTime, Logical, AuthorID, ItemID).
		sort.Slice(inserts, func(i, j int) bool {
			return compareOpTimestampItem(inserts[i], inserts[j]) < 0
		})

		// Filter out deleted items.
		var alive []Operation
		for _, ins := range inserts {
			if !deletedSet[ins.ItemID] {
				alive = append(alive, ins)
			}
		}

		// Build list respecting prev_item_id.
		ordered := orderRGAList(alive)

		// Each surviving insert in order is a result operation (the delete
		// ops are not emitted — deletions are absorbed into the absence).
		result = append(result, ordered...)
	}

	return result
}

// buildDeleteSet returns the set of item_ids that have a delete op whose
// timestamp is greater than at least one corresponding insert op.
//
// isLocalOp marks operations that originated from the local slice. A delete
// from the local node is treated as speculative: it only applies to inserts
// that were also present in the local slice (the deleting node knew about
// them). A delete from remote applies to all inserts.
func buildDeleteSet(inserts, deletes []Operation, isLocalOp map[string]bool) map[string]bool {
	deleted := make(map[string]bool)
	for _, ins := range inserts {
		for _, del := range deletes {
			if del.ItemID != ins.ItemID {
				continue
			}
			if compareOpTimestamp(del, ins) <= 0 {
				continue
			}
			// A delete from the local node only applies to inserts also
			// from the local node (speculative-delete semantics).
			if isLocalOp[opKey(del)] && !isLocalOp[opKey(ins)] {
				continue
			}
			deleted[ins.ItemID] = true
			break
		}
	}
	return deleted
}

// orderRGAList orders surviving inserts by their prev_item_id references.
// Inserts are assumed already sorted by (WallTime, Logical, AuthorID, ItemID).
// When prev_item_id is not found (deleted or never existed) the item is
// appended at the end of the list.
func orderRGAList(inserts []Operation) []Operation {
	type listEntry struct {
		op     Operation
		itemID string
	}

	var list []listEntry
	for _, ins := range inserts {
		// Find the position of prev_item_id in the current list.
		pos := -1
		if ins.PrevItemID != "" {
			for i, entry := range list {
				if entry.itemID == ins.PrevItemID {
					pos = i
					break
				}
			}
		}

		entry := listEntry{op: ins, itemID: ins.ItemID}
		if pos == -1 {
			// Not found or empty prev — append at end.
			list = append(list, entry)
		} else {
			// Find the correct position: items after prev that have a LOWER timestamp
			// than this insert should come first (inserts arrive in timestamp order).
			insPos := pos + 1
			for insPos < len(list) && compareOpTimestamp(list[insPos].op, ins) < 0 {
				insPos++
			}
			// Insert at insPos.
			list = append(list, listEntry{})
			copy(list[insPos+1:], list[insPos:])
			list[insPos] = entry
		}
	}

	out := make([]Operation, len(list))
	for i, e := range list {
		out[i] = e.op
	}
	return out
}

// compareOpTimestamp compares two operations by (WallTime, Logical, AuthorID).
// Returns -1, 0, or 1.
func compareOpTimestamp(a, b Operation) int {
	if a.Timestamp.WallTime != b.Timestamp.WallTime {
		if a.Timestamp.WallTime > b.Timestamp.WallTime {
			return 1
		}
		return -1
	}
	if a.Timestamp.Logical != b.Timestamp.Logical {
		if a.Timestamp.Logical > b.Timestamp.Logical {
			return 1
		}
		return -1
	}
	switch {
	case a.AuthorID < b.AuthorID:
		return -1
	case a.AuthorID > b.AuthorID:
		return 1
	}
	return 0
}

// compareOpTimestampItem compares by (WallTime, Logical, AuthorID, ItemID).
func compareOpTimestampItem(a, b Operation) int {
	if c := compareOpTimestamp(a, b); c != 0 {
		return c
	}
	switch {
	case a.ItemID < b.ItemID:
		return -1
	case a.ItemID > b.ItemID:
		return 1
	}
	return 0
}

// Snapshot projects a merged (canonical) operation set into a JSON-friendly
// map. LWW fields become scalar values (the winner by timestamp). RGA lists
// become ordered slices of item objects.
//
// DocID is used to group operations that belong to the same document. If all
// operations share a single DocID the top-level keys are field names;
// otherwise the result is keyed by DocID with nested field maps.
func Snapshot(ops []Operation) map[string]any {
	// ---- split by doc_id ----
	byDoc := make(map[string][]Operation)
	for _, op := range ops {
		byDoc[op.DocID] = append(byDoc[op.DocID], op)
	}

	if len(byDoc) == 1 {
		for _, docOps := range byDoc {
			return snapshotDoc(docOps)
		}
	}

	out := make(map[string]any, len(byDoc))
	for docID, docOps := range byDoc {
		out[docID] = snapshotDoc(docOps)
	}
	return out
}

// snapshotDoc projects operations for a single document into a map.
func snapshotDoc(ops []Operation) map[string]any {
	out := make(map[string]any)

	// Collect LWW ops per field, pick winner.
	lwwByField := make(map[string][]Operation) // field -> ops
	rgaByField := make(map[string][]Operation) // field -> inserts

	for _, op := range ops {
		switch op.OpType {
		case OpLWW:
			lwwByField[op.Field] = append(lwwByField[op.Field], op)
		case OpRGAInsert:
			rgaByField[op.Field] = append(rgaByField[op.Field], op)
		}
	}

	// Build a delete set from all RGA deletes in the input (local-only
	// speculative semantics are not needed in a pure snapshot — every
	// delete whose timestamp beats its corresponding insert applies).
	deletedSet := buildDeleteSetFromOps(ops)

	// LWW fields.
	for field, fops := range lwwByField {
		winner := fops[0]
		for i := 1; i < len(fops); i++ {
			if compareOpTimestamp(fops[i], winner) > 0 {
				winner = fops[i]
			}
		}
		var v any
		if err := json.Unmarshal(winner.Value, &v); err == nil {
			out[field] = v
		}
	}

	// RGA fields.
	for field, inserts := range rgaByField {
		// Filter out deleted items.
		var alive []Operation
		for _, ins := range inserts {
			if !deletedSet[ins.ItemID] {
				alive = append(alive, ins)
			}
		}
		sort.Slice(alive, func(i, j int) bool {
			return compareOpTimestampItem(alive[i], alive[j]) < 0
		})
		// Order by prev_item_id.
		ordered := orderRGAList(alive)
		arr := make([]any, 0, len(ordered))
		for _, ins := range ordered {
			var v any
			if err := json.Unmarshal(ins.Value, &v); err == nil {
				arr = append(arr, v)
			}
		}
		if arr != nil {
			out[field] = arr
		}
	}

	return out
}

// buildDeleteSetFromOps returns the set of item_ids that have a delete op
// whose timestamp is strictly greater than at least one corresponding insert
// op among the given operations.
func buildDeleteSetFromOps(ops []Operation) map[string]bool {
	var inserts []Operation
	var deletes []Operation
	for _, op := range ops {
		switch op.OpType {
		case OpRGAInsert:
			inserts = append(inserts, op)
		case OpRGADelete:
			deletes = append(deletes, op)
		}
	}
	deleted := make(map[string]bool)
	for _, ins := range inserts {
		for _, del := range deletes {
			if del.ItemID != ins.ItemID {
				continue
			}
			if compareOpTimestamp(del, ins) <= 0 {
				continue
			}
			deleted[ins.ItemID] = true
			break
		}
	}
	return deleted
}

// opKey returns the deduplication key for an operation.
func opKey(op Operation) string {
	return fmt.Sprintf("%s|%s|%d|%d", op.DocID, op.AuthorID, op.Timestamp.WallTime, op.Timestamp.Logical)
}
