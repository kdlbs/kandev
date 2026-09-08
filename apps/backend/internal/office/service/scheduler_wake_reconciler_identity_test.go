package service

import "testing"

func TestWakeOperationID_IgnoresTerminalStateChanges(t *testing.T) {
	parentID := "parent-1"
	first := wakeOperationID(parentID, "child-1:CANCELLED,child-2:COMPLETED")
	second := wakeOperationID(parentID, "child-1:COMPLETED,child-2:COMPLETED")

	if first != second {
		t.Fatalf("terminal state edit changed operation id: first=%q second=%q", first, second)
	}
}

func TestWakeOperationID_ChangesWhenChildSetChanges(t *testing.T) {
	parentID := "parent-1"
	first := wakeOperationID(parentID, "child-1:COMPLETED")
	second := wakeOperationID(parentID, "child-1:COMPLETED,child-2:COMPLETED")

	if first == second {
		t.Fatalf("new child set reused operation id %q", first)
	}
}
