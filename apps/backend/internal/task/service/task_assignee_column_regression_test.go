package service

import (
	"context"
	"reflect"
	"testing"
)

// TestUpdateTask_AssigneeSurvivesOfficeMigration is a regression net for the
// office migration dropping tasks.assignee_user_id, and it is deliberately a
// service-level test rather than a repository one: the column is added by
// internal/task/repository/sqlite (ensureTeamAccessSchema) and then silently
// removed by internal/office's priority-to-TEXT rebuild, which recreates the
// tasks table from a hardcoded column list. Only a harness that runs BOTH
// repositories against one database sees the loss.
//
// Symptoms (before fix): every write path failed with "table tasks has no
// column named assignee_user_id" on a fresh install, while the ALTER that adds
// it reported success — MigrateLogger.Apply swallows errors, so nothing in the
// logs pointed at the rebuild.
func TestUpdateTask_AssigneeSurvivesOfficeMigration(t *testing.T) {
	svc, _ := setupOfficeTest(t)
	ctx := context.Background()

	created, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Assignable task",
		ProjectID:   "proj-1",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	assignee := "user-42"
	updated, err := svc.UpdateTask(ctx, created.Task.ID, &UpdateTaskRequest{
		AssigneeUserID: &assignee,
	})
	if err != nil {
		t.Fatalf("UpdateTask with assignee: %v", err)
	}
	if updated.AssigneeUserID != assignee {
		t.Fatalf("assignee not applied: got %q, want %q", updated.AssigneeUserID, assignee)
	}

	// Read it back through the repository so the assertion covers persistence,
	// not just the in-memory struct the update returned.
	reloaded, err := svc.GetTask(ctx, created.Task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if reloaded.AssigneeUserID != assignee {
		t.Fatalf("assignee did not persist: got %q, want %q", reloaded.AssigneeUserID, assignee)
	}
}

// TestUpdateTask_HumanAndAgentAssigneesAreIndependent pins the field
// independence the spec requires: a task can carry an agent runner and a human
// owner at once, and writing either one must leave the other alone. They are
// stored in different places (the agent assignee resolves through workflow
// step participants, the human one is a task column), so a regression here
// would most likely appear as one silently clearing the other.
func TestUpdateTask_HumanAndAgentAssigneesAreIndependent(t *testing.T) {
	svc, _ := setupOfficeTest(t)
	ctx := context.Background()

	created, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:            "ws-1",
		Title:                  "Dual assignee",
		ProjectID:              "proj-1",
		AssigneeAgentProfileID: "agent-1",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	agentAssignee := created.Task.AssigneeAgentProfileID
	if agentAssignee == "" {
		t.Fatal("fixture produced no agent assignee: the independence assertions below would be vacuous")
	}

	human := "user-42"
	updated, err := svc.UpdateTask(ctx, created.Task.ID, &UpdateTaskRequest{AssigneeUserID: &human})
	if err != nil {
		t.Fatalf("UpdateTask with human assignee: %v", err)
	}
	if updated.AssigneeUserID != human {
		t.Fatalf("human assignee = %q, want %q", updated.AssigneeUserID, human)
	}
	if updated.AssigneeAgentProfileID != agentAssignee {
		t.Fatalf("setting the human assignee changed the agent assignee: %q -> %q",
			agentAssignee, updated.AssigneeAgentProfileID)
	}

	// And the reverse: reassigning the human owner is what takeover is, so it
	// must not disturb the runner either.
	other := "user-7"
	reassigned, err := svc.UpdateTask(ctx, created.Task.ID, &UpdateTaskRequest{AssigneeUserID: &other})
	if err != nil {
		t.Fatalf("reassign: %v", err)
	}
	if reassigned.AssigneeAgentProfileID != agentAssignee {
		t.Fatalf("takeover changed the agent assignee: %q -> %q",
			agentAssignee, reassigned.AssigneeAgentProfileID)
	}
	if reassigned.AssigneeUserID != other {
		t.Fatalf("human assignee = %q, want %q", reassigned.AssigneeUserID, other)
	}
}

// TestUpdateTask_TakeoverLeavesExecutionStateUntouched is the "takeover is
// reassign plus continue, not a lock" guarantee: a reassignment writes exactly
// one field. Anything else changing here (state, workspace, session or
// workflow placement) would mean a handover silently disturbed a running
// agent's execution context.
func TestUpdateTask_TakeoverLeavesExecutionStateUntouched(t *testing.T) {
	svc, _ := setupOfficeTest(t)
	ctx := context.Background()

	created, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "In flight",
		ProjectID:   "proj-1",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	before, err := svc.GetTask(ctx, created.Task.ID)
	if err != nil {
		t.Fatalf("GetTask before: %v", err)
	}

	human := "user-42"
	if _, err := svc.UpdateTask(ctx, created.Task.ID, &UpdateTaskRequest{AssigneeUserID: &human}); err != nil {
		t.Fatalf("takeover: %v", err)
	}
	after, err := svc.GetTask(ctx, created.Task.ID)
	if err != nil {
		t.Fatalf("GetTask after: %v", err)
	}

	if after.AssigneeUserID != human {
		t.Fatalf("assignee = %q, want %q", after.AssigneeUserID, human)
	}
	// Normalise the two fields a reassignment is allowed to move, then compare
	// everything else field by field.
	after.AssigneeUserID = before.AssigneeUserID
	after.UpdatedAt = before.UpdatedAt
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("takeover changed more than the assignee:\nbefore: %+v\nafter:  %+v", before, after)
	}
}
