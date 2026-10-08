package executor

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-MANAGED-CLONE-RELOCATION-001.1, AC-TASKS-MANAGED-CLONE-RELOCATION-002.1
func TestManualRecoveryPreflightRequestsInspectionWaitWithoutDirtyAuthorization(t *testing.T) {
	repo := newMockRepository()
	seedSelectedWorktreeRecoveryEnvironment(repo, "task-manual-recovery", "session-manual-recovery", models.TaskSessionStateCancelled)
	session := repo.sessions["session-manual-recovery"]

	var got worktree.RecoveryAdmissionRequest
	executor := newTestExecutor(t, &mockAgentManager{}, repo)
	executor.SetSelectedWorktreeRecoveryAdmission(func(_ context.Context, req worktree.RecoveryAdmissionRequest) (*worktree.RecoveryAdmission, error) {
		got = req
		return nil, nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	callerDeadline, ok := ctx.Deadline()
	require.True(t, ok)
	_, err := executor.PreflightSessionWorktreeRecovery(ctx, session.TaskID, session, false)
	require.NoError(t, err)
	require.NotEmpty(t, got.Slots)
	require.True(t, got.SelectionSnapshot.Valid())
	require.Len(t, got.SelectionSnapshot.Slots, 1)
	require.Equal(t, worktree.RecoveryInspectionWaitBudget, got.InspectionWait)
	require.True(t, got.InspectionDeadline.Equal(callerDeadline),
		"manual recovery preflight must retain the earlier caller deadline")
	require.False(t, got.RelocateDirty, "waiting for an inspection must not authorize dirty relocation")
}
