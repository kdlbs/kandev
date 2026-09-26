package engine

import "testing"

// Regression coverage for idempotencyKey's EntryID/OperationID precedence
// (AC-OFFICE-STEP-ENTRY-001.2/.7/.10): EntryID takes priority when set
// (AC-003.2's "same EntryID twice produces the same key" contract, mirrored
// here as same EntryID -> same key regardless of OperationID), and falls
// back to OperationID only when EntryID is empty (AC-003.6's
// opposite-direction contract: different EntryID must still change the
// key even when OperationID stays the same).

func TestIdempotencyKey_SameEntryIDProducesSameKeyRegardlessOfOperationID(t *testing.T) {
	base := ActionInput{
		Trigger: TriggerOnEnter,
		EntryID: "entry-1",
		Step:    StepSpec{ID: "step-1"},
		Action:  Action{Kind: ActionRunCodeReview},
	}
	variant := base
	variant.OperationID = "op-different-each-time"

	key1 := idempotencyKey(base, "agent-1", "task-1")
	key2 := idempotencyKey(variant, "agent-1", "task-1")

	if key1 == "" {
		t.Fatal("expected non-empty key for non-empty EntryID")
	}
	if key1 != key2 {
		t.Fatalf("expected identical keys for the same EntryID regardless of OperationID, got %q vs %q", key1, key2)
	}
}

func TestIdempotencyKey_DifferentEntryIDProducesDifferentKey(t *testing.T) {
	first := ActionInput{
		Trigger: TriggerOnEnter,
		EntryID: "entry-1",
		Step:    StepSpec{ID: "step-1"},
		Action:  Action{Kind: ActionRunCodeReview},
	}
	second := first
	second.EntryID = "entry-2"

	key1 := idempotencyKey(first, "agent-1", "task-1")
	key2 := idempotencyKey(second, "agent-1", "task-1")

	if key1 == key2 {
		t.Fatalf("expected different keys for different EntryIDs, both got %q", key1)
	}
}

func TestIdempotencyKey_EmptyEntryIDFallsBackToOperationID(t *testing.T) {
	in := ActionInput{
		Trigger:     TriggerOnEnter,
		EntryID:     "",
		OperationID: "op-1",
		Step:        StepSpec{ID: "step-1"},
		Action:      Action{Kind: ActionRunCodeReview},
	}
	withOtherOp := in
	withOtherOp.OperationID = "op-2"

	key1 := idempotencyKey(in, "agent-1", "task-1")
	key2 := idempotencyKey(withOtherOp, "agent-1", "task-1")

	if key1 == "" {
		t.Fatal("expected non-empty key when OperationID is set")
	}
	if key1 == key2 {
		t.Fatalf("expected different keys for different OperationIDs when EntryID is empty, both got %q", key1)
	}
}

func TestIdempotencyKey_BothEmptyProducesEmptyKey(t *testing.T) {
	in := ActionInput{
		Trigger: TriggerOnEnter,
		Step:    StepSpec{ID: "step-1"},
		Action:  Action{Kind: ActionRunCodeReview},
	}
	if got := idempotencyKey(in, "agent-1", "task-1"); got != "" {
		t.Fatalf("expected empty key when both EntryID and OperationID are empty, got %q", got)
	}
}
