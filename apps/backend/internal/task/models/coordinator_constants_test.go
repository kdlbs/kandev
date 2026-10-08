package models

import "testing"

// TestTaskOriginCoordinator_Value pins the origin value copilot.md's
// conversation task creation and the startup cleanup pass key off of. It is
// declared here (task 01) for use by a later work package (task 03).
func TestTaskOriginCoordinator_Value(t *testing.T) {
	if TaskOriginCoordinator != "coordinator" {
		t.Errorf("TaskOriginCoordinator = %q, want %q", TaskOriginCoordinator, "coordinator")
	}
}

// TestMetaKeyCoordinatorID_Value pins the task metadata key a conversation
// task uses to record the coordinator that owns it.
func TestMetaKeyCoordinatorID_Value(t *testing.T) {
	if MetaKeyCoordinatorID != "coordinator_id" {
		t.Errorf("MetaKeyCoordinatorID = %q, want %q", MetaKeyCoordinatorID, "coordinator_id")
	}
}
