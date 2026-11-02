package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TestIsFromOfficeProjection_RealWorkspaceWorkflow covers the SELECT-time
// `isFromOfficeProjection` SQL — the COALESCE(project_id) || EXISTS(...
// office_workflow_id join) expression that every task SELECT carries.
//
// The check matters because:
//
//   - Wrong column reference, broken JOIN, or a stale CASE branch would
//     silently mislabel every task. The frontend would either hide the
//     office view link on real office tasks (false negative on
//     office-workflow tasks) or expose it on kanban tasks (false positive
//     via the project-linked branch).
//   - The model-level TestTaskIsFromOfficeField only exercises Go-side
//     field assignment, which would still pass under a broken projection.
//
// Three rows cover the three branches of the projection:
//
//  1. Office-workflow task: workflow_id == workspace.office_workflow_id,
//     no project. Must return true via the EXISTS branch.
//  2. Project-linked task: non-empty project_id, workflow_id is some
//     unrelated kanban workflow. Must return true via the COALESCE branch.
//  3. Kanban task: workflow_id is some unrelated kanban workflow,
//     project_id is empty. Must return false (both branches fail).
//
// See apps/backend/internal/task/repository/sqlite/task.go isFromOfficeProjection.
func TestIsFromOfficeProjection_RealWorkspaceWorkflow(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()

	const workspaceID = "ws-office-detect"
	if err := repo.CreateWorkspace(ctx, &models.Workspace{
		ID:      workspaceID,
		Name:    "office-detect",
		OwnerID: "u-1",
	}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	// Materialise the office workflow on the workspace. Stamps
	// workspaces.office_workflow_id, which is the join key the projection's
	// EXISTS subquery hangs off of.
	officeWorkflowID, err := repo.EnsureOfficeWorkflow(ctx, workspaceID)
	if err != nil {
		t.Fatalf("EnsureOfficeWorkflow: %v", err)
	}
	if officeWorkflowID == "" {
		t.Fatalf("EnsureOfficeWorkflow returned empty workflow id")
	}

	// Some kanban workflow id that is NOT the office workflow. We don't
	// have to insert a workflow row for this test — the projection only
	// joins to `workspaces`, not to `workflows`, so the bare ID is enough
	// to exercise the false-negative branch.
	const kanbanWorkflowID = "wf-kanban-not-office"

	tasks := []struct {
		id          string
		workflowID  string
		projectID   string
		wantOffice  bool
		description string
	}{
		{
			id:          "t-office-via-workflow",
			workflowID:  officeWorkflowID,
			projectID:   "",
			wantOffice:  true,
			description: "office-workflow task → is_from_office = true via EXISTS branch",
		},
		{
			id:          "t-office-via-project",
			workflowID:  kanbanWorkflowID,
			projectID:   "proj-1",
			wantOffice:  true,
			description: "project-linked task → is_from_office = true via COALESCE(project_id) branch",
		},
		{
			id:          "t-kanban",
			workflowID:  kanbanWorkflowID,
			projectID:   "",
			wantOffice:  false,
			description: "kanban task → is_from_office = false (both branches miss)",
		},
	}

	for _, tc := range tasks {
		if err := repo.CreateTask(ctx, &models.Task{
			ID:          tc.id,
			WorkspaceID: workspaceID,
			WorkflowID:  tc.workflowID,
			Title:       tc.id,
			State:       "BACKLOG",
			ProjectID:   tc.projectID,
		}); err != nil {
			t.Fatalf("CreateTask %q: %v", tc.id, err)
		}
	}

	for _, tc := range tasks {
		t.Run(tc.id, func(t *testing.T) {
			got, err := repo.GetTask(ctx, tc.id)
			if err != nil {
				t.Fatalf("GetTask: %v", err)
			}
			if got.IsFromOffice != tc.wantOffice {
				t.Errorf("%s\n  IsFromOffice = %v, want %v\n  (workflow_id=%q, project_id=%q, office_workflow_id=%q)",
					tc.description, got.IsFromOffice, tc.wantOffice,
					tc.workflowID, tc.projectID, officeWorkflowID)
			}
		})
	}
}

func TestIsFromOfficeProjection_SystemRoutineWorkspace(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()
	for _, id := range []string{"office", "other-office", "kanban"} {
		if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: id, Name: id}); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"office", "other-office"} {
		if _, err := repo.EnsureOfficeWorkflow(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	routine, err := repo.EnsureRoutineWorkflow(ctx, "office")
	if err != nil {
		t.Fatal(err)
	}
	other, err := repo.EnsureRoutineWorkflow(ctx, "other-office")
	if err != nil {
		t.Fatal(err)
	}
	kanban, err := repo.EnsureRoutineWorkflow(ctx, "kanban")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id, workspace, workflow string
		office                  bool
	}{
		{"owned-routine", "office", routine, true},
		{"foreign-routine", "office", other, false},
		{"non-office-routine", "kanban", kanban, false},
	} {
		t.Run(tc.id, func(t *testing.T) {
			if err := repo.CreateTask(ctx, &models.Task{ID: tc.id, WorkspaceID: tc.workspace, WorkflowID: tc.workflow, Title: tc.id, State: "TODO"}); err != nil {
				t.Fatal(err)
			}
			got, err := repo.GetTask(ctx, tc.id)
			if err != nil {
				t.Fatal(err)
			}
			if got.IsFromOffice != tc.office {
				t.Fatalf("Office identity = %v, want %v", got.IsFromOffice, tc.office)
			}
		})
	}
	// A user workflow cannot acquire Office identity by copying the template name.
	_, err = repo.db.ExecContext(ctx, `UPDATE workflows SET is_system=0 WHERE id=?`, routine)
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetTask(ctx, "owned-routine")
	if err != nil {
		t.Fatal(err)
	}
	if got.IsFromOffice {
		t.Fatal("a user-owned routine-template workflow must remain Kanban")
	}
}
