package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

// errWorkspaceRepo is a WorkspaceRepository that always returns an error from
// ListWorkspaces. Used to exercise the DB-error path of GetOfficeWorkflowIDs.
type errWorkspaceRepo struct {
	// embed the real repo for all methods except ListWorkspaces.
	WorkspaceRepositoryStub
}

// WorkspaceRepositoryStub satisfies the full WorkspaceRepository interface
// with no-op / panic stubs. Only methods under test need real implementations.
type WorkspaceRepositoryStub struct{}

func (WorkspaceRepositoryStub) CreateWorkspace(_ context.Context, _ *models.Workspace) error {
	panic("not implemented")
}
func (WorkspaceRepositoryStub) GetWorkspace(_ context.Context, _ string) (*models.Workspace, error) {
	panic("not implemented")
}
func (WorkspaceRepositoryStub) UpdateWorkspace(_ context.Context, _ *models.Workspace) error {
	panic("not implemented")
}
func (WorkspaceRepositoryStub) DeleteWorkspace(_ context.Context, _ string) error {
	panic("not implemented")
}
func (WorkspaceRepositoryStub) ListWorkspaces(_ context.Context) ([]*models.Workspace, error) {
	panic("not implemented")
}

func (e errWorkspaceRepo) ListWorkspaces(_ context.Context) ([]*models.Workspace, error) {
	return nil, errors.New("db unavailable")
}

func TestService_GetOfficeWorkflowIDs_Empty(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	// No workspaces → empty result.
	ids := svc.GetOfficeWorkflowIDs(ctx)
	if len(ids) != 0 {
		t.Errorf("expected empty map, got %v", ids)
	}

	// Workspace with no office_workflow_id → still excluded.
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-no-office", Name: "No Office"})
	ids = svc.GetOfficeWorkflowIDs(ctx)
	if len(ids) != 0 {
		t.Errorf("expected empty map for workspace without office_workflow_id, got %v", ids)
	}
}

func TestService_GetOfficeWorkflowIDs_SingleWorkflow(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{
		ID:               "ws-1",
		Name:             "WS 1",
		OfficeWorkflowID: "wf-office-1",
	})

	ids := svc.GetOfficeWorkflowIDs(ctx)
	if _, ok := ids["wf-office-1"]; !ok {
		t.Errorf("expected wf-office-1 in result, got %v", ids)
	}
	if len(ids) != 1 {
		t.Errorf("expected exactly 1 id, got %d", len(ids))
	}
}

func TestService_GetOfficeWorkflowIDs_MultipleWorkflows(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	workspaces := []struct {
		id   string
		wfID string
	}{
		{"ws-a", "wf-office-a"},
		{"ws-b", "wf-office-b"},
		{"ws-c", ""},
	}
	for _, ws := range workspaces {
		_ = repo.CreateWorkspace(ctx, &models.Workspace{
			ID:               ws.id,
			Name:             ws.id,
			OfficeWorkflowID: ws.wfID,
		})
	}

	ids := svc.GetOfficeWorkflowIDs(ctx)
	if _, ok := ids["wf-office-a"]; !ok {
		t.Errorf("expected wf-office-a")
	}
	if _, ok := ids["wf-office-b"]; !ok {
		t.Errorf("expected wf-office-b")
	}
	if len(ids) != 2 {
		t.Errorf("expected 2 ids (ws-c has no office wf), got %d: %v", len(ids), ids)
	}
}

func TestService_GetOfficeWorkflowIDs_DBError(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	// Seed a workspace first so we know the real repo would return something.
	_ = repo.CreateWorkspace(ctx, &models.Workspace{
		ID:               "ws-ok",
		Name:             "OK",
		OfficeWorkflowID: "wf-ok",
	})

	// Replace the workspace repo with one that always errors.
	svc.workspaces = errWorkspaceRepo{}

	ids := svc.GetOfficeWorkflowIDs(ctx)
	if ids != nil {
		t.Errorf("expected nil on DB error, got %v", ids)
	}
}

func TestService_DeleteWorkspaceDeletesWorkspaceOwnedTasksAndWorkflows(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-delete", Name: "Delete Me"})
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-keep", Name: "Keep Me"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-delete", WorkspaceID: "ws-delete", Name: "Doomed"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-keep", WorkspaceID: "ws-keep", Name: "Keep"})
	if err := repo.CreateTask(ctx, &models.Task{
		ID:             "task-delete",
		WorkspaceID:    "ws-delete",
		WorkflowID:     "wf-delete",
		WorkflowStepID: "step-delete",
		Title:          "Delete task",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	if err := svc.DeleteWorkspace(ctx, "ws-delete"); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}

	if _, err := repo.GetWorkspace(ctx, "ws-delete"); err == nil {
		t.Fatalf("workspace should be deleted")
	}
	if _, err := repo.GetTask(ctx, "task-delete"); err == nil {
		t.Fatalf("workspace task should be deleted")
	}
	workflows, err := repo.ListWorkflows(ctx, "ws-delete", true)
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	if len(workflows) != 0 {
		t.Fatalf("workspace workflows should be deleted, got %d", len(workflows))
	}
	if _, err := repo.GetWorkflow(ctx, "wf-keep"); err != nil {
		t.Fatalf("unrelated workflow should remain: %v", err)
	}
}

// TestService_DeleteWorkflow_ArchivesChildTasks verifies the cascade fix for
// issue #1279: workflow deletion archives any active child tasks instead of
// leaving them with a dangling workflow_id (tasks.workflow_id has no FK, so
// SQLite cannot CASCADE for us).
func TestService_DeleteWorkflow_ArchivesChildTasks(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-doomed", WorkspaceID: "ws-1", Name: "Doomed"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-keep", WorkspaceID: "ws-1", Name: "Keep"})

	tasks := []*models.Task{
		{ID: "task-a", WorkspaceID: "ws-1", WorkflowID: "wf-doomed", WorkflowStepID: "step-1", Title: "A"},
		{ID: "task-b", WorkspaceID: "ws-1", WorkflowID: "wf-doomed", WorkflowStepID: "step-1", Title: "B"},
		{ID: "task-other", WorkspaceID: "ws-1", WorkflowID: "wf-keep", WorkflowStepID: "step-1", Title: "Other"},
	}
	for _, task := range tasks {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask %s: %v", task.ID, err)
		}
	}

	if err := svc.DeleteWorkflow(ctx, "wf-doomed"); err != nil {
		t.Fatalf("DeleteWorkflow: %v", err)
	}

	if _, err := svc.workflows.GetWorkflow(ctx, "wf-doomed"); err == nil {
		t.Fatalf("expected workflow to be deleted")
	}

	for _, id := range []string{"task-a", "task-b"} {
		got, err := repo.GetTask(ctx, id)
		if err != nil {
			t.Fatalf("GetTask %s after cascade: %v", id, err)
		}
		if got.ArchivedAt == nil {
			t.Errorf("task %s: expected archived_at to be set, got nil", id)
		}
	}

	other, err := repo.GetTask(ctx, "task-other")
	if err != nil {
		t.Fatalf("GetTask task-other: %v", err)
	}
	if other.ArchivedAt != nil {
		t.Errorf("task in unrelated workflow should not be archived, got archived_at=%v", other.ArchivedAt)
	}
}

// leakyListTaskRepo wraps the real TaskRepository and injects extra tasks
// into ListTasks results, simulating a TOCTOU race where a task is archived
// between the snapshot and the cascade loop.
type leakyListTaskRepo struct {
	repository.TaskRepository
	extra []*models.Task
}

func (l leakyListTaskRepo) ListTasks(ctx context.Context, workflowID string) ([]*models.Task, error) {
	real, err := l.TaskRepository.ListTasks(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	return append(real, l.extra...), nil
}

// TestService_DeleteWorkflow_SkipsConcurrentlyArchivedTask covers the
// TOCTOU race window between Service.tasks.ListTasks and Service.ArchiveTask:
// if a task is archived by another caller in that window, ArchiveTask
// returns ErrTaskAlreadyArchived and the cascade must continue rather than
// abort, so the workflow row still gets deleted.
func TestService_DeleteWorkflow_SkipsConcurrentlyArchivedTask(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-doomed", WorkspaceID: "ws-1", Name: "Doomed"})

	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-live", WorkspaceID: "ws-1", WorkflowID: "wf-doomed", WorkflowStepID: "step-1", Title: "Live",
	}); err != nil {
		t.Fatalf("CreateTask live: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-raced", WorkspaceID: "ws-1", WorkflowID: "wf-doomed", WorkflowStepID: "step-1", Title: "Raced",
	}); err != nil {
		t.Fatalf("CreateTask raced: %v", err)
	}
	if err := repo.ArchiveTask(ctx, "task-raced"); err != nil {
		t.Fatalf("pre-archive raced task: %v", err)
	}

	raced, err := repo.GetTask(ctx, "task-raced")
	if err != nil {
		t.Fatalf("GetTask raced: %v", err)
	}
	svc.tasks = leakyListTaskRepo{TaskRepository: repo, extra: []*models.Task{raced}}

	if err := svc.DeleteWorkflow(ctx, "wf-doomed"); err != nil {
		t.Fatalf("DeleteWorkflow should swallow ErrTaskAlreadyArchived: %v", err)
	}

	if _, err := svc.workflows.GetWorkflow(ctx, "wf-doomed"); err == nil {
		t.Fatalf("expected workflow to be deleted despite race")
	}

	got, err := repo.GetTask(ctx, "task-live")
	if err != nil {
		t.Fatalf("GetTask live: %v", err)
	}
	if got.ArchivedAt == nil {
		t.Errorf("live task should be archived by cascade")
	}
}

// TestService_DeleteWorkflow_PartialArchiveErrorPreservesWorkflow verifies
// the fail-fast contract: when ArchiveTask returns a non-sentinel error
// part-way through the cascade, DeleteWorkflow surfaces it, leaves the
// workflow row intact, and the tasks archived before the failure stay
// archived. Retries are safe because ListTasks filters them out.
func TestService_DeleteWorkflow_PartialArchiveErrorPreservesWorkflow(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-doomed", WorkspaceID: "ws-1", Name: "Doomed"})

	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-first", WorkspaceID: "ws-1", WorkflowID: "wf-doomed", WorkflowStepID: "step-1", Title: "First",
	}); err != nil {
		t.Fatalf("CreateTask first: %v", err)
	}
	// "task-ghost" never actually exists in the DB — the leaky list returns
	// it so the cascade's ArchiveTask call hits a real GetTask error.
	ghost := &models.Task{ID: "task-ghost", WorkspaceID: "ws-1", WorkflowID: "wf-doomed", WorkflowStepID: "step-1", Title: "Ghost"}
	svc.tasks = leakyListTaskRepo{TaskRepository: repo, extra: []*models.Task{ghost}}

	err := svc.DeleteWorkflow(ctx, "wf-doomed")
	if err == nil {
		t.Fatalf("expected error when ArchiveTask fails mid-cascade")
	}
	if errors.Is(err, ErrTaskAlreadyArchived) {
		t.Fatalf("non-sentinel error must propagate, got sentinel: %v", err)
	}

	if _, err := svc.workflows.GetWorkflow(ctx, "wf-doomed"); err != nil {
		t.Fatalf("workflow row must survive a partial cascade, got: %v", err)
	}
	first, err := repo.GetTask(ctx, "task-first")
	if err != nil {
		t.Fatalf("GetTask first: %v", err)
	}
	if first.ArchivedAt == nil {
		t.Errorf("task archived before the failure should remain archived")
	}
}

// TestService_ArchiveTask_ReturnsAlreadyArchivedSentinel locks in the
// sentinel-error contract DeleteWorkflow relies on.
func TestService_ArchiveTask_ReturnsAlreadyArchivedSentinel(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "T",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	if err := svc.ArchiveTask(ctx, "task-1"); err != nil {
		t.Fatalf("first ArchiveTask: %v", err)
	}
	err := svc.ArchiveTask(ctx, "task-1")
	if !errors.Is(err, ErrTaskAlreadyArchived) {
		t.Fatalf("second ArchiveTask: want ErrTaskAlreadyArchived, got %v", err)
	}
}

// TestApplyRepositoryUpdates_CopyFilesNilLeavesUntouched verifies the
// pointer-nil convention: a nil CopyFiles field on the request must not
// clobber an existing repository value.
func TestApplyRepositoryUpdates_CopyFilesNilLeavesUntouched(t *testing.T) {
	repo := &models.Repository{CopyFiles: "existing"}
	if err := applyRepositoryUpdates(repo, &UpdateRepositoryRequest{}); err != nil {
		t.Fatalf("applyRepositoryUpdates: %v", err)
	}
	if repo.CopyFiles != "existing" {
		t.Errorf("CopyFiles = %q, want %q (nil request field must not overwrite)", repo.CopyFiles, "existing")
	}
}

// TestApplyRepositoryUpdates_CopyFilesEmptyStringClears verifies that an
// explicit empty-string pointer clears the value (distinct from "no update").
func TestApplyRepositoryUpdates_CopyFilesEmptyStringClears(t *testing.T) {
	repo := &models.Repository{CopyFiles: "existing"}
	empty := ""
	if err := applyRepositoryUpdates(repo, &UpdateRepositoryRequest{CopyFiles: &empty}); err != nil {
		t.Fatalf("applyRepositoryUpdates: %v", err)
	}
	if repo.CopyFiles != "" {
		t.Errorf("CopyFiles = %q, want empty string", repo.CopyFiles)
	}
}
