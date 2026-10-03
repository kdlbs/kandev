package executor

import (
	"context"
	"testing"

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

	_, err := executor.PreflightSessionWorktreeRecovery(context.Background(), session.TaskID, session, false)
	require.NoError(t, err)
	require.NotEmpty(t, got.Slots)
	require.True(t, got.SelectionSnapshot.Valid())
	require.Len(t, got.SelectionSnapshot.Slots, 1)
	require.Equal(t, worktree.ManualRecoveryInspectionWait, got.InspectionWait)
	require.False(t, got.RelocateDirty, "waiting for an inspection must not authorize dirty relocation")
}
