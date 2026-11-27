package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.3
func TestDeliveryPreflightProjectsRealSQLInboxBeforeExpiredReplay(t *testing.T) {
	ctx := context.Background()
	repo, _ := runOwnerTestRepository(t)
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", Title: "Inbox replay"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task", QueueIncarnationID: "session"}))
	for index, kind := range []string{"message_chunk", "reasoning", "tool_call", "tool_update", "context_window", "context_window"} {
		payload, err := json.Marshal(agentctl.AgentEvent{Type: kind, Text: "saved " + kind, ReasoningText: "saved " + kind})
		require.NoError(t, err)
		_, err = repo.ReceiveAgentDeliveryEvent(ctx, &models.AgentDeliveryEvent{SessionID: "session", IncarnationID: "session", HarnessGeneration: 1, StreamID: "session:g1", Sequence: int64(index + 1), EventType: kind, Payload: payload}, 6)
		require.NoError(t, err)
	}
	client := ackTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/agent/delivery", r.URL.Path, "inbox projection must not send acknowledgments or prompts")
		require.Equal(t, "session:g1", r.URL.Query().Get("stream_id"))
		require.NoError(t, json.NewEncoder(w).Encode(agentctl.DeliveryStatus{SessionID: "session", IncarnationID: "session", HarnessGeneration: 1, StreamID: "session:g1", StorageCapability: journal.StorageCapability{Durable: true, Version: journal.CurrentVersion}, Stream: &journal.Stream{SessionID: "session", IncarnationID: "session", HarnessGeneration: 1, StreamID: "session:g1", HighWater: 6, Acknowledged: 6, FirstRetained: 7}}))
	})
	execution := &AgentExecution{SessionID: "session", TaskID: "task", DeliveryMode: DurableDeliveryV1, DeliveryIncarnationID: "session", DeliveryHarnessGeneration: 1, DeliveryStreamID: "session:g1"}
	var callbacks []string
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{OnAgentEvent: func(_ *AgentExecution, event agentctl.AgentEvent) { callbacks = append(callbacks, event.Type) }}, nil, nil)
	t.Cleanup(sm.Wait)
	after, err := sm.initialDeliveryReplayCursor(ctx, execution, repo, client, "session:g1")
	require.NoError(t, err, "original SQL receipts must be projected instead of declaring unavailable output")
	require.EqualValues(t, 6, after)
	cursor, err := repo.GetAgentDeliveryCursor(ctx, "session:g1")
	require.NoError(t, err)
	require.EqualValues(t, 6, cursor.ReceivedSequence)
	require.EqualValues(t, 6, cursor.ProjectedSequence)
	require.Zero(t, cursor.RetiredSequence)
	messages, err := repo.ListMessages(ctx, "session")
	require.NoError(t, err)
	var contents []string
	for _, message := range messages {
		if thinking, exists := message.Metadata["thinking"].(string); exists {
			contents = append(contents, thinking)
		} else {
			contents = append(contents, message.Content)
		}
	}
	require.ElementsMatch(t, []string{"saved message_chunk", "saved reasoning"}, contents)
	require.Contains(t, callbacks, "tool_call")
	require.Contains(t, callbacks, "tool_update")
}

// @covers AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.2
// @covers AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.8
func TestDeliveryPreflightRequiresPositiveCompatibilityEvidence(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mode    DurableDeliveryMode
		status  int
		fresh   bool
		allowed bool
	}{
		{"fresh durable before negotiation", DurableDeliveryLegacy, 200, true, true},
		{"fresh negotiated durable", DurableDeliveryV1, 200, true, true},
		{"legacy endpoint absent", DurableDeliveryLegacy, 404, false, true},
		{"durable cannot downgrade", DurableDeliveryV1, 404, false, false},
		{"unknown capability", DurableDeliveryLegacy, 200, false, false},
		{"temporary failure", DurableDeliveryLegacy, 503, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, _ := runOwnerTestRepository(t)
			client := ackTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/api/v1/agent/delivery", r.URL.Path)
				require.Equal(t, "incarnation:g8", r.URL.Query().Get("stream_id"))
				w.WriteHeader(tc.status)
				if tc.fresh {
					require.NoError(t, json.NewEncoder(w).Encode(agentctl.DeliveryStatus{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 8, StreamID: "incarnation:g8", StorageCapability: journal.StorageCapability{Version: journal.CurrentVersion, Durable: true}}))
				} else {
					_, err := w.Write([]byte(`{}`))
					require.NoError(t, err)
				}
			})
			sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
			t.Cleanup(sm.Wait)
			execution := &AgentExecution{SessionID: "session", DeliveryMode: tc.mode, DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 8, DeliveryStreamID: "incarnation:g8"}
			after, err := sm.initialDeliveryReplayCursor(context.Background(), execution, repo, client, "incarnation:g8")
			if tc.allowed {
				require.NoError(t, err)
				require.Zero(t, after)
			} else {
				require.Error(t, err)
			}
		})
	}
}
