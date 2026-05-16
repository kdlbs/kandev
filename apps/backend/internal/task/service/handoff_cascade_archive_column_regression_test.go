package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TestArchiveTaskTree_FreshTask_PreservesCascadeColumn is a regression
// net for the office migration dropping tasks.archived_by_cascade_id.
//
// Symptoms (before fix): POST /api/v1/tasks/:id/archive returned 500
// for any freshly-archived task because ArchiveTaskIfActive's CAS
// UPDATE referenced a column the office priority-recreate migration
// had silently dropped from the recreated tasks table.
//
// The repro creates a single task via the real sqlite repository (no
// parent, no children, no workspace-group membership) and calls
// ArchiveTaskTree directly — that is the exact path httpArchiveTask
// takes when a HandoffService is wired. The fix lives in
// internal/office/repository/sqlite/base_migrations.go
// (taskPriorityMigrationStatements) which now carries
// archived_by_cascade_id through the table recreate.
func TestArchiveTaskTree_FreshTask_PreservesCascadeColumn(t *testing.T) {
	svc, repo := setupOfficeTest(t)
	ctx := context.Background()

	// Create a fresh office task with the same path the CLI / UI use.
	// ProjectID resolves the office workflow + start step (mirrors the
	// CLI's `tasks create` payload).
	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Repro target",
		ProjectID:   "proj-1",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// Build the same HandoffService wiring httpArchiveTask uses. We do
	// NOT wire a WorkspaceGroupRepo because the bug report says the
	// task has no workspace-group context yet — passing nil mirrors
	// that condition precisely.
	handoff := NewHandoffService(repo, repo, nil, nil, nil, nil)

	outcome, err := handoff.ArchiveTaskTree(ctx, task.ID)
	if err != nil {
		t.Fatalf("ArchiveTaskTree returned error (this would surface as 500 from httpArchiveTask): %v", err)
	}
	if outcome == nil {
		t.Fatal("expected non-nil outcome")
	}

	foundRoot := false
	for _, id := range outcome.ArchivedTaskIDs {
		if id == task.ID {
			foundRoot = true
			break
		}
	}
	if !foundRoot {
		t.Errorf("ArchivedTaskIDs = %v, want it to contain %s", outcome.ArchivedTaskIDs, task.ID)
	}

	// Confirm the row was actually archived in the DB.
	got, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask after archive: %v", err)
	}
	if got == nil {
		t.Fatal("task disappeared after archive")
	}
	if got.ArchivedAt == nil {
		t.Error("expected archived_at to be set")
	}

	// Sanity-check the cascade-id column is present in the schema and
	// got stamped. The taskSelectColumns projection doesn't include
	// archived_by_cascade_id (it's an internal cascade audit column),
	// so we go straight to the DB. A missing column here is exactly
	// the regression that caused the original 500.
	var cascadeID string
	if err := repo.DB().QueryRowContext(ctx,
		`SELECT COALESCE(archived_by_cascade_id, '') FROM tasks WHERE id = ?`,
		task.ID).Scan(&cascadeID); err != nil {
		t.Fatalf("select archived_by_cascade_id: %v", err)
	}
	if cascadeID == "" {
		t.Error("expected archived_by_cascade_id to be stamped on the row")
	}
	if cascadeID != outcome.CascadeID {
		t.Errorf("archived_by_cascade_id = %q, want %q", cascadeID, outcome.CascadeID)
	}

	_ = models.Task{} // keep models import alive if test body shrinks
}
