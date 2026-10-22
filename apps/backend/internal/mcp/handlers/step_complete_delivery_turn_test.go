package handlers

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestHandleStepComplete_DeliveryFallbackStaleEntry(t *testing.T) {
	ctx := t.Context()
	svc, repo := newTestTaskService(t)
	seedStepCompleteTarget(t, repo, "entry-task", "entry-session", "step-a", models.TaskSessionStateRunning)
	createStepCompleteRecoveryTurn(t, repo, "entry-turn", "entry-task", "entry-session")
	for _, step := range []string{"step-b", "step-a"} {
		task, err := repo.GetTask(ctx, "entry-task")
		if err != nil {
			t.Fatal(err)
		}
		task.WorkflowStepID = step
		if err := repo.UpdateTask(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	bus := &mcpRecordingEventBus{}
	h := newStepCompleteHandler(t, svc, repo, bus)
	msg := makeWSMessage(t, ws.ActionMCPStepComplete, map[string]interface{}{"task_id": "entry-task", "session_id": "entry-session", "summary": "stale-entry signal"})
	resp, err := h.handleStepComplete(ctx, msg)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(resp.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["code"] != ws.ErrorCodeValidation {
		t.Fatalf("stale entry response = %s, want VALIDATION_ERROR; published events = %d", resp.Payload, len(bus.events))
	}
}

func TestHandleStepComplete_DeliveryFallbackTurn(t *testing.T) {
	for _, kind := range []string{"text", "thinking", "shell"} {
		for _, state := range []models.TaskSessionState{models.TaskSessionStateRunning, models.TaskSessionStateWaitingForInput} {
			t.Run(kind+"/"+string(state), func(t *testing.T) {
				svc, repo := newTestTaskService(t)
				seedStepCompleteTarget(t, repo, "task", "session", "step-a", state)
				createStepCompleteRecoveryTurn(t, repo, "predecessor", "task", "session")
				require.NoError(t, repo.CompleteTurn(t.Context(), "predecessor"))
				_, err := repo.DB().Exec(`UPDATE task_sessions SET queue_incarnation_id='incarnation' WHERE id='session'`)
				require.NoError(t, err)
				agent := streams.AgentEvent{Type: streams.EventTypeMessageChunk, Text: "awake", CanonicalMessageID: "message"}
				if kind == "thinking" {
					agent.Type = streams.EventTypeReasoning
					agent.ReasoningText = "awake reasoning"
				}
				if kind == "shell" {
					agent = streams.AgentEvent{Type: streams.EventTypeToolCall, ToolCallID: "tool", ToolTitle: "command", ToolStatus: "pending", NormalizedPayload: streams.NewShellExec("cmd", "/tmp", "", 0, false)}
				}
				raw, err := json.Marshal(agent)
				require.NoError(t, err)
				event := &models.AgentDeliveryEvent{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream", Sequence: 1, EventType: agent.Type, Payload: raw}
				_, err = repo.ReceiveAgentDeliveryEvent(t.Context(), event, 1)
				require.NoError(t, err)
				effect := &models.AgentDeliveryEffect{EffectKey: "event", StreamID: "stream", Sequence: 1, EffectType: "agent_delivery.event"}
				_, err = repo.ProjectCanonicalAgentDeliveryEvent(t.Context(), event, effect)
				require.NoError(t, err)
				bus := &mcpRecordingEventBus{}
				h := newStepCompleteHandler(t, svc, repo, bus)
				msg := makeWSMessage(t, ws.ActionMCPStepComplete, map[string]interface{}{"task_id": "task", "session_id": "session", "summary": "complete"})
				response, err := h.handleStepComplete(t.Context(), msg)
				require.NoError(t, err)
				var payload map[string]interface{}
				require.NoError(t, json.Unmarshal(response.Payload, &payload))
				require.Equal(t, true, payload["accepted"])
				session, err := repo.GetTaskSession(t.Context(), "session")
				require.NoError(t, err)
				signal, ok := models.LoadPendingStepSignal(session.Metadata)
				require.True(t, ok)
				require.NotNil(t, signal.WorkflowStepTransitionID)
				_, _, entry, err := repo.GetTaskWorkflowStepEntry(t.Context(), "task")
				require.NoError(t, err)
				require.Equal(t, entry, *signal.WorkflowStepTransitionID)
				turns, err := repo.ListTurnsBySession(t.Context(), "session")
				require.NoError(t, err)
				require.Len(t, turns, 2)
				require.Equal(t, "agent-delivery-turn-stream-1", turns[1].ID)
				task, err := repo.GetTask(t.Context(), "task")
				require.NoError(t, err)
				for _, step := range []string{"step-b", "step-a"} {
					task.WorkflowStepID = step
					require.NoError(t, repo.UpdateTask(t.Context(), task))
				}
				response, err = h.handleStepComplete(t.Context(), msg)
				require.NoError(t, err)
				assertWSError(t, response, ws.ErrorCodeValidation)
				require.Len(t, bus.events, 1, "stale entry must not emit an additional signal")
			})
		}
	}
}
