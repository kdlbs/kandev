package orchestrator

import (
	"context"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type retiredIdleProbe struct{ *mockAgentManager }

func (m *retiredIdleProbe) ProbeAgentRunningForSession(context.Context, string) (bool, error) {
	return false, agentctl.ErrRuntimeLeaseRetired
}

func TestRetiredIdleReclaimRequiresDeadOwner(t *testing.T) {
	for _, state := range []models.ProcessLiveness{models.ProcessLivenessDead, models.ProcessLivenessAlive, models.ProcessLivenessUnknown} {
		t.Run(processLivenessClass(state), func(t *testing.T) {
			repo := setupTestRepo(t)
			ctx := context.Background()
			seedIdleReaperRow(t, repo, "task", "session", models.TaskSessionStateWaitingForInput, models.ExecutorRunningStatusRunning)
			row, err := repo.GetExecutorRunningBySessionID(ctx, "session")
			require.NoError(t, err)
			row.ResumeToken, row.WorktreePath = "native-conversation", "/retained/workspace"
			require.NoError(t, repo.UpsertExecutorRunning(ctx, row))
			manager := &retiredIdleProbe{&mockAgentManager{repoForExecutionLookup: repo,
				rowLivenessFn: func(*models.ExecutorRunning) models.ProcessLiveness { return state }}}
			service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
			service.turnService = &inactiveTurnService{}
			require.NoError(t, service.reclaimIdleSession(ctx, "session"))
			repaired, err := repo.GetExecutorRunningBySessionID(ctx, "session")
			require.NoError(t, err)
			want := models.ExecutorRunningStatusRunning
			if state == models.ProcessLivenessDead {
				want = models.ExecutorRunningStatusStopped
			}
			require.Equal(t, want, repaired.Status)
			require.Equal(t, row.ResumeToken, repaired.ResumeToken)
			require.Equal(t, row.WorktreePath, repaired.WorktreePath)
		})
	}
}
