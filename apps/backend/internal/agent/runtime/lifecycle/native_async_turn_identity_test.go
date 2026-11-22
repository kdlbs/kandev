package lifecycle

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.12
func TestNativeAsyncDeliveryDoesNotInheritCompletedPromptTurn(t *testing.T) {
	execution := &AgentExecution{SessionID: "session", promptTurnID: "completed-turn", promptGeneration: 10}
	for _, kind := range []string{"message_chunk", "tool_call", "complete"} {
		t.Run(kind, func(t *testing.T) {
			event := agentctl.AgentEvent{Type: kind, DeliveryStreamID: "stream", DeliverySequence: 100}
			stored := durableAgentEvent(execution, event)
			var payload agentctl.AgentEvent
			require.NoError(t, json.Unmarshal(stored.Payload, &payload))
			require.Empty(t, payload.TurnID, "unowned native activity must not acquire an old dispatched turn")
		})
	}
}

func TestNativeAsyncCompletionDoesNotPublishCompletedPromptOwner(t *testing.T) {
	manager, bus := createTestManagerWithTracking()
	execution := createTestExecution("execution", "task", "session")
	execution.promptTurnID = "completed-turn"
	execution.promptGeneration = 10
	execution.promptCompletionGeneration = 10
	require.NoError(t, manager.executionStore.Add(execution))
	manager.handleAgentEvent(execution, agentctl.AgentEvent{Type: "complete", SessionID: "session",
		Data: map[string]any{"synthetic": true, "synthetic_reason": "async_turn_idle"}})
	ready, complete := false, false
	for _, recorded := range bus.PublishedEvents {
		if recorded.Event == nil {
			continue
		}
		if recorded.Event.Type == events.AgentReady {
			payload := recorded.Event.Data.(AgentEventPayload)
			require.Empty(t, payload.TurnID)
			require.Zero(t, payload.PromptGeneration)
			ready = true
		}
		if payload, ok := recorded.Event.Data.(AgentStreamEventPayload); ok && payload.Data.Type == "complete" {
			require.Empty(t, payload.Data.TurnID)
			require.Zero(t, payload.Data.PromptGeneration)
			complete = true
		}
	}
	require.True(t, ready)
	require.True(t, complete)
}

func TestRecoveredLiveCompletionKeepsRestoredTurnOwnership(t *testing.T) {
	manager, bus := createTestManagerWithTracking()
	execution := createTestExecution("execution", "task", "session")
	execution.setPromptTurnID("surviving-turn")
	require.NoError(t, manager.executionStore.Add(execution))
	manager.publishRecoveredExecutionRunning(t.Context(), execution)
	event := agentctl.AgentEvent{Type: "complete", SessionID: "session", PromptGeneration: 5}
	stored := durableAgentEvent(execution, event)
	var payload agentctl.AgentEvent
	require.NoError(t, json.Unmarshal(stored.Payload, &payload))
	require.Equal(t, "surviving-turn", payload.TurnID)
	manager.handleAgentEvent(execution, event)
	ready := false
	for _, recorded := range bus.PublishedEvents {
		if recorded.Event != nil && recorded.Event.Type == events.AgentReady {
			payload := recorded.Event.Data.(AgentEventPayload)
			require.Equal(t, "surviving-turn", payload.TurnID)
			require.Equal(t, uint64(5), payload.PromptGeneration)
			ready = true
		}
	}
	require.True(t, ready)
	require.False(t, execution.recoveredPromptGenerationPending.Load())
	execution.setPromptTurnID("successor")
	require.Equal(t, uint64(6), beginExecutionPrompt(execution))
	require.Equal(t, "surviving-turn", execution.promptTurnIDForEvent(event), "retained old output must keep its original owner")
	before := len(bus.PublishedEvents)
	manager.handleAgentEvent(execution, event)
	require.Len(t, bus.PublishedEvents, before, "late old completion must not settle the successor")
}

func TestNumberedOutputKeepsMessageScopeAfterControlCompletion(t *testing.T) {
	for _, kind := range []string{"message_chunk", "reasoning"} {
		t.Run(kind, func(t *testing.T) {
			manager, _ := createTestManagerWithTracking()
			execution := createTestExecution("execution", "task", "session")
			execution.setPromptTurnID("original-turn")
			require.NoError(t, manager.executionStore.Add(execution))
			generation := beginExecutionPrompt(execution)
			execution.dispatchedPromptPending.Store(true)
			event := agentctl.AgentEvent{Type: kind, PromptGeneration: generation, DeliveryStreamID: "stream", DeliverySequence: 1}
			before := durableAgentEvent(execution, event)
			manager.handleAgentEvent(execution, agentctl.AgentEvent{Type: "complete", PromptGeneration: generation})
			event.DeliverySequence++
			after := durableAgentEvent(execution, event)
			var first, last agentctl.AgentEvent
			require.NoError(t, json.Unmarshal(before.Payload, &first))
			require.NoError(t, json.Unmarshal(after.Payload, &last))
			require.Equal(t, first.CanonicalMessageID, last.CanonicalMessageID)
			require.Equal(t, "original-turn", last.TurnID)
			manager.handleAgentEvent(execution, event)
			require.Equal(t, v1.AgentStatusReady, execution.Status, "retained output must not revive a completed prompt")
		})
	}
}

func TestDurableEventKeepsExplicitDispatchOwnership(t *testing.T) {
	for _, tc := range []struct {
		name    string
		event   agentctl.AgentEvent
		pending bool
		want    string
	}{
		{name: "explicit", event: agentctl.AgentEvent{TurnID: "native-turn"}, want: "native-turn"},
		{name: "current generation", event: agentctl.AgentEvent{PromptGeneration: 10}, want: "current-turn"},
		{name: "older generation", event: agentctl.AgentEvent{PromptGeneration: 9}, want: "previous-turn"},
		{name: "unknown generation", event: agentctl.AgentEvent{PromptGeneration: 8}},
		{name: "submission", event: agentctl.AgentEvent{DeliverySubmissionID: "submission"}, want: "current-turn"},
		{name: "pending legacy dispatch", pending: true, want: "current-turn"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			execution := &AgentExecution{SessionID: "session", promptTurnID: "current-turn", promptGeneration: 10,
				promptTurnIDs: map[uint64]string{9: "previous-turn"}}
			execution.dispatchedPromptPending.Store(tc.pending)
			stored := durableAgentEvent(execution, tc.event)
			var payload agentctl.AgentEvent
			require.NoError(t, json.Unmarshal(stored.Payload, &payload))
			require.Equal(t, tc.want, payload.TurnID)
		})
	}
}

func TestNativeRepliesProjectSeparatelyAcrossCompletedHumanPrompts(t *testing.T) {
	for _, protocolIDs := range []bool{false, true} {
		t.Run(fmt.Sprintf("protocol_ids_%t", protocolIDs), func(t *testing.T) {
			testNativeReplyProjection(t, protocolIDs)
		})
	}
}

func testNativeReplyProjection(t *testing.T, protocolIDs bool) {
	t.Helper()
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "delivery.db")
	repo, database := openRetentionLifecycleRepository(t, path)
	retained := openRetentionLifecycleJournal(t, filepath.Join(t.TempDir(), "journal.bbolt"))
	seed := seedRetentionIntegrationOutput(t, repo, database, retained)
	execution := &AgentExecution{ID: seed.ExecutionID, TaskID: seed.TaskID, SessionID: seed.SessionID,
		DeliveryIncarnationID: seed.IncarnationID, DeliveryHarnessGeneration: uint64(seed.HarnessGeneration),
		DeliveryStreamID: seed.StreamID}
	messages, err := repo.ListMessages(ctx, seed.SessionID)
	require.NoError(t, err)
	require.NoError(t, repo.CompleteTurn(ctx, messages[0].TurnID))
	var admitted []*models.AgentDeliveryEvent
	for reply := range 2 {
		human := fmt.Sprintf("human-%d", reply)
		native := fmt.Sprintf("native-%d", reply)
		require.NoError(t, repo.CreateTurn(ctx, &models.Turn{ID: human, TaskSessionID: seed.SessionID, TaskID: seed.TaskID}))
		require.NoError(t, repo.CompleteTurn(ctx, human))
		execution.setPromptTurnID(human)
		require.NoError(t, repo.CreateTurn(ctx, &models.Turn{ID: native, TaskSessionID: seed.SessionID, TaskID: seed.TaskID}))
		for range 2 {
			source := agentctl.AgentEvent{Type: "message_chunk", Text: native,
				DeliveryStreamID: seed.StreamID, DeliverySequence: uint64(len(admitted) + 2)}
			if protocolIDs {
				source.ProtocolMessageID = "journaled-" + native
			}
			event := durableAgentEvent(execution, source)
			var payload agentctl.AgentEvent
			require.NoError(t, json.Unmarshal(event.Payload, &payload))
			require.Empty(t, payload.TurnID)
			_, err := repo.ReceiveAgentDeliveryEvent(ctx, event, event.Sequence)
			require.NoError(t, err)
			_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, event, nil)
			require.NoError(t, err)
			admitted = append(admitted, event)
		}
		require.NoError(t, repo.CompleteTurn(ctx, native))
	}
	require.NoError(t, database.Close())
	repo, _ = openRetentionLifecycleRepository(t, path)
	for _, event := range admitted {
		_, err := repo.ReceiveAgentDeliveryEvent(ctx, event, event.Sequence)
		require.NoError(t, err)
		_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, event, nil)
		require.NoError(t, err)
	}
	messages, err = repo.ListMessages(ctx, seed.SessionID)
	require.NoError(t, err)
	require.Len(t, messages, 3, "separate native replies must not append to one historical row")
	for reply := range 2 {
		want := fmt.Sprintf("native-%d", reply)
		var found bool
		for _, message := range messages {
			if message.Content == want+want {
				require.Equal(t, want, message.TurnID)
				found = true
			}
		}
		require.True(t, found, "missing native reply %d", reply)
	}
}

func TestNativeMessageWithoutAnyGroupingIdentityUsesReplaySequence(t *testing.T) {
	execution := &AgentExecution{SessionID: "session", DeliveryStreamID: "stream"}
	first := agentctl.AgentEvent{Type: "message_chunk", DeliveryStreamID: "stream", DeliverySequence: 1}
	second := first
	second.DeliverySequence = 2
	require.NotEqual(t, canonicalAgentMessageID(execution, first), canonicalAgentMessageID(execution, second))
	require.Equal(t, canonicalAgentMessageID(execution, first), canonicalAgentMessageID(nil, first))
}

func TestCanonicalAgentMessageIDPreservesNativeAndReasoningIdentity(t *testing.T) {
	execution := &AgentExecution{SessionID: "session", DeliveryStreamID: "stream"}
	nativeFirst := agentctl.AgentEvent{
		Type: "message_chunk", ProtocolMessageID: "native-message", DeliverySequence: 1,
	}
	nativeLater := nativeFirst
	nativeLater.DeliverySequence = 17
	require.Equal(t, canonicalAgentMessageID(execution, nativeFirst), canonicalAgentMessageID(execution, nativeLater),
		"native protocol message identity is independent of delivery sequence")

	reasoning := nativeFirst
	reasoning.Type = "reasoning"
	require.NotEqual(t, canonicalAgentMessageID(execution, nativeFirst), canonicalAgentMessageID(execution, reasoning),
		"reasoning retains a separate canonical identity from assistant text")

	replayed := nativeFirst
	replayed.CanonicalMessageBaseID = "stable-base"
	replayed.CanonicalMessageID = "agent-delivery-segment-already-resolved"
	require.Equal(t, "stable-base", canonicalAgentMessageID(execution, replayed),
		"retry and reconnect derive from the stable base instead of hashing a resolved ID")
}
