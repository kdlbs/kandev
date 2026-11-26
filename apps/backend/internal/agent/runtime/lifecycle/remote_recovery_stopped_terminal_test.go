package lifecycle

import (
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

func TestRemoteRecoverySettlesRetainedTerminalOutcome(t *testing.T) {
	for _, outcome := range []models.DeliverySubmissionState{
		models.DeliverySubmissionFailed, models.DeliverySubmissionCompleted, models.DeliverySubmissionCancelled,
	} {
		t.Run(string(outcome), func(t *testing.T) {
			runStoppedRemoteRecoveryScenario(t, stoppedRemoteRecoveryScenario{
				runtime: agentruntime.RuntimeRemoteDocker, realDelivery: true, terminalOutcome: outcome,
			})
		})
	}
	t.Run("completed terminal uses the saved turn binding when its effect omits the turn", func(t *testing.T) {
		runStoppedRemoteRecoveryScenario(t, stoppedRemoteRecoveryScenario{
			runtime: agentruntime.RuntimeRemoteDocker, realDelivery: true,
			terminalOutcome: models.DeliverySubmissionCompleted, terminalTurnIDMissing: true,
		})
	})
	t.Run("turnless terminal settles workflow through saved binding without an open turn", func(t *testing.T) {
		runStoppedRemoteRecoveryScenario(t, stoppedRemoteRecoveryScenario{
			runtime: agentruntime.RuntimeRemoteDocker, realDelivery: true,
			terminalOutcome: models.DeliverySubmissionCompleted, terminalTurnIDMissing: true, noOpenTurn: true,
		})
	})
	t.Run("acknowledged SQL terminal is accepted without a retained peer summary", func(t *testing.T) {
		runStoppedRemoteRecoveryScenario(t, stoppedRemoteRecoveryScenario{
			runtime: agentruntime.RuntimeRemoteDocker, realDelivery: true,
			terminalOutcome: models.DeliverySubmissionFailed, omitTerminalSummary: true,
		})
	})
}

func TestRemoteRecoveryCreatesInterruptedNoticeWhenMetadataWasNotWritten(t *testing.T) {
	runStoppedRemoteRecoveryScenario(t, stoppedRemoteRecoveryScenario{
		runtime: agentruntime.RuntimeRemoteDocker, realDelivery: true, missingRecoveryNotice: true,
	})
}

func TestRemoteRecoveryAdoptsExactNoticeAfterSnapshotReadFailure(t *testing.T) {
	runStoppedRemoteRecoveryScenario(t, stoppedRemoteRecoveryScenario{
		runtime: agentruntime.RuntimeRemoteDocker, realDelivery: true,
		missingRecoveryNotice: true, snapshotFailsAfterNotice: true,
	})
}
