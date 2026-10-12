package service

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type visibilityReceiptRepo struct {
	*fakeCascadeRepo
	receipts map[string]*models.TaskLifecycleReceipt
}

func (r *visibilityReceiptRepo) GetTaskLifecycleReceipt(_ context.Context, workspaceID, taskID string) (*models.TaskLifecycleReceipt, error) {
	receipt := r.receipts[taskID]
	if receipt == nil || receipt.WorkspaceID != workspaceID {
		return nil, nil
	}
	copy := *receipt
	return &copy, nil
}

func (r *visibilityReceiptRepo) GetTaskVisibilitySnapshot(ctx context.Context, workspaceID, taskID string) (*models.Task, *models.TaskLifecycleReceipt, error) {
	task, err := r.GetTask(ctx, taskID)
	if errors.Is(err, repository.ErrTaskNotFound) {
		task = nil
		err = nil
	}
	if err != nil {
		return nil, nil, err
	}
	receipt, err := r.GetTaskLifecycleReceipt(ctx, workspaceID, taskID)
	return task, receipt, err
}

func TestGetTaskVisibilityUsesLiveProjectionAndDoesNotMutateReads(t *testing.T) {
	ctx := context.Background()
	tasks := newFakeTaskRepo()
	tasks.addTask("caller", "", "workspace-a")
	tasks.addTask("target", "", "workspace-a")
	tasks.mu.Lock()
	target := tasks.tasks["target"]
	target.Identifier = "LIVE-2"
	target.Title = "Live title"
	target.WorkflowID = "workflow-live"
	target.WorkflowStepID = "step-live"
	target.State = "DONE"
	tasks.mu.Unlock()
	repo := &visibilityReceiptRepo{fakeCascadeRepo: newCascadeRepo(tasks), receipts: map[string]*models.TaskLifecycleReceipt{
		"target": {TaskID: "target", WorkspaceID: "workspace-a", Identifier: "OLD-1", Title: "Old title", WorkflowID: "workflow-old", WorkflowStepID: "step-old", State: "TODO", Disposition: "active", ChangedAt: time.Now().UTC(), Revision: 4, Provenance: "move"},
	}}
	svc := NewHandoffService(repo, nil, nil, nil, nil, nil)

	tasks.mu.Lock()
	before := *tasks.tasks["target"]
	tasks.mu.Unlock()
	visibility, err := svc.GetTaskVisibilityForRequest(ctx, RelatedReadRequest{CallerTaskID: "caller", TargetTaskID: "target", Scope: RelatedReadScopeWorkspaceTaskTree})
	require.NoError(t, err)
	require.Equal(t, "LIVE-2", visibility.Identity.Identifier)
	require.Equal(t, "Live title", visibility.Identity.Title)
	require.Equal(t, "workflow-live", visibility.WorkflowID)
	require.Equal(t, "step-live", visibility.WorkflowStepID)
	require.Equal(t, "DONE", visibility.State)
	require.EqualValues(t, 4, visibility.Visibility.Revision)
	tasks.mu.Lock()
	after := *tasks.tasks["target"]
	tasks.mu.Unlock()
	require.True(t, reflect.DeepEqual(before, after), "visibility read must not mutate the task")
}

func TestGetTaskVisibilityRequiresDeletedTombstoneForAbsentTask(t *testing.T) {
	ctx := context.Background()
	tasks := newFakeTaskRepo()
	tasks.addTask("caller", "", "workspace-a")
	repo := &visibilityReceiptRepo{fakeCascadeRepo: newCascadeRepo(tasks), receipts: map[string]*models.TaskLifecycleReceipt{
		"deleted": {TaskID: "deleted", WorkspaceID: "workspace-a", Title: "Deleted", Disposition: "deleted", ChangedAt: time.Now().UTC(), Revision: 2, Provenance: "delete"},
		"active":  {TaskID: "active", WorkspaceID: "workspace-a", Title: "Stale", Disposition: "active", ChangedAt: time.Now().UTC(), Revision: 1, Provenance: "create"},
	}}
	svc := NewHandoffService(repo, nil, nil, nil, nil, nil)

	deleted, err := svc.GetTaskVisibilityForRequest(ctx, RelatedReadRequest{CallerTaskID: "caller", TargetTaskID: "deleted", Scope: RelatedReadScopeWorkspaceTaskTree})
	require.NoError(t, err)
	require.Equal(t, "deleted", deleted.Visibility.Disposition)
	for _, target := range []string{"active", "missing"} {
		_, err := svc.GetTaskVisibilityForRequest(ctx, RelatedReadRequest{CallerTaskID: "caller", TargetTaskID: target, Scope: RelatedReadScopeWorkspaceTaskTree})
		require.ErrorIs(t, err, ErrAccessDenied)
		reason, ok := RelatedReadDenialReasonFor(err)
		require.True(t, ok)
		require.Equal(t, RelatedReadDenialTargetUnavailable, reason)
	}
}

type deleteAfterVisibilitySnapshotRepo struct {
	*sqlite.Repository
	deleteTargetID string
}

func (r *deleteAfterVisibilitySnapshotRepo) GetTaskVisibilitySnapshot(ctx context.Context, workspaceID, taskID string) (*models.Task, *models.TaskLifecycleReceipt, error) {
	task, receipt, err := r.Repository.GetTaskVisibilitySnapshot(ctx, workspaceID, taskID)
	if err != nil || taskID != r.deleteTargetID {
		return task, receipt, err
	}
	deleted := make(chan error, 1)
	go func() { deleted <- r.DeleteTask(ctx, taskID) }()
	if err := <-deleted; err != nil {
		return nil, nil, err
	}
	return task, receipt, nil
}

func TestGetTaskVisibilitySQLiteSnapshotDoesNotMutateAndSurvivesConcurrentDelete(t *testing.T) {
	ctx := context.Background()
	repo, writer := newVisibilitySQLiteRepo(t)
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "visibility-snapshot-workspace", Name: "Visibility snapshot"}))
	caller := &models.Task{ID: "visibility-snapshot-caller", WorkspaceID: "visibility-snapshot-workspace", Title: "Caller"}
	target := &models.Task{ID: "visibility-snapshot-target", WorkspaceID: "visibility-snapshot-workspace", Title: "Before delete", Identifier: "VIS-2"}
	require.NoError(t, repo.CreateTask(ctx, caller))
	require.NoError(t, repo.CreateTask(ctx, target))

	before := visibilityPersistedState(t, writer)
	service := NewHandoffService(repo, nil, nil, nil, nil, nil)
	visibility, err := service.GetTaskVisibilityForRequest(ctx, RelatedReadRequest{CallerTaskID: caller.ID, TargetTaskID: target.ID, Scope: RelatedReadScopeWorkspaceTaskTree})
	require.NoError(t, err)
	require.Equal(t, "Before delete", visibility.Identity.Title)
	require.Equal(t, "active", visibility.Visibility.Disposition)
	require.EqualValues(t, 1, visibility.Visibility.Revision)
	require.Equal(t, before, visibilityPersistedState(t, writer), "a visibility read must not mutate task, receipt, session, or document rows")

	concurrentService := NewHandoffService(&deleteAfterVisibilitySnapshotRepo{Repository: repo, deleteTargetID: target.ID}, nil, nil, nil, nil, nil)
	beforeDelete, err := concurrentService.GetTaskVisibilityForRequest(ctx, RelatedReadRequest{CallerTaskID: caller.ID, TargetTaskID: target.ID, Scope: RelatedReadScopeWorkspaceTaskTree})
	require.NoError(t, err)
	require.Equal(t, "Before delete", beforeDelete.Identity.Title)
	require.Equal(t, "active", beforeDelete.Visibility.Disposition)
	require.Equal(t, "create", beforeDelete.Visibility.Provenance)
	require.EqualValues(t, 1, beforeDelete.Visibility.Revision)

	afterDelete, err := service.GetTaskVisibilityForRequest(ctx, RelatedReadRequest{CallerTaskID: caller.ID, TargetTaskID: target.ID, Scope: RelatedReadScopeWorkspaceTaskTree})
	require.NoError(t, err)
	require.Equal(t, "deleted", afterDelete.Visibility.Disposition)
	require.Equal(t, "delete", afterDelete.Visibility.Provenance)
	require.EqualValues(t, 2, afterDelete.Visibility.Revision)
}

func newVisibilitySQLiteRepo(t *testing.T) (*sqlite.Repository, *sqlx.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "visibility.db")
	writerConn, err := internaldb.OpenSQLite(path)
	require.NoError(t, err)
	readerConn, err := internaldb.OpenSQLiteReader(path)
	require.NoError(t, err)
	writer := sqlx.NewDb(writerConn, "sqlite3")
	reader := sqlx.NewDb(readerConn, "sqlite3")
	repo, err := sqlite.NewWithDB(writer, reader, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})
	return repo, writer
}

func visibilityPersistedState(t *testing.T, db *sqlx.DB) map[string]string {
	t.Helper()
	queries := map[string]string{
		"tasks":              `SELECT COALESCE(group_concat(id || '|' || workspace_id || '|' || title || '|' || state || '|' || COALESCE(archived_at, ''), ';'), '') FROM (SELECT id, workspace_id, title, state, archived_at FROM tasks ORDER BY id)`,
		"receipts":           `SELECT COALESCE(group_concat(task_id || '|' || workspace_id || '|' || disposition || '|' || provenance || '|' || revision, ';'), '') FROM (SELECT task_id, workspace_id, disposition, provenance, revision FROM task_lifecycle_receipts ORDER BY task_id)`,
		"sessions":           `SELECT COALESCE(group_concat(id || '|' || task_id || '|' || state, ';'), '') FROM (SELECT id, task_id, state FROM task_sessions ORDER BY id)`,
		"documents":          `SELECT COALESCE(group_concat(id || '|' || task_id || '|' || key || '|' || updated_at, ';'), '') FROM (SELECT id, task_id, key, updated_at FROM task_documents ORDER BY id)`,
		"document_revisions": `SELECT COALESCE(group_concat(id || '|' || task_id || '|' || document_key || '|' || revision_number, ';'), '') FROM (SELECT id, task_id, document_key, revision_number FROM task_document_revisions ORDER BY id)`,
	}
	state := make(map[string]string, len(queries))
	for name, query := range queries {
		var value string
		require.NoError(t, db.Get(&value, query), name)
		state[name] = value
	}
	return state
}
