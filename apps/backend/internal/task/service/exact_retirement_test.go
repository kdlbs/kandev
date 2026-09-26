package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestExactRetirementReceiptsEligible(t *testing.T) {
	tests := []struct {
		name     string
		statuses []ExactRetirementReceiptStatus
		want     bool
	}{
		{name: "all pass", statuses: []ExactRetirementReceiptStatus{ExactRetirementReceiptPass, ExactRetirementReceiptPass}, want: true},
		{name: "unknown blocks", statuses: []ExactRetirementReceiptStatus{ExactRetirementReceiptPass, ExactRetirementReceiptUnknown}},
		{name: "blocked blocks", statuses: []ExactRetirementReceiptStatus{ExactRetirementReceiptPass, ExactRetirementReceiptBlocked}},
		{name: "empty receipt set", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			receipts := make([]ExactRetirementPredicateReceipt, len(tt.statuses))
			for i, status := range tt.statuses {
				receipts[i].Status = status
			}
			require.Equal(t, tt.want, exactRetirementReceiptsEligible(receipts))
		})
	}
}

// @covers AC-TASKS-EXACT-RETIREMENT-001.2
// @covers AC-TASKS-EXACT-RETIREMENT-001.3
func TestPreviewExactRetirementFailsClosedWithoutInventoryAdapters(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace", Name: "Workspace"}))
	for _, task := range []*models.Task{
		{ID: "old", WorkspaceID: "workspace", Title: "old"},
		{ID: "replacement", WorkspaceID: "workspace", Title: "replacement"},
	} {
		require.NoError(t, repo.CreateTask(ctx, task))
	}
	oldTask, err := svc.GetTask(ctx, "old")
	require.NoError(t, err)
	replacementTask, err := svc.GetTask(ctx, "replacement")
	require.NoError(t, err)

	preview, err := svc.PreviewExactRetirement(ctxSynthetic(), ExactRetirementPreviewRequest{
		OldTaskID: "old", ReplacementTaskID: "replacement", WorkspaceID: "workspace",
		ExpectedOldGeneration:         exactRetirementGeneration(oldTask),
		ExpectedReplacementGeneration: exactRetirementGeneration(replacementTask),
	})
	require.NoError(t, err)
	require.False(t, preview.Eligible)
	require.Len(t, preview.Receipts, 10)
	for _, receipt := range preview.Receipts {
		require.NotEmpty(t, receipt.EvidenceDigest)
		require.NotEmpty(t, receipt.ObservedGeneration)
		if receipt.Predicate == ExactRetirementIdentityPredicate {
			require.Equal(t, ExactRetirementReceiptPass, receipt.Status)
		} else {
			require.Equal(t, ExactRetirementReceiptUnknown, receipt.Status)
			require.Equal(t, "INVENTORY_UNAVAILABLE", receipt.ReasonCode)
		}
	}
	after, err := svc.GetTask(ctx, "old")
	require.NoError(t, err)
	require.Equal(t, oldTask.UpdatedAt, after.UpdatedAt)
}

// @covers AC-TASKS-EXACT-RETIREMENT-001.1
func TestPreviewExactRetirementRejectsMismatchedWorkspaceBeforeReceipt(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-a", Name: "Workspace A"}))
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-b", Name: "Workspace B"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "old", WorkspaceID: "workspace-a", Title: "old"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "replacement", WorkspaceID: "workspace-b", Title: "replacement"}))
	oldTask, err := svc.GetTask(ctx, "old")
	require.NoError(t, err)
	replacementTask, err := svc.GetTask(ctx, "replacement")
	require.NoError(t, err)

	preview, err := svc.PreviewExactRetirement(ctxSynthetic(), ExactRetirementPreviewRequest{
		OldTaskID: "old", ReplacementTaskID: "replacement", WorkspaceID: "workspace-a",
		ExpectedOldGeneration:         exactRetirementGeneration(oldTask),
		ExpectedReplacementGeneration: exactRetirementGeneration(replacementTask),
	})
	require.ErrorIs(t, err, ErrExactRetirementWorkspaceInvalid)
	require.Nil(t, preview)
}

func TestPreviewExactRetirementRequiresTaskWrite(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "retirement-ws", Name: "Workspace", OwnerID: "owner"}))
	for _, id := range []string{"old", "replacement"} {
		require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: "retirement-ws", Title: id}))
	}
	require.NoError(t, repo.UpsertWorkspaceMember(ctx, &models.WorkspaceMember{
		WorkspaceID: "retirement-ws", UserID: "viewer", Role: "viewer",
	}))
	oldTask, err := svc.GetTask(ctx, "old")
	require.NoError(t, err)
	replacementTask, err := svc.GetTask(ctx, "replacement")
	require.NoError(t, err)

	_, err = svc.PreviewExactRetirement(ctxAs("viewer"), ExactRetirementPreviewRequest{
		OldTaskID: "old", ReplacementTaskID: "replacement", WorkspaceID: "retirement-ws",
		ExpectedOldGeneration: exactRetirementGeneration(oldTask), ExpectedReplacementGeneration: exactRetirementGeneration(replacementTask),
	})
	require.ErrorIs(t, err, ErrForbidden)
}

func TestPreviewExactRetirementHidesForeignWorkspace(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "private-ws", Name: "Private", OwnerID: "owner"}))
	for _, id := range []string{"old", "replacement"} {
		require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: "private-ws", Title: id}))
	}
	oldTask, err := svc.GetTask(ctx, "old")
	require.NoError(t, err)
	replacementTask, err := svc.GetTask(ctx, "replacement")
	require.NoError(t, err)

	_, err = svc.PreviewExactRetirement(ctxAs("outsider"), ExactRetirementPreviewRequest{
		OldTaskID: "old", ReplacementTaskID: "replacement", WorkspaceID: "private-ws",
		ExpectedOldGeneration: exactRetirementGeneration(oldTask), ExpectedReplacementGeneration: exactRetirementGeneration(replacementTask),
	})
	require.True(t, errors.Is(err, repoerrors.ErrTaskNotFound), "foreign task error = %v", err)
}

func TestPreviewExactRetirementRejectsDeletedWorkspace(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	const workspaceID = "retirement-ws"
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Workspace", OwnerID: "owner"}))
	for _, id := range []string{"old", "replacement"} {
		require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: workspaceID, Title: id}))
	}
	oldTask, err := svc.GetTask(ctx, "old")
	require.NoError(t, err)
	replacementTask, err := svc.GetTask(ctx, "replacement")
	require.NoError(t, err)
	require.NoError(t, repo.DeleteWorkspace(ctx, workspaceID))

	preview, err := svc.PreviewExactRetirement(ctxSynthetic(), ExactRetirementPreviewRequest{
		OldTaskID: "old", ReplacementTaskID: "replacement", WorkspaceID: workspaceID,
		ExpectedOldGeneration: exactRetirementGeneration(oldTask), ExpectedReplacementGeneration: exactRetirementGeneration(replacementTask),
	})
	require.ErrorIs(t, err, repoerrors.ErrWorkspaceNotFound)
	require.Nil(t, preview)
}

func TestPreviewExactRetirementHidesForeignReplacementFromOldWriter(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "old-workspace", Name: "Old", OwnerID: "owner"}))
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "foreign-workspace", Name: "Foreign", OwnerID: "foreign-owner"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "old", WorkspaceID: "old-workspace", Title: "old"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "foreign-replacement", WorkspaceID: "foreign-workspace", Title: "foreign"}))
	oldTask, err := svc.GetTask(ctx, "old")
	require.NoError(t, err)
	foreignReplacement, err := svc.GetTask(ctx, "foreign-replacement")
	require.NoError(t, err)

	preview, err := svc.PreviewExactRetirement(ctxAs("owner"), ExactRetirementPreviewRequest{
		OldTaskID: "old", ReplacementTaskID: "foreign-replacement", WorkspaceID: "old-workspace",
		ExpectedOldGeneration: exactRetirementGeneration(oldTask), ExpectedReplacementGeneration: exactRetirementGeneration(foreignReplacement),
	})
	require.ErrorIs(t, err, repoerrors.ErrTaskNotFound)
	require.Nil(t, preview)
}

func TestPreviewExactRetirementRequiresAdminTaskWriter(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "retirement-ws", Name: "Workspace", OwnerID: "owner"}))
	for _, id := range []string{"old", "replacement"} {
		require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: "retirement-ws", Title: id}))
	}
	require.NoError(t, repo.UpsertWorkspaceMember(ctx, &models.WorkspaceMember{
		WorkspaceID: "retirement-ws", UserID: "collaborator", Role: "collaborator",
	}))
	oldTask, err := svc.GetTask(ctx, "old")
	require.NoError(t, err)
	replacementTask, err := svc.GetTask(ctx, "replacement")
	require.NoError(t, err)
	request := ExactRetirementPreviewRequest{
		OldTaskID: "old", ReplacementTaskID: "replacement", WorkspaceID: "retirement-ws",
		ExpectedOldGeneration: exactRetirementGeneration(oldTask), ExpectedReplacementGeneration: exactRetirementGeneration(replacementTask),
	}
	assertTasksUnchanged := func() {
		t.Helper()
		afterOld, err := svc.GetTask(ctx, "old")
		require.NoError(t, err)
		afterReplacement, err := svc.GetTask(ctx, "replacement")
		require.NoError(t, err)
		require.Equal(t, oldTask, afterOld)
		require.Equal(t, replacementTask, afterReplacement)
	}
	preview, err := svc.PreviewExactRetirement(ctxAs("collaborator"), request)
	require.ErrorIs(t, err, ErrForbidden)
	require.Nil(t, preview)
	assertTasksUnchanged()
	preview, err = svc.PreviewExactRetirement(ctx, request)
	require.ErrorIs(t, err, ErrForbidden)
	require.Nil(t, preview)
	assertTasksUnchanged()

	adminCtx := authn.WithIdentity(ctx, authn.Identity{UserID: "collaborator", Role: authn.RoleAdmin})
	preview, err = svc.PreviewExactRetirement(adminCtx, request)
	require.NoError(t, err)
	require.False(t, preview.Eligible)
	assertTasksUnchanged()
}
