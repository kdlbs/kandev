package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestSilentRestoreSourceDeathProof(t *testing.T) {
	identity := processidentity.Identity{
		PID: 42, GroupID: 42, SessionID: 42, BirthToken: "linux:boot:1",
	}
	recovery := models.AgentDeliveryRecovery{
		SessionID: "session", AgentExecutionID: "source", OriginalRuntime: identity,
	}
	row := &models.ExecutorRunning{
		SessionID: "session", AgentExecutionID: "source", Runtime: agentruntime.RuntimeStandalone,
	}
	probeError := errors.New("process inspection failed")

	tests := []struct {
		name           string
		row            *models.ExecutorRunning
		recovery       models.AgentDeliveryRecovery
		liveness       models.ProcessLiveness
		terminated     bool
		terminationErr error
		wantProbes     int
		wantDead       bool
	}{
		{
			name: "unknown local row without persisted pid uses exact process proof",
			row:  row, recovery: recovery, liveness: models.ProcessLivenessUnknown,
			terminated: true, wantProbes: 1, wantDead: true,
		},
		{
			name: "already dead row needs no fallback probe",
			row:  row, recovery: recovery, liveness: models.ProcessLivenessDead,
			wantProbes: 0, wantDead: true,
		},
		{
			name: "live row blocks fallback",
			row:  row, recovery: recovery, liveness: models.ProcessLivenessAlive,
			terminated: true, wantProbes: 0,
		},
		{
			name:     "mismatched execution blocks fallback",
			row:      &models.ExecutorRunning{SessionID: "session", AgentExecutionID: "successor", Runtime: agentruntime.RuntimeStandalone},
			recovery: recovery, liveness: models.ProcessLivenessUnknown, terminated: true, wantProbes: 0,
		},
		{
			name: "incomplete process identity blocks fallback",
			row:  row, recovery: models.AgentDeliveryRecovery{
				SessionID: "session", AgentExecutionID: "source",
			}, liveness: models.ProcessLivenessUnknown, terminated: true, wantProbes: 0,
		},
		{
			name:     "remote row never uses local process proof",
			row:      &models.ExecutorRunning{SessionID: "session", AgentExecutionID: "source", Runtime: agentruntime.RuntimeDocker},
			recovery: recovery, liveness: models.ProcessLivenessUnknown, terminated: true, wantProbes: 0,
		},
		{
			name: "live descendant blocks exact owner proof",
			row:  row, recovery: recovery, liveness: models.ProcessLivenessUnknown,
			terminated: false, wantProbes: 1,
		},
		{
			name: "process inspection error blocks exact owner proof",
			row:  row, recovery: recovery, liveness: models.ProcessLivenessUnknown,
			terminationErr: probeError, wantProbes: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			probes := 0
			dead, err := silentRestoreSourceIsProvenDead(
				test.row, test.recovery, test.liveness,
				func(got processidentity.Identity) (bool, error) {
					probes++
					require.Equal(t, identity, got)
					return test.terminated, test.terminationErr
				},
			)
			require.Equal(t, test.wantProbes, probes)
			require.Equal(t, test.wantDead, dead)
			if test.wantDead {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

type silentRestoreCleanupRaceManager struct {
	*mockAgentManager
	beforeCleanup func()
}

func (m *silentRestoreCleanupRaceManager) CleanupStaleExecutionBySessionIDIfCurrent(
	context.Context,
	string,
	string,
	time.Time,
) error {
	if m.beforeCleanup != nil {
		m.beforeCleanup()
	}
	return nil
}

func TestSilentRestoreSourceCleanupKeepsWorkspaceSuccessor(t *testing.T) {
	fixture := newSilentRestoreCheckpointFixtureWithLaunching(t, false)
	observed, err := fixture.repo.GetExecutorRunningBySessionID(fixture.ctx, fixture.session.ID)
	require.NoError(t, err)
	fixture.manager.rowLivenessFn = func(*models.ExecutorRunning) models.ProcessLiveness {
		return models.ProcessLivenessDead
	}

	manager := &silentRestoreCleanupRaceManager{
		mockAgentManager: fixture.manager,
		beforeCleanup: func() {
			successor := *observed
			successor.ID = "workspace-successor"
			successor.AgentExecutionID = "workspace-successor"
			successor.Status = models.ExecutorRunningStatusRunning
			require.NoError(t, fixture.repo.UpsertExecutorRunning(fixture.ctx, &successor))
		},
	}
	fixture.service.agentManager = manager

	err = fixture.service.removeProvenDeadSourceExecution(
		fixture.ctx, fixture.inputs, fixture.checkpoint, observed,
	)
	require.ErrorIs(t, err, models.ErrExecutionRotated)

	current, readErr := fixture.repo.GetExecutorRunningBySessionID(fixture.ctx, fixture.session.ID)
	require.NoError(t, readErr)
	require.Equal(t, "workspace-successor", current.AgentExecutionID)
}

func TestSilentRestoreSourceCleanupRejectsExistingWorkspaceSuccessor(t *testing.T) {
	fixture := newSilentRestoreCheckpointFixtureWithLaunching(t, false)
	source, err := fixture.repo.GetExecutorRunningBySessionID(fixture.ctx, fixture.session.ID)
	require.NoError(t, err)
	successor := *source
	successor.ID = "workspace-successor"
	successor.AgentExecutionID = "workspace-successor"
	require.NoError(t, fixture.repo.UpsertExecutorRunning(fixture.ctx, &successor))

	livenessCalls := 0
	fixture.manager.rowLivenessFn = func(*models.ExecutorRunning) models.ProcessLiveness {
		livenessCalls++
		return models.ProcessLivenessDead
	}
	err = fixture.service.removeProvenDeadSourceExecution(
		fixture.ctx, fixture.inputs, fixture.checkpoint, source,
	)
	require.ErrorContains(t, err, "does not match either pinned identity")
	require.Zero(t, livenessCalls)

	current, readErr := fixture.repo.GetExecutorRunningBySessionID(fixture.ctx, fixture.session.ID)
	require.NoError(t, readErr)
	require.Equal(t, "workspace-successor", current.AgentExecutionID)
}
