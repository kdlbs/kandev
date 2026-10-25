package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TestCreateTaskRefusesReservedExternalIDPrefix pins
// docs/specs/coordinator/system-design/proposals.md#reserved-prefix: an
// ordinary create (the shape every HTTP and MCP request produces, since
// AllowReservedExternalID is json:"-" and can never be set from a decoded
// body) is refused with ErrExternalIDInvalid, and no task is created.
func TestCreateTaskRefusesReservedExternalIDPrefix(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	wfID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-reserved")

	_, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-reserved",
		WorkflowID:  wfID,
		Title:       "Task",
		ExternalID:  ReservedExternalIDPrefixCoordinatorProposal + "abc",
	})
	if !errors.Is(err, ErrExternalIDInvalid) {
		t.Fatalf("err = %v, want ErrExternalIDInvalid", err)
	}
	if countTasksHoldingExternalID(t, ctx, repo, "ws-reserved", ReservedExternalIDPrefixCoordinatorProposal+"abc") != 0 {
		t.Fatal("a refused create must not create a task")
	}
}

// TestCreateTaskAllowsReservedExternalIDPrefixWhenFlagged pins the
// coordinator service's own path: with AllowReservedExternalID set, the
// prefix is accepted and the task is created.
func TestCreateTaskAllowsReservedExternalIDPrefixWhenFlagged(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	wfID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-reserved-ok")

	externalID := ReservedExternalIDPrefixCoordinatorProposal + "abc"
	result, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:             "ws-reserved-ok",
		WorkflowID:              wfID,
		Title:                   "Task",
		ExternalID:              externalID,
		AllowReservedExternalID: true,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if result.Outcome != CreateTaskOutcomeCreated {
		t.Fatalf("Outcome = %v, want Created", result.Outcome)
	}
	if result.Task.ExternalID != externalID {
		t.Fatalf("ExternalID = %q, want %q", result.Task.ExternalID, externalID)
	}
}

// TestReleaseTaskExternalIDRefusesReservedPrefix pins the release half of
// the reserved-prefix contract: even a task that legitimately holds a
// coordinator-proposal: id (created with the flag) cannot have that
// identity released through the general-purpose release route, so the
// binding a completed proposal depends on can never be pulled out from
// under it.
func TestReleaseTaskExternalIDRefusesReservedPrefix(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-release", Name: "WS"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	externalID := ReservedExternalIDPrefixCoordinatorProposal + "xyz"
	task := &models.Task{WorkspaceID: "ws-release", Title: "Task", ExternalID: externalID}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	_, err := svc.ReleaseTaskExternalID(ctx, "ws-release", externalID)
	if !errors.Is(err, ErrExternalIDInvalid) {
		t.Fatalf("err = %v, want ErrExternalIDInvalid", err)
	}

	held, lookupErr := svc.GetTaskByExternalID(ctx, "ws-release", externalID)
	if lookupErr != nil {
		t.Fatalf("GetTaskByExternalID: %v", lookupErr)
	}
	if held.ID != task.ID {
		t.Fatal("release must leave the reserved external_id bound to its task")
	}
}
