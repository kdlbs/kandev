package service

import (
	"context"
	"testing"
)

// TestCreateTaskWithExternalIDReturnsArchivedTaskUnchanged covers the
// lifecycle scenario: an archived task still holds its identity, and a
// retry against it returns the same (still-archived) task rather than
// creating a new one or un-archiving it.
func TestCreateTaskWithExternalIDReturnsArchivedTaskUnchanged(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	wfID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-archived")

	first, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-archived", WorkflowID: wfID, Title: "Task", ExternalID: "ext-1",
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	mustSettle(t, ctx, repo, first.Task.ID, "ext-1")
	if err := svc.ArchiveTask(ctx, first.Task.ID); err != nil {
		t.Fatalf("archive task: %v", err)
	}

	retry, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-archived", WorkflowID: wfID, Title: "Retry", ExternalID: "ext-1",
	})
	if err != nil {
		t.Fatalf("create retry: %v", err)
	}
	if retry.Outcome != CreateTaskOutcomeFoundSettled {
		t.Fatalf("outcome = %v, want FoundSettled", retry.Outcome)
	}
	if retry.Task.ID != first.Task.ID {
		t.Fatalf("retry task id = %s, want %s", retry.Task.ID, first.Task.ID)
	}
	if retry.Task.ArchivedAt == nil {
		t.Fatal("expected the returned task to remain archived")
	}
}

// TestCreateTaskWithExternalIDAfterDeleteCreatesNewTask pins the "idempotency
// is scoped to the task's lifetime" contract: once the task holding an
// identity is deleted, that identity is free and a subsequent create with
// the same external_id makes a genuinely new task.
func TestCreateTaskWithExternalIDAfterDeleteCreatesNewTask(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	wfID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-deleted")

	first, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-deleted", WorkflowID: wfID, Title: "Task", ExternalID: "ext-1",
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	mustSettle(t, ctx, repo, first.Task.ID, "ext-1")
	if err := svc.DeleteTask(ctx, first.Task.ID); err != nil {
		t.Fatalf("delete task: %v", err)
	}

	second, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-deleted", WorkflowID: wfID, Title: "New task", ExternalID: "ext-1",
	})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	if second.Outcome != CreateTaskOutcomeCreated {
		t.Fatalf("outcome = %v, want Created", second.Outcome)
	}
	if second.Task.ID == first.Task.ID {
		t.Fatal("expected a new task id after the identity-holder was deleted")
	}
}

// TestCreateTaskWithExternalIDAfterRepositoryDeleteReleasesIdentity covers
// the office handoff cascade case: deletion that calls the repository
// directly (bypassing Service.DeleteTask) still frees the identity, because
// the columns are deleted with the row through *any* deletion path.
func TestCreateTaskWithExternalIDAfterRepositoryDeleteReleasesIdentity(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	wfID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-cascade-del")

	first, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-cascade-del", WorkflowID: wfID, Title: "Task", ExternalID: "ext-1",
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	mustSettle(t, ctx, repo, first.Task.ID, "ext-1")

	// Bypass the service entirely, mirroring an office handoff cascade that
	// calls the repository directly.
	if err := repo.DeleteTask(ctx, first.Task.ID); err != nil {
		t.Fatalf("repository DeleteTask: %v", err)
	}

	second, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-cascade-del", WorkflowID: wfID, Title: "New task", ExternalID: "ext-1",
	})
	if err != nil {
		t.Fatalf("create after cascade delete: %v", err)
	}
	if second.Outcome != CreateTaskOutcomeCreated {
		t.Fatalf("outcome = %v, want Created — no code path should leave a stale identity behind", second.Outcome)
	}
}

// TestCreateTaskWithExternalIDTrimPrecedesLookup covers the validation
// scenario: leading/trailing whitespace is trimmed before the (workspace_id,
// external_id) lookup runs, so a padded retry still dedupes.
func TestCreateTaskWithExternalIDTrimPrecedesLookup(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	wfID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-trim")

	first, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-trim", WorkflowID: wfID, Title: "Task", ExternalID: "ext-1",
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}

	retry, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-trim", WorkflowID: wfID, Title: "Retry", ExternalID: "  ext-1  ",
	})
	if err != nil {
		t.Fatalf("create retry: %v", err)
	}
	if retry.Task.ID != first.Task.ID {
		t.Fatalf("retry task id = %s, want %s — trimming must precede the lookup", retry.Task.ID, first.Task.ID)
	}
}

// TestCreateTaskWithExternalIDIsCaseSensitive covers the validation
// scenario: "ext-1" and "EXT-1" are distinct identities.
func TestCreateTaskWithExternalIDIsCaseSensitive(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	wfID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-case")

	lower, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-case", WorkflowID: wfID, Title: "Lower", ExternalID: "ext-1",
	})
	if err != nil {
		t.Fatalf("create lower: %v", err)
	}

	upper, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-case", WorkflowID: wfID, Title: "Upper", ExternalID: "EXT-1",
	})
	if err != nil {
		t.Fatalf("create upper: %v", err)
	}
	if upper.Outcome != CreateTaskOutcomeCreated {
		t.Fatalf("outcome = %v, want Created — case-sensitive identity", upper.Outcome)
	}
	if upper.Task.ID == lower.Task.ID {
		t.Fatal("expected two distinct tasks for ext-1 and EXT-1")
	}
}

// TestNormalizeExternalIDWhitespaceOnlyYieldsAbsent covers the "   " edge
// case: it trims to empty, so the task is created with no identity at all
// (not rejected — only control characters and over-length values are
// invalid).
func TestNormalizeExternalIDWhitespaceOnlyYieldsAbsent(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	wfID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-whitespace")

	result, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-whitespace", WorkflowID: wfID, Title: "Task", ExternalID: "   ",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if result.Task.ExternalID != "" {
		t.Fatalf("task.ExternalID = %q, want empty", result.Task.ExternalID)
	}
}

// TestNormalizeExternalIDTabOnlyIsRejected covers "\t": unlike "   " (plain
// spaces, which trim to absent), a tab is an ASCII control character and
// must be rejected before trimming ever runs.
func TestNormalizeExternalIDTabOnlyIsRejected(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	wfID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-tab")

	_, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-tab", WorkflowID: wfID, Title: "Task", ExternalID: "\t",
	})
	if err == nil {
		t.Fatal("expected a validation error for a bare tab character")
	}
	if len(eventBus.GetPublishedEvents()) != 0 {
		t.Fatal("no task should be created when external_id validation fails")
	}
}
