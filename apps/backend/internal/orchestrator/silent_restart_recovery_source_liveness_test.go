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
		AgentGroupID: 84, AgentBirthToken: "linux:boot:2",
	}
	agentGroup := processidentity.Identity{PID: 84, GroupID: 84, BirthToken: "linux:boot:2"}
	recovery := models.AgentDeliveryRecovery{
		SessionID: "session", AgentExecutionID: "source", OriginalRuntime: identity,
	}
	row := &models.ExecutorRunning{
		SessionID: "session", AgentExecutionID: "source", Runtime: agentruntime.RuntimeStandalone,
	}
	probeError := errors.New("process inspection failed")

	tests := []struct {
		name                string
		row                 *models.ExecutorRunning
		recovery            models.AgentDeliveryRecovery
		liveness            models.ProcessLiveness
		terminated          bool
		terminationErr      error
		groupTerminated     bool
		groupTerminationErr error
		groupIdentity       processidentity.Identity
		wantOwnerProbes     int
		wantGroupProbes     int
		wantDead            bool
	}{
		{
			name: "unknown inherited row uses original owner proof",
			row:  row, recovery: recovery, liveness: models.ProcessLivenessUnknown,
			terminated: true, wantOwnerProbes: 1, wantDead: true,
		},
		{
			name: "already dead row needs no fallback probe",
			row:  row, recovery: recovery, liveness: models.ProcessLivenessDead,
			wantDead: true,
		},
		{
			name: "registered shared runtime allows an exited agent group",
			row:  row, recovery: recovery, liveness: models.ProcessLivenessAlive,
			groupTerminated: true, groupIdentity: agentGroup, wantGroupProbes: 1, wantDead: true,
		},
		{
			name: "registered shared runtime blocks a live agent group",
			row:  row, recovery: recovery, liveness: models.ProcessLivenessAlive,
			groupIdentity: agentGroup, wantGroupProbes: 1,
		},
		{
			name: "registered shared runtime blocks an unverifiable agent group",
			row:  row, recovery: recovery, liveness: models.ProcessLivenessAlive,
			groupTerminationErr: probeError, groupIdentity: agentGroup, wantGroupProbes: 1,
		},
		{
			name: "registered row without agent group proof stays blocked",
			row:  row, recovery: models.AgentDeliveryRecovery{
				SessionID: "session", AgentExecutionID: "source",
				OriginalRuntime: processidentity.Identity{PID: 42, GroupID: 42, SessionID: 42, BirthToken: "linux:boot:1"},
			}, liveness: models.ProcessLivenessAlive,
		},
		{
			name:     "mismatched execution blocks fallback",
			row:      &models.ExecutorRunning{SessionID: "session", AgentExecutionID: "successor", Runtime: agentruntime.RuntimeStandalone},
			recovery: recovery, liveness: models.ProcessLivenessUnknown, terminated: true,
		},
		{
			name: "incomplete process identity blocks fallback",
			row:  row, recovery: models.AgentDeliveryRecovery{
				SessionID: "session", AgentExecutionID: "source",
			}, liveness: models.ProcessLivenessUnknown, terminated: true,
		},
		{
			name:     "remote row never uses local process proof",
			row:      &models.ExecutorRunning{SessionID: "session", AgentExecutionID: "source", Runtime: agentruntime.RuntimeDocker},
			recovery: recovery, liveness: models.ProcessLivenessUnknown, terminated: true,
		},
		{
			name: "live descendant blocks exact owner proof",
			row:  row, recovery: recovery, liveness: models.ProcessLivenessUnknown,
			terminated: false, wantOwnerProbes: 1,
		},
		{
			name: "process inspection error blocks exact owner proof",
			row:  row, recovery: recovery, liveness: models.ProcessLivenessUnknown,
			terminationErr: probeError, wantOwnerProbes: 1,
		},
		{
			name:     "remote live row cannot use local group proof",
			row:      &models.ExecutorRunning{SessionID: "session", AgentExecutionID: "source", Runtime: agentruntime.RuntimeDocker},
			recovery: recovery, liveness: models.ProcessLivenessAlive, groupTerminated: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ownerProbes, groupProbes := 0, 0
			testRecovery := test.recovery
			if testRecovery.SessionID == "" {
				testRecovery = recovery
			}
			dead, err := silentRestoreSourceIsProvenDead(
				test.row, testRecovery, test.liveness,
				func(got processidentity.Identity) (bool, error) {
					ownerProbes++
					require.Equal(t, testRecovery.OriginalRuntime, got)
					return test.terminated, test.terminationErr
				},
				func(got processidentity.Identity) (bool, error) {
					groupProbes++
					require.Equal(t, test.groupIdentity, got)
					return test.groupTerminated, test.groupTerminationErr
				},
			)
			require.Equal(t, test.wantOwnerProbes, ownerProbes)
			require.Equal(t, test.wantGroupProbes, groupProbes)
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
