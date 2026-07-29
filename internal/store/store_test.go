package store

import (
	"encoding/json"
	"testing"

	"github.com/matthewghuang/ioweyou/internal/crdt"
)

func TestNewStore(t *testing.T) {
	s, err := NewSQLiteStateStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
}

func TestAppendAndGetOps(t *testing.T) {
	s, err := NewSQLiteStateStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	op := crdt.Operation{
		DocID: "doc1", OpType: crdt.OpLWW, Field: "name",
		Value: json.RawMessage(`"test"`), AuthorID: "alice",
		Timestamp: crdt.Timestamp{WallTime: 100, Logical: 1},
	}
	if err := s.Append(op); err != nil {
		t.Fatal(err)
	}

	ops, err := s.GetOps("doc1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
}

func TestAppendDedup(t *testing.T) {
	s, _ := NewSQLiteStateStore(":memory:")
	defer s.Close()

	op := crdt.Operation{
		DocID: "doc1", OpType: crdt.OpLWW, Field: "x",
		Value: json.RawMessage(`"v1"`), AuthorID: "alice",
		Timestamp: crdt.Timestamp{WallTime: 1, Logical: 1},
	}
	if err := s.Append(op); err != nil {
		t.Fatal(err)
	}
	// Same op again (same author+timestamp) -> ignored
	if err := s.Append(op); err != nil {
		t.Fatal(err)
	}
	ops, _ := s.GetOps("doc1", nil)
	if len(ops) != 1 {
		t.Fatalf("expected 1 op after dedup, got %d", len(ops))
	}
}

func TestGetLatestStateLWW(t *testing.T) {
	s, _ := NewSQLiteStateStore(":memory:")
	defer s.Close()

	s.Append(crdt.Operation{
		DocID: "doc1", OpType: crdt.OpLWW, Field: "color",
		Value: json.RawMessage(`"red"`), AuthorID: "alice",
		Timestamp: crdt.Timestamp{WallTime: 1, Logical: 1},
	})
	s.Append(crdt.Operation{
		DocID: "doc1", OpType: crdt.OpLWW, Field: "color",
		Value: json.RawMessage(`"blue"`), AuthorID: "bob",
		Timestamp: crdt.Timestamp{WallTime: 2, Logical: 1},
	})

	state, err := s.GetLatestState("doc1")
	if err != nil {
		t.Fatal(err)
	}
	if state["color"] != "blue" {
		t.Fatalf("expected 'blue', got %v", state["color"])
	}
}

func TestGetLatestStateRGA(t *testing.T) {
	s, _ := NewSQLiteStateStore(":memory:")
	defer s.Close()

	s.Append(crdt.Operation{
		DocID: "doc1", OpType: crdt.OpRGAInsert, Field: "items",
		ItemID: "i1", Value: json.RawMessage(`{"text":"A"}`),
		AuthorID: "alice", Timestamp: crdt.Timestamp{WallTime: 1, Logical: 1},
	})
	s.Append(crdt.Operation{
		DocID: "doc1", OpType: crdt.OpRGAInsert, Field: "items",
		ItemID: "i2", PrevItemID: "i1", Value: json.RawMessage(`{"text":"B"}`),
		AuthorID: "alice", Timestamp: crdt.Timestamp{WallTime: 2, Logical: 1},
	})

	state, _ := s.GetLatestState("doc1")
	items, ok := state["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("expected 2 items, got %v", state["items"])
	}
}

func TestGetVersionVector(t *testing.T) {
	s, _ := NewSQLiteStateStore(":memory:")
	defer s.Close()

	s.Append(crdt.Operation{
		DocID: "doc1", OpType: crdt.OpLWW, Field: "x",
		Value: json.RawMessage(`"1"`), AuthorID: "alice",
		Timestamp: crdt.Timestamp{WallTime: 5, Logical: 3},
	})

	vv, _ := s.GetVersionVector("doc1")
	ts, ok := vv["alice"]
	if !ok {
		t.Fatal("missing alice in version vector")
	}
	if ts.WallTime != 5 || ts.Logical != 3 {
		t.Fatalf("expected wall=5, logical=3; got wall=%d, logical=%d", ts.WallTime, ts.Logical)
	}
}

func TestGetOpsSince(t *testing.T) {
	s, _ := NewSQLiteStateStore(":memory:")
	defer s.Close()

	s.Append(crdt.Operation{
		DocID: "doc1", OpType: crdt.OpLWW, Field: "a",
		Value: json.RawMessage(`"1"`), AuthorID: "alice",
		Timestamp: crdt.Timestamp{WallTime: 1, Logical: 1},
	})
	s.Append(crdt.Operation{
		DocID: "doc1", OpType: crdt.OpLWW, Field: "b",
		Value: json.RawMessage(`"2"`), AuthorID: "alice",
		Timestamp: crdt.Timestamp{WallTime: 2, Logical: 1},
	})

	since := map[string]crdt.Timestamp{"alice": {WallTime: 1, Logical: 1}}
	ops, _ := s.GetOps("doc1", since)
	if len(ops) != 1 {
		t.Fatalf("expected 1 op after cursor, got %d", len(ops))
	}
	if string(ops[0].Value) != `"2"` {
		t.Fatalf("expected '2', got %s", string(ops[0].Value))
	}
}
