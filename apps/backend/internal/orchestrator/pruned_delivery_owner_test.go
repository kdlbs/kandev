package orchestrator

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type prunedDeliveryRowProber struct {
	*mockAgentManager
	liveness models.ProcessLiveness
}

func (m *prunedDeliveryRowProber) RowLiveness(*models.ExecutorRunning) models.ProcessLiveness {
	return m.liveness
}

// @covers AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.4
// @covers AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.8
func TestPrunedDeliverySourceCleanupSeparatelyProvesReplacementDead(t *testing.T) {
	for _, state := range []models.ProcessLiveness{models.ProcessLivenessDead, models.ProcessLivenessAlive, models.ProcessLivenessUnknown} {
		t.Run(fmt.Sprint(state), func(t *testing.T) {
			fixture := newSilentRestoreCheckpointFixtureWithLaunching(t, false)
			fixture.inputs.recovery.HistoryGap = &models.AgentDeliveryHistoryGap{FirstSequence: 2, LastSequence: 6, ReceivedSequence: 1, ProjectedSequence: 1}
			fixture.checkpoint.SourceRecovery = fixture.inputs.recovery
			replacement := &models.ExecutorRunning{ID: "failed-replacement", TaskID: fixture.session.TaskID, SessionID: fixture.session.ID,
				AgentExecutionID: "failed-replacement", Status: models.ExecutorRunningStatusStopped}
			require.NoError(t, fixture.repo.UpsertExecutorRunning(context.Background(), replacement))
			prober := &prunedDeliveryRowProber{mockAgentManager: fixture.manager, liveness: state}
			fixture.service.agentManager = prober
			row, err := fixture.service.loadSilentRestoreSourceExecution(fixture.ctx, fixture.inputs, fixture.checkpoint, replacement)
			if state != models.ProcessLivenessDead {
				require.Error(t, err)
				return
			}
			require.NoError(t, err, "an independently proven dead failed startup must not pin the old interrupted prompt forever")
			require.Equal(t, "failed-replacement", row.AgentExecutionID)
			require.NoError(t, fixture.service.proveSilentRestoreSourceDead(row, fixture.inputs.recovery))
			prober.liveness = models.ProcessLivenessAlive
			require.Error(t, fixture.service.proveSilentRestoreSourceDead(row, fixture.inputs.recovery), "row selection cannot authorize cleanup after liveness changes")
		})
	}
}
