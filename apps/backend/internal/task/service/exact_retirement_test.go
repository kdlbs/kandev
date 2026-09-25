package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-EXACT-RETIREMENT-001.2
// @covers AC-TASKS-EXACT-RETIREMENT-001.3
func TestPreviewExactRetirementFailsClosedWithoutInventoryAdapters(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
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

	preview, err := svc.PreviewExactRetirement(ctx, ExactRetirementPreviewRequest{
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
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "old", WorkspaceID: "workspace-a", Title: "old"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "replacement", WorkspaceID: "workspace-b", Title: "replacement"}))
	oldTask, err := svc.GetTask(ctx, "old")
	require.NoError(t, err)
	replacementTask, err := svc.GetTask(ctx, "replacement")
	require.NoError(t, err)

	preview, err := svc.PreviewExactRetirement(ctx, ExactRetirementPreviewRequest{
		OldTaskID: "old", ReplacementTaskID: "replacement", WorkspaceID: "workspace-a",
		ExpectedOldGeneration:         exactRetirementGeneration(oldTask),
		ExpectedReplacementGeneration: exactRetirementGeneration(replacementTask),
	})
	require.ErrorIs(t, err, ErrExactRetirementWorkspaceInvalid)
	require.Nil(t, preview)
}
