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

func TestResumedDeliveryCheckpointReceivesAuthenticatedFirstEvent(t *testing.T) {
	for _, scenario := range []string{"unreceived", "acknowledged", "pruned", "foreign_native", "foreign_execution", "foreign_event", "missing_first", "terminal_envelope", "conflicting_type", "message_chunk", "tool_call", "owner_replaced_during_replay", "orphan_receipt"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			repo, database := runOwnerTestRepository(t)
			require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", Title: "Resumed delivery"}))
			require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task", QueueIncarnationID: "incarnation"}))
			require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: "running", TaskID: "task", SessionID: "session", AgentExecutionID: "execution"}))
			require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{SessionID: "session", IncarnationID: "incarnation", Generation: 1, NativeSessionID: "native", CreationReason: "native_started"}))
			execution := &AgentExecution{ID: "execution", TaskID: "task", SessionID: "session", ACPSessionID: "native", DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 1, DeliveryStreamID: "incarnation:g1"}
			execution.deliveryBootstrap.Store(true)
			descriptor := agentctl.DeliveryStatus{StorageCapability: journal.StorageCapability{Version: journal.CurrentVersion, Durable: true}, SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "incarnation:g1",
				Stream: &journal.Stream{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "incarnation:g1", HighWater: 2, FirstRetained: 1}, SubmissionCount: 1}
			descriptor.Unresolved = true
			first := journal.Event{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "incarnation:g1", Sequence: 1, Type: "agent_capabilities", Payload: []byte(`{"type":"agent_capabilities"}`)}
			switch scenario {
			case "acknowledged":
				descriptor.Stream.Acknowledged = 1
			case "pruned":
				descriptor.Stream.FirstRetained = 2
			case "foreign_native":
				execution.ACPSessionID = "other-native"
			case "foreign_execution":
				execution.ID = "other-execution"
			case "foreign_event":
				first.IncarnationID = "other-incarnation"
			case "missing_first":
				first.Sequence = 2
			case "terminal_envelope":
				first.Terminal = true
			case "conflicting_type":
				first.Type = "complete"
			case "message_chunk", "tool_call":
				first.Type = scenario
				first.Payload = []byte(`{"type":"` + scenario + `"}`)
			case "orphan_receipt":
				_, err := repo.ReceiveAgentDeliveryEvent(ctx, &models.AgentDeliveryEvent{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "incarnation:g1", Sequence: 2, EventType: "agent_capabilities", Payload: first.Payload}, 2)
				require.NoError(t, err)
				_, err = database.Exec("DELETE FROM agent_delivery_cursors")
				require.NoError(t, err)
			}
			client := ackTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method, "checkpoint recovery must not send a prompt or acknowledgement")
				if r.URL.Path == "/api/v1/agent/delivery" {
					require.NoError(t, json.NewEncoder(w).Encode(descriptor))
					return
				}
				require.Equal(t, "/api/v1/agent/delivery/stream", r.URL.Path)
				if scenario == "owner_replaced_during_replay" {
					require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: "running", TaskID: "task", SessionID: "session", AgentExecutionID: "replacement"}))
				}
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"events": []journal.Event{first}, "stream": descriptor.Stream}))
			})
			t.Cleanup(client.Close)
			sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
			t.Cleanup(sm.Wait)
			after, err := sm.initialDeliveryReplayCursor(ctx, execution, repo, client, execution.DeliveryStreamID)
			if scenario != "unreceived" {
				require.Error(t, err)
				_, cursorErr := repo.GetAgentDeliveryCursor(ctx, execution.DeliveryStreamID)
				require.Error(t, cursorErr, "invalid evidence must not manufacture a cursor")
				return
			}
			require.NoError(t, err, "verified unreceived history must remain recoverable after bootstrap failure")
			require.Zero(t, after)
			cursor, err := repo.GetAgentDeliveryCursor(ctx, execution.DeliveryStreamID)
			require.NoError(t, err)
			require.EqualValues(t, 1, cursor.ReceivedSequence, "checkpoint must accompany a real receipt")
			require.Zero(t, cursor.ProjectedSequence, "preflight cannot claim projection")
			execution.DeliveryMode = DurableDeliveryV1
			require.NoError(t, sm.processRecoveredDeliveryEvent(ctx, execution, client, repo, first, 0, true))
			cursor, err = repo.GetAgentDeliveryCursor(ctx, execution.DeliveryStreamID)
			require.NoError(t, err)
			require.EqualValues(t, 1, cursor.ProjectedSequence)
			tail := first
			tail.Sequence, tail.Type = 2, "message_chunk"
			tail.Payload = []byte(`{"type":"message_chunk","text":"retained output"}`)
			require.NoError(t, sm.processRecoveredDeliveryEvent(ctx, execution, client, repo, tail, 0, true))
			messages, err := repo.ListMessages(ctx, execution.SessionID)
			require.NoError(t, err)
			require.Len(t, messages, 1)
			require.Equal(t, "retained output", messages[0].Content)
		})
	}
}
