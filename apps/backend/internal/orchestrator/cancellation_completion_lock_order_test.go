package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type settlementGuardAgentManager struct {
	*mockAgentManager
	service        *Service
	guardAvailable bool
	acknowledged   []retainedPromptAck
}

func (m *settlementGuardAgentManager) AcknowledgeRetainedPromptFailure(executionID string, generation uint64) bool {
	mutex, release := m.service.acquireCancelInFlightGuard("s1")
	defer release()
	m.guardAvailable = mutex.TryLock()
	if m.guardAvailable {
		mutex.Unlock()
	}
	if executionID != m.currentPromptExecutionID || generation != m.currentPromptGeneration.Load() {
		return false
	}
	m.acknowledged = append(m.acknowledged, retainedPromptAck{executionID: executionID, generation: generation})
	return true
}

func newSettlementGuardFixture(t *testing.T) (*Service, *settlementGuardAgentManager, *mockMessageCreator) {
	t.Helper()
	svc, messages := newTransientTestService(t)
	t.Cleanup(svc.cancelAllTransientRetries)
	base := svc.agentManager.(*mockAgentManager)
	base.isAgentRunning = true
	base.currentPromptExecutionID = "execution-1"
	base.currentPromptGeneration.Store(7)
	manager := &settlementGuardAgentManager{mockAgentManager: base, service: svc}
	svc.agentManager = manager
	svc.taskRepo = newTaskServiceStateRepository(svc.repo.(*sqliterepo.Repository), svc.eventBus)
	seedExecutorRunning(t, svc.repo.(*sqliterepo.Repository), "s1", "t1", "execution-1")
	armTransientPromptEvidence(svc)
	return svc, manager, messages
}

// @covers AC-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001.1
func TestCancelAgent_SettlementAcknowledgementReleasesGuard(t *testing.T) {
	svc, manager, _ := newSettlementGuardFixture(t)

	require.NoError(t, svc.CancelAgent(context.Background(), "s1"))

	require.True(t, manager.guardAvailable,
		"late stream publication must be able to take the session guard while lifecycle acknowledgement waits")
	require.Equal(t, []retainedPromptAck{{executionID: "execution-1", generation: 7}}, manager.acknowledged)
	require.Equal(t, int32(1), manager.cancelAgentCalls.Load())
	require.False(t, svc.CancellationPending("s1"))
	assertResumeSessionState(t, svc.repo, "s1", models.TaskSessionStateWaitingForInput)
	requireNoCancelGuardReferences(t, svc, "s1")
}

// @covers AC-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001.1
func TestRetainedTurnFailure_AcknowledgementReleasesGuard(t *testing.T) {
	svc, manager, messages := newSettlementGuardFixture(t)

	svc.handleAgentTurnFailed(context.Background(), retainedTurnFailureData())

	require.True(t, manager.guardAvailable,
		"retained failure acknowledgement must not hold the guard needed by ordered stream consumers")
	require.Equal(t, []retainedPromptAck{{executionID: "execution-1", generation: 7}}, manager.acknowledged)
	require.Len(t, messages.sessionMessages, 1)
	assertResumeSessionState(t, svc.repo, "s1", models.TaskSessionStateWaitingForInput)
	requireNoCancelGuardReferences(t, svc, "s1")
}
