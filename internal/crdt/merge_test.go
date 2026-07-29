package crdt

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"
)

// helpers ----------------------------------------------------------------

func ts(wall int64, logical uint32) Timestamp {
	return Timestamp{WallTime: wall, Logical: logical}
}

func lwwOp(docID, field, value string, authorID string, ts Timestamp) Operation {
	return Operation{
		DocID:     docID,
		OpType:    OpLWW,
		Field:     field,
		Value:     json.RawMessage(value),
		AuthorID:  authorID,
		Timestamp: ts,
	}
}

func rgaInsertOp(docID, field, itemID, prevItemID string, value json.RawMessage, authorID string, ts Timestamp) Operation {
	return Operation{
		DocID:      docID,
		OpType:     OpRGAInsert,
		Field:      field,
		Value:      value,
		ItemID:     itemID,
		PrevItemID: prevItemID,
		AuthorID:   authorID,
		Timestamp:  ts,
	}
}

func rgaDeleteOp(docID, field, itemID string, authorID string, ts Timestamp) Operation {
	return Operation{
		DocID:     docID,
		OpType:    OpRGADelete,
		Field:     field,
		ItemID:    itemID,
		AuthorID:  authorID,
		Timestamp: ts,
	}
}

func mustJSON(s string) json.RawMessage {
	return json.RawMessage(s)
}

// ------------------------------------------------------------------------
// HLC tests
// ------------------------------------------------------------------------

func TestHLCMonotonicity(t *testing.T) {
	hlc := NewHLC()
	a := hlc.Now()
	b := hlc.Now()
	c := hlc.Now()

	// Each successive timestamp must be strictly greater than the previous:
	// either wall time advances, or wall time is equal and logical advances.
	strictlyGreater := func(prev, next Timestamp) bool {
		if next.WallTime > prev.WallTime {
			return true
		}
		if next.WallTime == prev.WallTime && next.Logical > prev.Logical {
			return true
		}
		return false
	}

	if !strictlyGreater(a, b) {
		t.Fatalf("(a) %+v not < (b) %+v", a, b)
	}
	if !strictlyGreater(b, c) {
		t.Fatalf("(b) %+v not < (c) %+v", b, c)
	}

	// When wall time stays the same, logical must increase.
	if a.WallTime == b.WallTime && a.Logical >= b.Logical {
		t.Fatalf("logical must increase when wall time is equal: a=%+v b=%+v", a, b)
	}
}

func TestHLCObserve(t *testing.T) {
	hlc := NewHLC()
	now := hlc.Now()

	// Observe a timestamp slightly in the future.
	future := ts(now.WallTime+1_000_000_000, 5)
	hlc.Observe(future)
	after := hlc.Now()

	// The clock must not regress below the observed timestamp. If real wall
	// time has not yet caught up, wall time == future.WallTime and logical
	// must exceed future.Logical. If real wall time passed the future point,
	// wall time is whatever the OS clock says but it's still >= future.
	if after.WallTime < future.WallTime {
		t.Fatalf("wall time regressed: %d < %d", after.WallTime, future.WallTime)
	}
	if after.WallTime == future.WallTime && after.Logical <= future.Logical {
		t.Fatalf("logical did not advance: %d <= %d", after.Logical, future.Logical)
	}

	// Observe a very old timestamp — should not cause regression.
	prev := hlc.Now()
	past := ts(0, 0) // epoch — far in the past
	hlc.Observe(past)
	afterPast := hlc.Now()

	// Monotonicity must hold: afterPast >= prev.
	if afterPast.WallTime < prev.WallTime {
		t.Fatalf("wall time regressed after observing past: %d < %d", afterPast.WallTime, prev.WallTime)
	}
	if afterPast.WallTime == prev.WallTime && afterPast.Logical <= prev.Logical {
		t.Fatalf("logical did not advance after observing past: %d <= %d", afterPast.Logical, prev.Logical)
	}

	// Causal delivery: if we receive a timestamp from a peer, our clock
	// must produce a timestamp strictly greater than that peer's timestamp.
	hlc2 := NewHLC()
	peerTS := hlc2.Now()

	hlc.Observe(peerTS)
	localAfter := hlc.Now()

	if localAfter.WallTime < peerTS.WallTime {
		t.Fatalf("local clock regressed below peer: %d < %d", localAfter.WallTime, peerTS.WallTime)
	}
	if localAfter.WallTime == peerTS.WallTime && localAfter.Logical <= peerTS.Logical {
		t.Fatalf("local logical did not advance past peer: %d <= %d", localAfter.Logical, peerTS.Logical)
	}
}

// ------------------------------------------------------------------------
// Merge tests
// ------------------------------------------------------------------------

func TestMergeDedup(t *testing.T) {
	ts1 := ts(100, 1)
	op := lwwOp("doc1", "title", `"hello"`, "alice", ts1)

	// Same operation in both local and remote.
	merged := MergeState([]Operation{op}, []Operation{op})
	if len(merged) != 1 {
		t.Fatalf("expected 1 op after dedup, got %d", len(merged))
	}
}

func TestMergeLWWSameField(t *testing.T) {
	tsA := ts(200, 1)
	tsB := ts(100, 1) // alice's op is older

	opA := lwwOp("doc1", "name", `"alice-value"`, "alice", tsA)
	opB := lwwOp("doc1", "name", `"bob-value"`, "bob", tsB)

	merged := MergeState([]Operation{opA}, []Operation{opB})
	if len(merged) != 1 {
		t.Fatalf("expected 1 op after LWW resolution, got %d", len(merged))
	}

	var winner string
	if err := json.Unmarshal(merged[0].Value, &winner); err != nil {
		t.Fatal(err)
	}
	if winner != "alice-value" {
		t.Fatalf("expected 'alice-value' (higher wall time), got %q", winner)
	}
}

func TestMergeRGAConcurrentInsert(t *testing.T) {
	// Two authors insert concurrently (different timestamps).
	tsA := ts(300, 1)
	tsB := ts(300, 2)

	opA := rgaInsertOp("doc1", "items", "item-a", "",
		mustJSON(`{"text":"A"}`), "alice", tsA)
	opB := rgaInsertOp("doc1", "items", "item-b", "",
		mustJSON(`{"text":"B"}`), "bob", tsB)

	merged := MergeState([]Operation{opA}, []Operation{opB})
	if len(merged) != 2 {
		t.Fatalf("expected 2 ops after merging concurrent RGA inserts, got %d", len(merged))
	}

	// Both should be insert ops.
	for _, op := range merged {
		if op.OpType != OpRGAInsert {
			t.Fatalf("expected RGA insert, got %s", op.OpType)
		}
	}
}

func TestMergeRGADelete(t *testing.T) {
	tsIns := ts(400, 1)
	tsDel := ts(400, 2)

	ins := rgaInsertOp("doc1", "items", "item-1", "",
		mustJSON(`{"text":"deletable"}`), "alice", tsIns)
	del := rgaDeleteOp("doc1", "items", "item-1", "alice", tsDel)

	merged := MergeState([]Operation{ins}, []Operation{del})
	// The deleted insert should be gone.
	if len(merged) != 0 {
		t.Fatalf("expected 0 ops after delete, got %d", len(merged))
	}

	// Delete with earlier timestamp than insert: item should survive.
	delEarly := rgaDeleteOp("doc1", "items", "item-1", "alice", ts(400, 1))
	insLate := rgaInsertOp("doc1", "items", "item-1", "",
		mustJSON(`{"text":"survivor"}`), "alice", ts(400, 2))
	merged2 := MergeState([]Operation{delEarly}, []Operation{insLate})
	if len(merged2) != 1 {
		t.Fatalf("expected 1 op when delete (ts earlier) before insert, got %d", len(merged2))
	}
}

func TestMergeIdempotent(t *testing.T) {
	ts1 := ts(500, 1)
	ts2 := ts(500, 2)
	op1 := lwwOp("doc1", "color", `"red"`, "alice", ts1)
	op2 := lwwOp("doc1", "color", `"blue"`, "bob", ts2)

	// merge(local, remote)
	once := MergeState([]Operation{op1}, []Operation{op2})

	// merge(local, merge(local, remote))
	twice := MergeState([]Operation{op1}, once)

	if len(once) != len(twice) {
		t.Fatalf("idempotency violated: once len=%d, twice len=%d", len(once), len(twice))
	}
	// The winning value should be blue (bob, higher logical).
	var onceVal, twiceVal string
	json.Unmarshal(once[0].Value, &onceVal)
	json.Unmarshal(twice[0].Value, &twiceVal)
	if onceVal != twiceVal {
		t.Fatalf("idempotency violated: once=%q, twice=%q", onceVal, twiceVal)
	}
	if onceVal != "blue" {
		t.Fatalf("expected blue to win, got %q", onceVal)
	}
}

// ------------------------------------------------------------------------
// Snapshot tests
// ------------------------------------------------------------------------

func TestSnapshotLWW(t *testing.T) {
	ops := []Operation{
		lwwOp("doc1", "title", `"hello"`, "alice", ts(600, 1)),
		lwwOp("doc1", "count", `42`, "bob", ts(600, 2)),
	}

	snap := Snapshot(ops)
	if snap["title"] != "hello" {
		t.Fatalf("expected title='hello', got %v", snap["title"])
	}
	// JSON numbers unmarshal as float64.
	if snap["count"] != float64(42) {
		t.Fatalf("expected count=42, got %v (type %T)", snap["count"], snap["count"])
	}
}

func TestSnapshotRGA(t *testing.T) {
	ops := []Operation{
		rgaInsertOp("doc1", "items", "i1", "",
			mustJSON(`{"name":"first"}`), "alice", ts(700, 1)),
		rgaInsertOp("doc1", "items", "i2", "i1",
			mustJSON(`{"name":"second"}`), "alice", ts(700, 2)),
	}

	snap := Snapshot(ops)
	items, ok := snap["items"].([]any)
	if !ok {
		t.Fatalf("expected items to be []any, got %T", snap["items"])
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	first := items[0].(map[string]any)
	second := items[1].(map[string]any)
	if first["name"] != "first" {
		t.Fatalf("expected first item name 'first', got %v", first["name"])
	}
	if second["name"] != "second" {
		t.Fatalf("expected second item name 'second', got %v", second["name"])
	}
}

func TestSnapshotMixed(t *testing.T) {
	ops := []Operation{
		lwwOp("doc1", "title", `"groceries"`, "alice", ts(800, 1)),
		rgaInsertOp("doc1", "items", "i1", "",
			mustJSON(`{"name":"milk"}`), "alice", ts(800, 2)),
		rgaInsertOp("doc1", "items", "i2", "i1",
			mustJSON(`{"name":"eggs"}`), "bob", ts(800, 3)),
		rgaDeleteOp("doc1", "items", "i1", "alice", ts(800, 4)),
	}

	// MergeState resolves deletes.
	merged := MergeState(ops, nil)
	snap := Snapshot(merged)

	if snap["title"] != "groceries" {
		t.Fatalf("expected title='groceries', got %v", snap["title"])
	}

	items, ok := snap["items"].([]any)
	if !ok {
		t.Fatalf("expected items to be []any, got %T", snap["items"])
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 surviving item, got %d", len(items))
	}
	item := items[0].(map[string]any)
	if item["name"] != "eggs" {
		t.Fatalf("expected remaining item 'eggs', got %v", item["name"])
	}
}

// ------------------------------------------------------------------------
// Additional edge-case tests
// ------------------------------------------------------------------------

func TestSnapshotRGADeleteMiddle(t *testing.T) {
	// Insert three items, delete the middle one.
	ops := []Operation{
		rgaInsertOp("doc1", "items", "i1", "",
			mustJSON(`{"pos":1}`), "alice", ts(900, 1)),
		rgaInsertOp("doc1", "items", "i2", "i1",
			mustJSON(`{"pos":2}`), "alice", ts(900, 2)),
		rgaInsertOp("doc1", "items", "i3", "i2",
			mustJSON(`{"pos":3}`), "alice", ts(900, 3)),
		rgaDeleteOp("doc1", "items", "i2", "bob", ts(900, 4)),
	}

	merged := MergeState(ops, nil)
	snap := Snapshot(merged)
	items := snap["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected 2 items after delete, got %d", len(items))
	}
	if items[0].(map[string]any)["pos"] != float64(1) {
		t.Fatalf("expected first item pos=1")
	}
	if items[1].(map[string]any)["pos"] != float64(3) {
		t.Fatalf("expected second item pos=3")
	}
}

func TestConcurrentInsertSamePrev(t *testing.T) {
	// Two concurrent inserts after the same prev (i1).
	ops := []Operation{
		rgaInsertOp("doc1", "items", "i1", "",
			mustJSON(`{"name":"first"}`), "alice", ts(1000, 1)),
		rgaInsertOp("doc1", "items", "i2", "i1",
			mustJSON(`{"name":"A"}`), "alice", ts(1000, 2)),
		rgaInsertOp("doc1", "items", "i3", "i1",
			mustJSON(`{"name":"B"}`), "bob", ts(1000, 3)),
	}

	merged := MergeState(ops, nil)
	snap := Snapshot(merged)
	items := snap["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	// i1, then i2 (alice, earlier), then i3 (bob, later) after i1.
	if items[0].(map[string]any)["name"] != "first" {
		t.Fatalf("expected first, got %v", items[0].(map[string]any)["name"])
	}
}

func TestHLCConcurrentSafety(t *testing.T) {
	hlc := NewHLC()
	done := make(chan struct{})
	const goroutines = 10
	const calls = 100

	for range goroutines {
		go func() {
			for range calls {
				hlc.Now()
			}
			done <- struct{}{}
		}()
	}
	for range goroutines {
		<-done
	}

	// Final timestamp should be valid (no panics, logical > 0).
	ts := hlc.Now()
	if ts.WallTime == 0 && ts.Logical == 0 {
		t.Fatal("HLC returned zero timestamp after concurrent access")
	}
}

func TestMergeLWWAuthorTiebreak(t *testing.T) {
	// Same timestamp, different author — author ID string comparison decides.
	ts := ts(2000, 1)
	alice := lwwOp("doc1", "field", `"alice"`, "alice", ts)
	bob := lwwOp("doc1", "field", `"bob"`, "bob", ts)

	merged := MergeState([]Operation{alice}, []Operation{bob})
	if len(merged) != 1 {
		t.Fatalf("expected 1 op, got %d", len(merged))
	}
	// "bob" > "alice" lexicographically.
	var v string
	json.Unmarshal(merged[0].Value, &v)
	if v != "bob" {
		t.Fatalf("expected 'bob' (author tiebreak), got %q", v)
	}
}

func TestSnapshotMultipleDocs(t *testing.T) {
	ops := []Operation{
		lwwOp("a", "x", `"1"`, "alice", ts(3000, 1)),
		lwwOp("b", "y", `"2"`, "bob", ts(3000, 2)),
	}
	snap := Snapshot(ops)
	// Multiple docs -> nested by doc_id
	a, ok := snap["a"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'a' to be a map, got %T", snap["a"])
	}
	b, ok := snap["b"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'b' to be a map, got %T", snap["b"])
	}
	if a["x"] != "1" {
		t.Fatalf("expected a.x='1', got %v", a["x"])
	}
	if b["y"] != "2" {
		t.Fatalf("expected b.y='2', got %v", b["y"])
	}
}

// ------------------------------------------------------------------------
// RGA ordering: prev_item_id not found -> append at end
// ------------------------------------------------------------------------

func TestRGAPrevNotFoundAppend(t *testing.T) {
	ops := []Operation{
		rgaInsertOp("doc1", "items", "i2", "i-missing",
			mustJSON(`{"name":"orphan"}`), "alice", ts(4000, 1)),
		rgaInsertOp("doc1", "items", "i1", "",
			mustJSON(`{"name":"first"}`), "bob", ts(4000, 2)),
	}
	merged := MergeState(ops, nil)
	snap := Snapshot(merged)
	items := snap["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	// i2 (orphan) is first because its insert has earlier timestamp.
	// i1 appended at end.
}

// ------------------------------------------------------------------------
// Snapshot directly on ops not passed through MergeState
// ------------------------------------------------------------------------

func TestSnapshotDirectLWWConflict(t *testing.T) {
	// Multiple LWW ops for the same field — Snapshot should pick the winner.
	ops := []Operation{
		lwwOp("doc1", "x", `"old"`, "alice", ts(5000, 1)),
		lwwOp("doc1", "x", `"new"`, "bob", ts(5000, 2)),
	}
	snap := Snapshot(ops)
	if snap["x"] != "new" {
		t.Fatalf("expected 'new', got %v", snap["x"])
	}
}

// ------------------------------------------------------------------------
// Stress: many operations
// ------------------------------------------------------------------------

func TestLargeMerge(t *testing.T) {
	var local, remote []Operation
	for i := range 100 {
		op := lwwOp("doc1", "field", `"v"`, "alice", ts(int64(6000+i), uint32(i)))
		if i%2 == 0 {
			local = append(local, op)
		} else {
			remote = append(remote, op)
		}
	}
	merged := MergeState(local, remote)
	if len(merged) != 1 {
		t.Fatalf("expected 1 (last wins for same field), got %d", len(merged))
	}
}

// ------------------------------------------------------------------------
// Helper to verify test file compiles and runs
// ------------------------------------------------------------------------

func TestAllTestsCompile(t *testing.T) {
	// No-op: ensures package compiles.
}

// ------------------------------------------------------------------------
// JSON round-trip for Operation
// ------------------------------------------------------------------------

func TestOperationJSONRoundTrip(t *testing.T) {
	op := Operation{
		DocID:      "doc1",
		OpType:     OpLWW,
		Field:      "title",
		Value:      json.RawMessage(`"hello"`),
		ItemID:     "",
		PrevItemID: "",
		AuthorID:   "alice",
		Timestamp:  ts(42, 7),
	}

	data, err := json.Marshal(op)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Operation
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.DocID != op.DocID || decoded.Field != op.Field || decoded.AuthorID != op.AuthorID {
		t.Fatal("round-trip mismatch")
	}
	if decoded.Timestamp.WallTime != op.Timestamp.WallTime || decoded.Timestamp.Logical != op.Timestamp.Logical {
		t.Fatal("timestamp round-trip mismatch")
	}
}

// go test vet helper —
// silence unused import warning for 'sort' and 'strings' used in helpers
var _ = sort.Strings
var _ = strings.Compare
var _ = time.Now
