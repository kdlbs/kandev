package service

import "testing"

func TestWakeOperationID_IgnoresTerminalStateChanges(t *testing.T) {
	parentID := "parent-1"
	generation := "2026-01-01 00:00:00"
	first := wakeOperationID(parentID, "child-1:CANCELLED,child-2:COMPLETED", generation)
	second := wakeOperationID(parentID, "child-1:COMPLETED,child-2:COMPLETED", generation)

	if first != second {
		t.Fatalf("terminal state edit changed operation id: first=%q second=%q", first, second)
	}
}

func TestWakeOperationID_ChangesWhenChildSetChanges(t *testing.T) {
	parentID := "parent-1"
	generation := "2026-01-01 00:00:00"
	first := wakeOperationID(parentID, "child-1:COMPLETED", generation)
	second := wakeOperationID(parentID, "child-1:COMPLETED,child-2:COMPLETED", generation)

	if first == second {
		t.Fatalf("new child set reused operation id %q", first)
	}
}

func TestWakeOperationID_ChangesWhenGenerationChanges(t *testing.T) {
	parentID := "parent-1"
	childSetKey := "child-1:COMPLETED"
	first := wakeOperationID(parentID, childSetKey, "2026-01-01 00:00:00")
	second := wakeOperationID(parentID, childSetKey, "2026-01-01 00:00:01")

	if first == second {
		t.Fatalf("reopen+recomplete generation change reused operation id %q", first)
	}
}
