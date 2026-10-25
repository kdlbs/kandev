package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
)

func TestDurablePromptTurnBindingPersistsBeforeProviderDispatch(t *testing.T) {
	sm, execution, _, _ := initialDeliveryPersistenceFixture(t)
	execution.DeliveryStreamID = "stream"
	execution.setPromptTurnID("turn-initial")
	store := NewExecutionStore()
	require.NoError(t, store.Add(execution))
	var persisted *models.ExecutorRunning
	sm.SetPromptStarter(func(id string) (uint64, error) {
		return store.BeginPrompt(id)
	})
	sm.persistPromptTurnBinding = func(_ context.Context, current *AgentExecution) error {
		persisted = buildRunningFromExecution(current, nil)
		return nil
	}
	captured := false
	sm.turnChangeAdmitted = func(_ context.Context, current *AgentExecution, generation uint64) error {
		require.NotNil(t, persisted, "persist durable ownership before capturing a turn")
		require.Equal(t, "turn-initial", current.promptTurnIDSnapshot())
		require.NotZero(t, generation)
		captured = true
		return nil
	}
	sm.beforePromptDispatchHook = func() {
		require.True(t, captured, "capture the admitted turn before provider dispatch")
		require.NotNil(t, persisted)
		encoded, err := json.Marshal(persisted.Metadata)
		require.NoError(t, err)
		var metadata map[string]interface{}
		require.NoError(t, json.Unmarshal(encoded, &metadata))
		binding, ok := metadata["delivery_prompt_turn_binding"].(map[string]interface{})
		require.True(t, ok, "the durable runtime owner must retain the admitted turn before dispatch")
		require.Equal(t, "turn-initial", binding["turn_id"])
		require.Equal(t, initialPromptDeliverySubmissionID(execution), binding["submission_id"])
		require.Equal(t, execution.ID, binding["execution_id"])
		require.True(t, IsSessionScopedMetadataKey("delivery_prompt_turn_binding"))
	}
	_, err := sm.sendPrompt(context.Background(), execution, "initial instruction", false, nil, true,
		sendPromptCallbacks{deliverySubmissionID: initialPromptDeliverySubmissionID(execution)}, false)
	require.NoError(t, err)
}

func TestDurablePromptTurnBindingWriteFailurePreventsProviderDispatch(t *testing.T) {
	sm, execution, repository, _ := initialDeliveryPersistenceFixture(t)
	execution.DeliveryStreamID = "stream"
	execution.setPromptTurnID("turn-initial")
	writeErr := errors.New("runtime owner write failed")
	sm.persistPromptTurnBinding = func(context.Context, *AgentExecution) error { return writeErr }
	sm.turnChangeAdmitted = func(context.Context, *AgentExecution, uint64) error {
		t.Fatal("a failed durable owner write must prevent turn capture")
		return nil
	}
	reached := false
	sm.beforePromptDispatchHook = func() { reached = true }
	id := initialPromptDeliverySubmissionID(execution)
	_, err := sm.sendPrompt(context.Background(), execution, "initial instruction", false, nil, true,
		sendPromptCallbacks{deliverySubmissionID: id}, false)
	require.ErrorIs(t, err, writeErr)
	require.False(t, reached, "a failed durable owner write must prevent provider dispatch")
	stored, err := repository.GetAgentDeliverySubmission(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionDispatching, stored.State)
}

func TestDurablePromptWithoutTurnClearsThePersistedPredecessorBinding(t *testing.T) {
	sm, execution, _, _ := initialDeliveryPersistenceFixture(t)
	execution.setMetadataValue("delivery_prompt_turn_binding", "predecessor")
	persisted := false
	sm.persistPromptTurnBinding = func(_ context.Context, current *AgentExecution) error {
		persisted = true
		require.Nil(t, buildRunningFromExecution(current, nil).Metadata["delivery_prompt_turn_binding"])
		return nil
	}
	sm.beforePromptDispatchHook = func() { require.True(t, persisted) }
	_, err := sm.sendPrompt(context.Background(), execution, "initial instruction", false, nil, true,
		sendPromptCallbacks{deliverySubmissionID: initialPromptDeliverySubmissionID(execution)}, false)
	require.NoError(t, err)
}

func TestDurableAdoptionRestoresOnlyTheMatchingPromptTurnBinding(t *testing.T) {
	for _, field := range []string{"matching", "missing", "task_id", "session_id", "execution_id", "incarnation_id", "harness_generation", "stream_id", "submission_id", "turn_id"} {
		t.Run(field, func(t *testing.T) {
			repository := durableAdoptionRepository()
			repository.submissions["submission-terminal"] = &models.AgentDeliverySubmission{
				ID: "submission-terminal", SessionID: "session-1", IncarnationID: "incarnation-1",
				HarnessGeneration: 2, PayloadHash: "hash-terminal", State: models.DeliverySubmissionDispatching,
			}
			status := durableAdoptionStatus()
			status.Submissions = []journal.SubmissionSummary{{
				ID: "submission-terminal", SessionID: "session-1", IncarnationID: "incarnation-1",
				HarnessGeneration: 2, Hash: "hash-terminal", State: journal.SubmissionCompleted,
				TerminalEventRetained: true, TerminalSequence: 3,
			}}
			binding := map[string]interface{}{
				"task_id": "task-1", "session_id": "session-1", "execution_id": "execution-1",
				"incarnation_id": "incarnation-1", "harness_generation": float64(2),
				"stream_id": "stream-1", "submission_id": "submission-terminal", "turn_id": "turn-original",
			}
			switch field {
			case "harness_generation":
				binding[field] = float64(3)
			case "turn_id":
				binding[field] = ""
			case "matching", "missing":
			default:
				binding[field] = "foreign"
			}
			metadata := map[string]interface{}{"delivery_prompt_turn_binding": binding}
			if field == "missing" {
				metadata = nil
			}
			execution := &AgentExecution{ID: "execution-1", TaskID: "task-1", SessionID: "session-1", metadata: metadata}
			err := newAdoptionManager(repository).restoreRecoveredDelivery(context.Background(), execution,
				&ExecutorInstance{DeliveryStatus: status})
			switch field {
			case "matching":
				require.NoError(t, err)
				require.Equal(t, "turn-original", execution.promptTurnIDSnapshot())
			case "missing":
				require.NoError(t, err)
				require.Empty(t, execution.promptTurnIDSnapshot(), "missing proof must not invent a turn binding")
			default:
				require.ErrorIs(t, err, ErrDurableAdoptionIdentityMismatch)
				require.Empty(t, execution.promptTurnIDSnapshot(), "foreign proof must not attach another turn")
			}
		})
	}
}
