package service

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
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
