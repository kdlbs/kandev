package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type managedCompletionReceiptAgentManager struct {
	*mockAgentManager
	pending  bool
	ackCalls int
}

func (m *managedCompletionReceiptAgentManager) ManagedAgentCompletionPending(context.Context, string) (bool, error) {
	return m.pending, nil
}

func (m *managedCompletionReceiptAgentManager) AcknowledgeManagedAgentCompletion(context.Context, string) error {
	m.ackCalls++
	m.pending = false
	return nil
}

func TestManagedCompletionReplayIsAcknowledgedOnlyOnce(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t-managed-completion", "s-managed-completion", "step1")
	manager := &managedCompletionReceiptAgentManager{mockAgentManager: &mockAgentManager{}, pending: true}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.turnService = &repoTurnService{repo: repo}
	eventBus := &recordingEventBus{}
	svc.eventBus = eventBus
	turn, err := svc.turnService.StartTurn(ctx, "s-managed-completion")
	require.NoError(t, err)
	payload := &lifecycle.AgentStreamEventPayload{
		TaskID: "t-managed-completion", SessionID: "s-managed-completion", ExecutionID: "exec-managed-completion",
		ManagedAgentOperationID: "operation-managed-completion",
		Data: &lifecycle.AgentStreamEventData{
			Type: agentEventComplete, TurnID: turn.ID,
			Data: map[string]interface{}{"remote_terminal": true, "stop_reason": "end_turn"},
		},
	}

	svc.handleAgentStreamEvent(ctx, payload)
	effectsAfterFirstDelivery := len(eventBus.events)
	svc.handleAgentStreamEvent(ctx, payload)

	require.Equal(t, 1, manager.ackCalls, "the completion receipt should be acknowledged once")
	var turnMessageEvents int
	for _, event := range eventBus.events {
		if event.subject == events.AgentTurnMessageSaved {
			turnMessageEvents++
		}
	}
	require.Equal(t, 1, turnMessageEvents, "replayed completion must not republish turn completion")
	require.Len(t, eventBus.events, effectsAfterFirstDelivery, "replayed completion must not repeat workflow or queue event effects")
	active, err := svc.turnService.GetActiveTurn(ctx, "s-managed-completion")
	require.NoError(t, err)
	require.Nil(t, active, "the captured turn should be closed by the first delivery")
}

func TestDeferCompleteEventStateTransitionForRemoteTerminal(t *testing.T) {
	svc := &Service{logger: testLogger()}
	session := &models.TaskSession{State: models.TaskSessionStateRunning}
	remoteComplete := &lifecycle.AgentStreamEventPayload{
		Data: &lifecycle.AgentStreamEventData{Data: map[string]interface{}{"remote_terminal": true}},
	}
	ordinaryComplete := &lifecycle.AgentStreamEventPayload{Data: &lifecycle.AgentStreamEventData{}}

	require.False(t, svc.deferCompleteEventStateTransition(remoteComplete, session))
	require.True(t, svc.deferCompleteEventStateTransition(ordinaryComplete, session))
}
