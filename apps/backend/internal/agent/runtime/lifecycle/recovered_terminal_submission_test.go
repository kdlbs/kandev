package lifecycle

import (
	"context"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func recoveredTerminalLiveExecution() *AgentExecution {
	return &AgentExecution{
		ID: "execution-live", TaskID: "task-live", SessionID: "session-live",
		Status: v1.AgentStatusRunning, DeliveryMode: DurableDeliveryV1,
		DeliveryStreamID: "incarnation-live:g2", DeliveryIncarnationID: "incarnation-live",
		DeliveryHarnessGeneration: 2, ReplacesNativeConversation: true,
	}
}

func recoveredTerminalEvent(execution *AgentExecution, submissionID string, sequence uint64) agentctl.AgentEvent {
	return agentctl.AgentEvent{
		Type: streams.EventTypeError, PromptGeneration: 1, Recovered: true,
		DeliveryStreamID: execution.DeliveryStreamID, DeliverySequence: sequence,
		DeliveryIncarnationID:     execution.DeliveryIncarnationID,
		DeliveryHarnessGeneration: execution.DeliveryHarnessGeneration,
		DeliverySubmissionID:      submissionID,
		Error:                     "retained provider error",
		Data:                      map[string]any{"is_error": true},
	}
}

func newRecoveredTerminalRepository(execution *AgentExecution) *retentionBootstrapRepository {
	repository := &retentionBootstrapRepository{firstRead: make(chan struct{})}
	repository.generation.Store(&models.HarnessSessionGeneration{
		SessionID: execution.SessionID, IncarnationID: execution.DeliveryIncarnationID,
		Generation: 2, NativeSessionID: "native-live", CreationReason: "native_replaced",
	})
	return repository
}

func newRecoveredTerminalStreamManager(t *testing.T, callbacks *[]string) *StreamManager {
	t.Helper()
	manager := NewStreamManager(newTestLogger(), StreamCallbacks{
		OnAgentEvent: func(_ *AgentExecution, event agentctl.AgentEvent) {
			*callbacks = append(*callbacks, event.Type)
		},
	}, nil, nil)
	t.Cleanup(manager.Wait)
	return manager
}

func TestRecoveredTerminalSubmissionDoesNotCompleteLivePrompt(t *testing.T) {
	execution := recoveredTerminalLiveExecution()
	staleSubmissionID := initialPromptSubmissionID(execution.SessionID, execution.DeliveryHarnessGeneration) + ":execution-stale"
	repository := newRecoveredTerminalRepository(execution)
	var callbacks []string
	manager := newRecoveredTerminalStreamManager(t, &callbacks)
	event := recoveredTerminalEvent(execution, staleSubmissionID, 1)

	prepared, err := manager.prepareAgentEvent(context.Background(), execution, nil, repository, event)
	require.NoError(t, err)
	require.True(t, prepared.staleTerminal, "a terminal event from another prompt attempt is stale")
	require.Empty(t, execution.deliverySubmissionIDSnapshot(),
		"a stale terminal event must not overwrite the live prompt identity")
	require.NoError(t, manager.processPreparedAgentEvent(context.Background(), execution, nil, repository, prepared, 0))
	require.Empty(t, callbacks, "a stale terminal event must not complete the live prompt")
	require.EqualValues(t, 1, repository.receipts.Load(), "the stale event must still be received")
	require.EqualValues(t, 1, repository.projections.Load(), "the stale event must still be projected")
}

func TestRecoveredTerminalSubmissionMatchingLivePromptCompletes(t *testing.T) {
	execution := recoveredTerminalLiveExecution()
	liveSubmissionID := initialPromptDeliverySubmissionID(execution)
	repository := newRecoveredTerminalRepository(execution)
	var callbacks []string
	manager := newRecoveredTerminalStreamManager(t, &callbacks)
	event := recoveredTerminalEvent(execution, liveSubmissionID, 1)

	prepared, err := manager.prepareAgentEvent(context.Background(), execution, nil, repository, event)
	require.NoError(t, err)
	require.False(t, prepared.staleTerminal, "the live prompt's own terminal event remains eligible")
	require.Equal(t, liveSubmissionID, execution.deliverySubmissionIDSnapshot())
	require.NoError(t, manager.processPreparedAgentEvent(context.Background(), execution, nil, repository, prepared, 0))
	require.Equal(t, []string{streams.EventTypeError}, callbacks)
}

func TestClaimPromptCompletionRejectsSupersededSubmission(t *testing.T) {
	manager := newTestManager(t)
	execution := recoveredTerminalLiveExecution()
	liveSubmissionID := initialPromptDeliverySubmissionID(execution)
	execution.setDeliverySubmissionID(liveSubmissionID)
	execution.promptGeneration = 1
	execution.promptDoneCh = make(chan PromptCompletionSignal, 1)
	require.NoError(t, manager.executionStore.Add(execution))

	staleSubmissionID := initialPromptSubmissionID(execution.SessionID, execution.DeliveryHarnessGeneration) + ":execution-stale"
	event := recoveredTerminalEvent(execution, staleSubmissionID, 1)
	_, claimed := manager.claimPromptCompletion(execution, &event, true)
	require.False(t, claimed, "a completion from another prompt attempt cannot claim the live prompt")

	event.DeliverySubmissionID = liveSubmissionID
	claim, claimed := manager.claimPromptCompletion(execution, &event, true)
	require.True(t, claimed)
	require.NotNil(t, claim.execution)
	execution.promptLifecycleMu.Unlock()
}
