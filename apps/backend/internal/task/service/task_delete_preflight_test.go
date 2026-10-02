package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/worktree"
	"github.com/kandev/kandev/pkg/api/v1"
)

type taskDeletePreflightCleanup struct {
	worktreesByTaskID map[string][]*worktree.Worktree
	dirty             []worktree.DirtyWorktree
	inspectErr        error
	inventoryErr      error
	inspected         []*worktree.Worktree
	inspectCalls      int
}

func taskDeletePreflightHumanContext() context.Context {
	return authn.WithIdentity(context.Background(), authn.Identity{UserID: "test-human", Synthetic: true})
}

func (c *taskDeletePreflightCleanup) OnTaskDeleted(context.Context, string) error { return nil }

func (c *taskDeletePreflightCleanup) GetAllByTaskID(
	_ context.Context, taskID string,
) ([]*worktree.Worktree, error) {
	if c.inventoryErr != nil {
		return nil, c.inventoryErr
	}
	return c.worktreesByTaskID[taskID], nil
}

func (c *taskDeletePreflightCleanup) InspectDirtyWorktrees(
	_ context.Context, worktrees []*worktree.Worktree,
) ([]worktree.DirtyWorktree, error) {
	c.inspectCalls++
	c.inspected = append(c.inspected, worktrees...)
	if c.inspectErr != nil {
		return nil, c.inspectErr
	}
	allowed := make(map[string]struct{}, len(worktrees))
	for _, item := range worktrees {
		if item != nil {
			allowed[item.ID] = struct{}{}
		}
	}
	dirty := make([]worktree.DirtyWorktree, 0, len(c.dirty))
	for _, item := range c.dirty {
		if _, ok := allowed[item.WorktreeID]; ok {
			dirty = append(dirty, item)
		}
	}
	return dirty, nil
}

type taskDeletePreflightWithoutInspector struct{}

func (taskDeletePreflightWithoutInspector) OnTaskDeleted(context.Context, string) error { return nil }

func (taskDeletePreflightWithoutInspector) GetAllByTaskID(
	context.Context, string,
) ([]*worktree.Worktree, error) {
	return nil, nil
}

func seedTaskDeletePreflightTask(t *testing.T, repo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateWorkflow(context.Context, *models.Workflow) error
	CreateTask(context.Context, *models.Task) error
}, taskID, parentID string, archived bool) {
	t.Helper()
	ctx := context.Background()
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-preflight", Name: "Preflight"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-preflight", WorkspaceID: "ws-preflight", Name: "Workflow"})
	var archivedAt *time.Time
	if archived {
		now := time.Now().UTC()
		archivedAt = &now
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID:          taskID,
		WorkspaceID: "ws-preflight",
		WorkflowID:  "wf-preflight",
		Title:       taskID,
		State:       v1.TaskStateCreated,
		Priority:    "medium",
		ParentID:    parentID,
		ArchivedAt:  archivedAt,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create task %s: %v", taskID, err)
	}
}

func TestTaskDeletePreflightUsesExactScopeAndReportsDirtyWorktrees(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	seedTaskDeletePreflightTask(t, repo, "root", "", false)
	seedTaskDeletePreflightTask(t, repo, "child", "root", false)
	seedTaskDeletePreflightTask(t, repo, "archived-child", "root", true)
	cleanup := &taskDeletePreflightCleanup{
		worktreesByTaskID: map[string][]*worktree.Worktree{
			"root":           {{ID: "wt-root", TaskID: "root", SessionID: ""}},
			"child":          {{ID: "wt-child", TaskID: "child"}},
			"archived-child": {{ID: "wt-archived", TaskID: "archived-child"}},
		},
		dirty: []worktree.DirtyWorktree{{WorktreeID: "wt-child"}},
	}
	svc.SetWorktreeCleanup(cleanup)

	clean, err := svc.TaskDeletePreflight(taskDeletePreflightHumanContext(), []string{"root"}, false)
	if err != nil {
		t.Fatalf("direct preflight: %v", err)
	}
	if clean.RequiresDiscardConsent {
		t.Fatal("direct preflight included a child worktree")
	}

	dirty, err := svc.TaskDeletePreflight(taskDeletePreflightHumanContext(), []string{"root", "child", "root"}, true)
	if err != nil {
		t.Fatalf("cascade preflight: %v", err)
	}
	if !dirty.RequiresDiscardConsent {
		t.Fatal("cascade preflight did not report dirty descendant")
	}
	if len(cleanup.inspected) != 4 {
		t.Fatalf("inspected worktrees = %d, want 4 across exact direct and deduplicated cascade scopes", len(cleanup.inspected))
	}
	if events := eventBus.GetPublishedEvents(); len(events) != 0 {
		t.Fatalf("preflight published events: %#v", events)
	}
	if _, err := repo.GetTask(context.Background(), "root"); err != nil {
		t.Fatalf("root changed during preflight: %v", err)
	}
}

func TestTaskDeletePreflightTreatsEmptyInventoryAsClean(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedTaskDeletePreflightTask(t, repo, "empty", "", false)
	cleanup := &taskDeletePreflightCleanup{worktreesByTaskID: map[string][]*worktree.Worktree{}}
	svc.SetWorktreeCleanup(cleanup)

	result, err := svc.TaskDeletePreflight(taskDeletePreflightHumanContext(), []string{"empty"}, false)
	if err != nil {
		t.Fatalf("empty preflight: %v", err)
	}
	if result.RequiresDiscardConsent {
		t.Fatal("empty inventory requires discard consent")
	}
	if cleanup.inspectCalls != 1 {
		t.Fatal("empty inventory did not confirm the inspection capability")
	}
}

func TestTaskDeleteConfirmationBindsHumanOptionsAndCurrentTree(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedTaskDeletePreflightTask(t, repo, "confirmed-root", "", false)
	seedTaskDeletePreflightTask(t, repo, "confirmed-child", "confirmed-root", false)
	svc.SetWorktreeCleanup(&taskDeletePreflightCleanup{worktreesByTaskID: map[string][]*worktree.Worktree{}})

	ctx := taskDeletePreflightHumanContext()
	preview, err := svc.TaskDeletePreflight(ctx, []string{"confirmed-root"}, false, false)
	if err != nil || preview.ConfirmationID == "" {
		t.Fatalf("TaskDeletePreflight = %+v, %v; want a confirmation id", preview, err)
	}
	if err := svc.WithTaskDeleteConfirmation(ctx, preview.ConfirmationID, "confirmed-root", false, true, func() error { return nil }); !errors.Is(err, ErrTaskDeleteConfirmationMismatch) {
		t.Fatalf("confirmation with changed discard option = %v, want mismatch", err)
	}
	otherHuman := authn.WithIdentity(context.Background(), authn.Identity{UserID: "other-human", Synthetic: true})
	if err := svc.WithTaskDeleteConfirmation(otherHuman, preview.ConfirmationID, "confirmed-root", false, false, func() error { return nil }); !errors.Is(err, ErrTaskDeleteConfirmationMismatch) {
		t.Fatalf("confirmation from another Human = %v, want mismatch", err)
	}
	if _, err := repo.DB().ExecContext(context.Background(), "UPDATE tasks SET updated_at = ? WHERE id = ?", time.Now().UTC().Add(time.Second), "confirmed-child"); err != nil {
		t.Fatalf("change child after preview: %v", err)
	}
	if err := svc.WithTaskDeleteConfirmation(ctx, preview.ConfirmationID, "confirmed-root", false, false, func() error { return nil }); !errors.Is(err, ErrTaskDeleteConfirmationStale) {
		t.Fatalf("confirmation after tree change = %v, want stale", err)
	}

	fresh, err := svc.TaskDeletePreflight(ctx, []string{"confirmed-root"}, false, false)
	if err != nil {
		t.Fatalf("fresh TaskDeletePreflight: %v", err)
	}
	deletes := 0
	deleteRoot := func() error { deletes++; return nil }
	if err := svc.WithTaskDeleteConfirmation(ctx, fresh.ConfirmationID, "confirmed-root", false, false, deleteRoot); err != nil {
		t.Fatalf("confirmed delete: %v", err)
	}
	if err := svc.WithTaskDeleteConfirmation(ctx, fresh.ConfirmationID, "confirmed-root", false, false, deleteRoot); !errors.Is(err, ErrTaskDeleteConfirmationReplay) {
		t.Fatalf("replayed confirmation = %v, want replay rejection", err)
	}
	if deletes != 1 {
		t.Fatalf("delete callback calls = %d, want one", deletes)
	}
}

func TestTaskDeletePreflightFailsClosedWhenInspectionIsUnavailable(t *testing.T) {
	tests := []struct {
		name    string
		cleanup WorktreeCleanup
		wantErr error
	}{
		{name: "missing cleanup", wantErr: ErrTaskDeletePreflightUnavailable},
		{name: "missing inspector", cleanup: taskDeletePreflightWithoutInspector{}, wantErr: ErrTaskDeletePreflightUnavailable},
		{name: "inventory error", cleanup: &taskDeletePreflightCleanup{inventoryErr: errors.New("inventory failed")}, wantErr: ErrTaskDeletePreflightUnavailable},
		{name: "inspection error", cleanup: &taskDeletePreflightCleanup{inspectErr: errors.New("inspection failed")}, wantErr: ErrTaskDeletePreflightUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			seedTaskDeletePreflightTask(t, repo, tt.name, "", false)
			if tt.cleanup != nil {
				svc.SetWorktreeCleanup(tt.cleanup)
			}
			_, err := svc.TaskDeletePreflight(taskDeletePreflightHumanContext(), []string{tt.name}, false)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestTaskDeletePreflightRejectsEmptySelection(t *testing.T) {
	svc, _, _ := createTestService(t)
	_, err := svc.TaskDeletePreflight(context.Background(), []string{" ", ""}, false)
	if !errors.Is(err, ErrTaskDeletePreflightInvalid) {
		t.Fatalf("error = %v, want invalid preflight", err)
	}
}

// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.6
// @covers AC-TASKS-RUNTIME-CLEANUP-001.9
func TestTaskDeletePreflightPreservesInspectionCause(t *testing.T) {
	for _, cause := range []error{errors.New("private Git failure"), context.Canceled, context.DeadlineExceeded} {
		t.Run(cause.Error(), func(t *testing.T) {
			svc, eventBus, repo := createTestService(t)
			seedTaskDeletePreflightTask(t, repo, "inspection", "", false)
			cleanup := &taskDeletePreflightCleanup{worktreesByTaskID: map[string][]*worktree.Worktree{
				"inspection": {{ID: "healthy"}, {ID: "broken"}},
			}}
			svc.SetWorktreeCleanup(cleanup)
			ctx := taskDeletePreflightHumanContext()
			control, err := svc.TaskDeletePreflight(ctx, []string{"inspection"}, false)
			if err != nil || control.ConfirmationID == "" {
				t.Fatalf("healthy control = %+v, %v; want a confirmation", control, err)
			}
			before, err := repo.GetTask(ctx, "inspection")
			if err != nil {
				t.Fatal(err)
			}
			inspection := &worktree.CleanupInspectionError{
				Stage: "working_tree_status", Reason: worktree.CleanupInspectionReasonRepoUnavailable, Err: cause,
			}
			cleanup.inspectErr = inspection
			result, err := svc.TaskDeletePreflight(ctx, []string{"inspection"}, false)
			var got *worktree.CleanupInspectionError
			if !errors.Is(err, ErrTaskDeletePreflightUnavailable) || !errors.As(err, &got) || got != inspection || !errors.Is(err, cause) {
				t.Fatalf("preflight error = %v; want unavailable sentinel and original typed cause", err)
			}
			if result != (TaskDeletePreflightResult{}) {
				t.Fatalf("failed preflight issued a result: %+v", result)
			}
			after, readErr := repo.GetTask(ctx, "inspection")
			if readErr != nil || after.State != before.State || !after.UpdatedAt.Equal(before.UpdatedAt) {
				t.Fatalf("preflight changed task: %+v, %v", after, readErr)
			}
			if len(eventBus.GetPublishedEvents()) != 0 {
				t.Fatal("preflight published task mutations")
			}
		})
	}
}

func TestTaskDeletePreflightAuthorizesEveryRootBeforeInspection(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedTaskDeletePreflightTask(t, repo, "foreign", "", false)
	if _, err := repo.DB().ExecContext(context.Background(),
		"UPDATE workspaces SET owner_id = ? WHERE id = ?", "user-b", "ws-preflight"); err != nil {
		t.Fatalf("update workspace owner: %v", err)
	}
	cleanup := &taskDeletePreflightCleanup{
		worktreesByTaskID: map[string][]*worktree.Worktree{
			"foreign": {{ID: "wt-foreign", TaskID: "foreign"}},
		},
	}
	svc.SetWorktreeCleanup(cleanup)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{
		UserID: "user-a",
		Role:   authn.RoleMember,
	})

	_, err := svc.TaskDeletePreflight(ctx, []string{"foreign"}, false)
	if !errors.Is(err, taskrepo.ErrTaskNotFound) {
		t.Fatalf("error = %v, want task not found for an unreachable workspace", err)
	}
	if cleanup.inspectCalls != 0 {
		t.Fatalf("inspections = %d, want none after authorization failure", cleanup.inspectCalls)
	}
}
