package lifecycle

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestIncrementalShellCommittedProjectionCallback(t *testing.T) {
	ctx := context.Background()
	repo, _ := openRetentionLifecycleRepository(t, filepath.Join(t.TempDir(), "fixture.db"))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", Title: "fixture"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task", QueueIncarnationID: "owner"}))
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "turn", TaskID: "task", TaskSessionID: "session"}))
	require.NoError(t, repo.CreateMessage(ctx, &models.Message{ID: "tool-message", TaskID: "task", TaskSessionID: "session", TurnID: "turn", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeToolExecute, Metadata: map[string]any{"tool_call_id": "tool", "normalized": streams.NewShellExec("cmd", "", "", 0, false)}}))
	patch, err := streams.EncodeShellOutputPatch(nil, streams.ShellExecOutput{Stdout: "live"}, "epoch", true)
	require.NoError(t, err)
	raw, err := json.Marshal(streams.AgentEvent{Type: streams.EventTypeShellOutputUpdate, ToolCallID: "tool", ToolStatus: "in_progress", ShellOutput: patch, NormalizedPayload: streams.NewShellExec("cmd", "", "", 0, false)})
	require.NoError(t, err)
	event := &models.AgentDeliveryEvent{SessionID: "session", IncarnationID: "owner", HarnessGeneration: 1, StreamID: "stream", Sequence: 1, EventType: streams.EventTypeShellOutputUpdate, Payload: raw}
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, event, 1)
	require.NoError(t, err)
	var callback agentctl.AgentEvent
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{OnAgentEvent: func(_ *AgentExecution, event agentctl.AgentEvent) { callback = event }}, nil, nil)
	execution := &AgentExecution{SessionID: "session", DeliveryMode: DurableDeliveryV1, DeliveryStreamID: "stream", DeliveryIncarnationID: "owner", DeliveryHarnessGeneration: 1}
	require.NoError(t, sm.projectReceivedDeliveryEvent(ctx, execution, repo, event))
	require.True(t, callback.CanonicalProjection)
	require.Equal(t, streams.EventTypeToolUpdate, callback.Type)
	require.Equal(t, "tool-message", callback.CanonicalMessageID)
	message, err := repo.GetMessage(ctx, "tool-message")
	require.NoError(t, err)
	require.NoError(t, repo.RehydrateMessagePayload(ctx, message))
	output, ok := models.ExtractShellExecOutput(message.Metadata)
	require.True(t, ok)
	require.Equal(t, "live", output.Stdout)
}

func TestIncrementalShellToolStartDoesNotDependOnSubscriber(t *testing.T) {
	ctx := context.Background()
	repo, _ := openRetentionLifecycleRepository(t, filepath.Join(t.TempDir(), "fixture.db"))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", Title: "fixture"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task", QueueIncarnationID: "owner"}))
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "turn", TaskID: "task", TaskSessionID: "session"}))
	var callback agentctl.AgentEvent
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{OnAgentEvent: func(_ *AgentExecution, event agentctl.AgentEvent) { callback = event }}, nil, nil)
	execution := &AgentExecution{SessionID: "session", DeliveryMode: DurableDeliveryV1, DeliveryStreamID: "stream", DeliveryIncarnationID: "owner", DeliveryHarnessGeneration: 1}
	start := streams.AgentEvent{Type: streams.EventTypeToolCall, ToolCallID: "tool", ToolStatus: "pending", ToolTitle: "cmd", TurnID: "turn", NormalizedPayload: streams.NewShellExec("cmd", "", "", 0, false)}
	raw, err := json.Marshal(start)
	require.NoError(t, err)
	first := &models.AgentDeliveryEvent{SessionID: "session", IncarnationID: "owner", HarnessGeneration: 1, StreamID: "stream", Sequence: 1, EventType: streams.EventTypeToolCall, Payload: raw}
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, first, 1)
	require.NoError(t, err)
	require.NoError(t, sm.projectReceivedDeliveryEvent(ctx, execution, repo, first))
	require.True(t, callback.CanonicalProjection, "a queued callback must already have a canonical tool card")
	message, err := repo.GetMessageByToolCallID(ctx, "session", "tool")
	require.NoError(t, err)
	require.Equal(t, message.ID, callback.CanonicalMessageID)
	patch, err := streams.EncodeShellOutputPatch(nil, streams.ShellExecOutput{Stdout: "output"}, "epoch", true)
	require.NoError(t, err)
	update := start
	update.Type, update.ToolStatus, update.ShellOutput = streams.EventTypeShellOutputUpdate, "in_progress", patch
	raw, err = json.Marshal(update)
	require.NoError(t, err)
	second := *first
	second.Sequence, second.EventType, second.Payload = 2, streams.EventTypeShellOutputUpdate, raw
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, &second, 2)
	require.NoError(t, err)
	require.NoError(t, sm.projectReceivedDeliveryEvent(ctx, execution, repo, &second))
	require.Equal(t, message.ID, callback.CanonicalMessageID)
	require.Equal(t, streams.EventTypeToolUpdate, callback.Type)
	message, err = repo.GetMessage(ctx, message.ID)
	require.NoError(t, err)
	require.NoError(t, repo.RehydrateMessagePayload(ctx, message))
	output, ok := models.ExtractShellExecOutput(message.Metadata)
	require.True(t, ok)
	require.Equal(t, "output", output.Stdout)
}
