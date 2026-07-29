package crdt

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestVerifyOrderingBug(t *testing.T) {
	ops := []Operation{
		{DocID: "doc1", OpType: OpRGAInsert, Field: "items", ItemID: "i1", Value: json.RawMessage(`{"name":"first"}`), AuthorID: "alice", Timestamp: ts(1000, 1)},
		{DocID: "doc1", OpType: OpRGAInsert, Field: "items", ItemID: "i2", PrevItemID: "i1", Value: json.RawMessage(`{"name":"A"}`), AuthorID: "alice", Timestamp: ts(1000, 2)},
		{DocID: "doc1", OpType: OpRGAInsert, Field: "items", ItemID: "i3", PrevItemID: "i1", Value: json.RawMessage(`{"name":"B"}`), AuthorID: "bob", Timestamp: ts(1000, 3)},
	}

	merged := MergeState(ops, nil)
	snap := Snapshot(merged)
	items := snap["items"].([]any)
	fmt.Printf("Got %d items\n", len(items))
	for i, item := range items {
		m := item.(map[string]any)
		fmt.Printf("  items[%d] = %v\n", i, m["name"])
	}
	if len(items) >= 3 {
		first, A, B := items[0].(map[string]any)["name"], items[1].(map[string]any)["name"], items[2].(map[string]any)["name"]
		if first != "first" {
			t.Errorf("items[0] expected 'first', got %v", first)
		}
		if A != "A" {
			t.Errorf("items[1] expected 'A', got %v", A)
		}
		if B != "B" {
			t.Errorf("items[2] expected 'B', got %v", B)
		}
	}
}
