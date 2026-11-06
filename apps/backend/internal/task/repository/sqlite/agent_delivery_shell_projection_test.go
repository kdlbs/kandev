package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-INCREMENTAL-SHELL-001.3, AC-PLATFORM-INCREMENTAL-SHELL-001.4
func TestIncrementalShellDetailSnapshot(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedIncrementalShellMessage(t, repo)
	output := streams.ShellExecOutput{Stdout: strings.Repeat("a", 8192)}
	patch, err := streams.EncodeShellOutputPatch(nil, output, "epoch", true)
	require.NoError(t, err)
	event := incrementalShellTestEvent(t, 1, patch)
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, event, 1)
	require.NoError(t, err)
	_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, event, completedRetentionEventEffect(event))
	require.NoError(t, err)
	message, err := repo.GetMessage(ctx, "shell-message")
	require.NoError(t, err)
	require.NoError(t, repo.RehydrateMessagePayload(ctx, message))
	snapshot, ok := models.ExtractShellExecOutput(message.Metadata)
	require.True(t, ok)
	require.Equal(t, output.Stdout, snapshot.Stdout)
	var blobs int
	require.NoError(t, repo.db.GetContext(ctx, &blobs, `SELECT COUNT(*) FROM task_message_payloads`))
	require.Zero(t, blobs, "live compact updates must not externalize whole-tail blobs")
	state, err := streams.ApplyShellOutputPatch(nil, patch)
	require.NoError(t, err)
	output.Stdout = output.Stdout[16:] + "new output bytes"
	second, err := streams.EncodeShellOutputPatch(state, output, "epoch", false)
	require.NoError(t, err)
	event = incrementalShellTestEvent(t, 2, second)
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, event, 2)
	require.NoError(t, err)
	for range 2 {
		_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, event, completedRetentionEventEffect(event))
		require.NoError(t, err)
	}
	message, err = repo.GetMessage(ctx, "shell-message")
	require.NoError(t, err)
	require.NoError(t, repo.RehydrateMessagePayload(ctx, message))
	snapshot, ok = models.ExtractShellExecOutput(message.Metadata)
	require.True(t, ok)
	require.Equal(t, output.Stdout, snapshot.Stdout)
	cursor, err := repo.GetAgentDeliveryCursor(ctx, event.StreamID)
	require.NoError(t, err)
	require.EqualValues(t, 2, cursor.ProjectedSequence)
}

func incrementalShellTestEvent(t *testing.T, sequence int64, patch *streams.ShellOutputPatch) *models.AgentDeliveryEvent {
	t.Helper()
	payload, err := json.Marshal(streams.AgentEvent{Type: streams.EventTypeShellOutputUpdate, ToolCallID: "tool", ToolStatus: "in_progress", ShellOutput: patch, NormalizedPayload: streams.NewShellExec("test-command", "/tmp", "", 0, false)})
	require.NoError(t, err)
	return &models.AgentDeliveryEvent{SessionID: "shell-session", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "shell-stream", Sequence: sequence, EventType: streams.EventTypeShellOutputUpdate, Payload: payload}
}

func TestIncrementalShellProjectionCrashReplay(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedIncrementalShellMessage(t, repo)
	patch, err := streams.EncodeShellOutputPatch(nil, streams.ShellExecOutput{Stdout: "accepted before crash"}, "epoch", true)
	require.NoError(t, err)
	event := incrementalShellTestEvent(t, 1, patch)
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, event, 1)
	require.NoError(t, err)
	_, err = repo.db.Exec(`CREATE TRIGGER fail_shell_projection BEFORE UPDATE OF metadata ON task_session_messages BEGIN SELECT RAISE(ABORT,'injected shell commit failure'); END`)
	require.NoError(t, err)
	_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, event, completedRetentionEventEffect(event))
	require.Error(t, err)
	var heads int
	require.NoError(t, repo.db.GetContext(ctx, &heads, `SELECT COUNT(*) FROM task_message_shell_output`))
	require.Zero(t, heads)
	cursor, err := repo.GetAgentDeliveryCursor(ctx, event.StreamID)
	require.NoError(t, err)
	require.Zero(t, cursor.ProjectedSequence)
	_, err = repo.db.Exec(`DROP TRIGGER fail_shell_projection`)
	require.NoError(t, err)
	for range 2 {
		_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, event, completedRetentionEventEffect(event))
		require.NoError(t, err)
	}
	state, err := streams.ApplyShellOutputPatch(nil, patch)
	require.NoError(t, err)
	next, err := streams.EncodeShellOutputPatch(state, streams.ShellExecOutput{Stdout: "accepted before crash and after"}, "epoch", false)
	require.NoError(t, err)
	second := incrementalShellTestEvent(t, 2, next)
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, second, 2)
	require.NoError(t, err)
	_, err = repo.db.Exec(`DELETE FROM task_message_shell_output WHERE message_id='shell-message'`)
	require.NoError(t, err)
	_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, second, completedRetentionEventEffect(second))
	require.Error(t, err)
	cursor, err = repo.GetAgentDeliveryCursor(ctx, event.StreamID)
	require.NoError(t, err)
	require.EqualValues(t, 1, cursor.ProjectedSequence)
}

func seedIncrementalShellMessage(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	seedAgentDeliveryTestSession(t, repo, "shell-session", "incarnation")
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "shell-turn", TaskID: "delivery-fixture-task-shell-session", TaskSessionID: "shell-session"}))
	require.NoError(t, repo.CreateMessage(ctx, &models.Message{ID: "shell-message", TurnID: "shell-turn", TaskID: "delivery-fixture-task-shell-session", TaskSessionID: "shell-session", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeToolExecute, Metadata: map[string]any{"tool_call_id": "tool", "normalized": streams.NewShellExec("test-command", "/tmp", "", 0, false), "status": "running"}}))
}

func TestIncrementalShellRetentionPrunesProjectedBodies(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedIncrementalShellMessage(t, repo)
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	repo.clockNow = func() time.Time { return now }
	patch, err := streams.EncodeShellOutputPatch(nil, streams.ShellExecOutput{Stdout: "retained after receipt pruning"}, "epoch", true)
	require.NoError(t, err)
	event := incrementalShellTestEvent(t, 1, patch)
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, event, 1)
	require.NoError(t, err)
	_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, event, completedRetentionEventEffect(event))
	require.NoError(t, err)
	now = now.Add(2 * time.Hour)
	result, err := repo.RunAgentDeliveryRetentionBatch(ctx, models.AgentDeliveryRetentionBatchOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, result.EventPayloadsPruned)
	message, err := repo.GetMessage(ctx, "shell-message")
	require.NoError(t, err)
	require.NoError(t, repo.RehydrateMessagePayload(ctx, message))
	output, ok := models.ExtractShellExecOutput(message.Metadata)
	require.True(t, ok)
	require.Equal(t, "retained after receipt pruning", output.Stdout)
	cold := NewWithInitializedDB(repo.db, repo.db, nil)
	_, err = cold.ReceiveAgentDeliveryEvent(ctx, event, 1)
	require.NoError(t, err, "lost ACK after body pruning still uses the original receipt hash")
	_, err = cold.ProjectCanonicalAgentDeliveryEvent(ctx, event, completedRetentionEventEffect(event))
	require.NoError(t, err)
	changed := *event
	changed.Payload = []byte(`{"changed":"duplicate"}`)
	_, err = cold.ReceiveAgentDeliveryEvent(ctx, &changed, 1)
	require.Error(t, err)
}

func TestIncrementalShellRemovedPayloadCannotResurrect(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedIncrementalShellMessage(t, repo)
	patch, err := streams.EncodeShellOutputPatch(nil, streams.ShellExecOutput{Stdout: strings.Repeat("x", 8192)}, "epoch", true)
	require.NoError(t, err)
	first := incrementalShellTestEvent(t, 1, patch)
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, first, 1)
	require.NoError(t, err)
	_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, first, completedRetentionEventEffect(first))
	require.NoError(t, err)
	message, err := repo.GetMessage(ctx, "shell-message")
	require.NoError(t, err)
	raw, err := json.Marshal(message.Metadata)
	require.NoError(t, err)
	reduced, err := models.ReduceToolPayload("tool_execute", raw, time.Now().UTC())
	require.NoError(t, err)
	require.Empty(t, reduced.Reason)
	_, err = repo.db.Exec(`UPDATE task_session_messages SET metadata=? WHERE id='shell-message'`, string(reduced.Metadata))
	require.NoError(t, err)
	_, err = repo.db.Exec(`DELETE FROM task_message_shell_output WHERE message_id='shell-message'`)
	require.NoError(t, err)
	state, err := streams.ApplyShellOutputPatch(nil, patch)
	require.NoError(t, err)
	patch, err = streams.EncodeShellOutputPatch(state, streams.ShellExecOutput{Stdout: "late terminal", ExitCode: nil}, "epoch", true)
	require.NoError(t, err)
	last := incrementalShellTestEvent(t, 2, patch)
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, last, 2)
	require.NoError(t, err)
	_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, last, completedRetentionEventEffect(last))
	require.NoError(t, err)
	message, err = repo.GetMessage(ctx, "shell-message")
	require.NoError(t, err)
	require.True(t, models.ToolPayloadRemoved(message.Metadata))
	var count int
	require.NoError(t, repo.db.Get(&count, `SELECT COUNT(*) FROM task_message_shell_output_chunks`))
	require.Zero(t, count)
}

func TestIncrementalShellProjectionUpdatesTitleAndContent(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedIncrementalShellMessage(t, repo)
	_, err := repo.db.Exec(`UPDATE task_session_messages SET content='initial command' WHERE id='shell-message'`)
	require.NoError(t, err)
	var state *streams.ShellOutputState
	expected := "initial command"
	for index, title := range []string{"updated command", "renamed while quiet", ""} {
		patch, encodeErr := streams.EncodeShellOutputPatch(state, streams.ShellExecOutput{Stdout: "output"}, "epoch", true)
		require.NoError(t, encodeErr)
		if state != nil {
			patch.Checkpoint, patch.Stdout, patch.Stderr = false, nil, nil
		}
		state, err = streams.ApplyShellOutputPatch(state, patch)
		require.NoError(t, err)
		event := incrementalShellTestEvent(t, int64(index+1), patch)
		var agent streams.AgentEvent
		require.NoError(t, json.Unmarshal(event.Payload, &agent))
		agent.ToolTitle = title
		event.Payload, err = json.Marshal(agent)
		require.NoError(t, err)
		_, err = repo.ReceiveAgentDeliveryEvent(ctx, event, event.Sequence)
		require.NoError(t, err)
		_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, event, completedRetentionEventEffect(event))
		require.NoError(t, err)
		message, readErr := repo.GetMessage(ctx, "shell-message")
		require.NoError(t, readErr)
		if title != "" {
			expected = title
		}
		require.Equal(t, expected, message.Content)
		require.Equal(t, expected, message.Metadata["title"])
	}
}

func TestIncrementalShellOrdinaryReplacementRetiresChunks(t *testing.T) {
	for _, receipt := range []bool{false, true} {
		for _, stdout := range []string{"background result", strings.Repeat("B", 8192)} {
			t.Run(fmt.Sprintf("receipt_%t_bytes_%d", receipt, len(stdout)), func(t *testing.T) {
				repo := newRepoForSessionTests(t)
				ctx := context.Background()
				seedIncrementalShellMessage(t, repo)
				patch, err := streams.EncodeShellOutputPatch(nil, streams.ShellExecOutput{Stdout: "foreground"}, "epoch", true)
				require.NoError(t, err)
				event := incrementalShellTestEvent(t, 1, patch)
				_, err = repo.ReceiveAgentDeliveryEvent(ctx, event, 1)
				require.NoError(t, err)
				_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, event, completedRetentionEventEffect(event))
				require.NoError(t, err)
				message, err := repo.GetMessage(ctx, "shell-message")
				require.NoError(t, err)
				write := func() error {
					if receipt {
						_, writeErr := repo.UpdateMessageWithConversationReceipt(ctx, message)
						return writeErr
					}
					return repo.UpdateMessage(ctx, message)
				}
				normalized := streams.NewShellExec("test-command", "/tmp", "", 0, true)
				normalized.ShellExec().Output = &streams.ShellExecOutput{Stdout: stdout}
				message.Metadata["normalized"] = normalized
				_, err = repo.db.Exec(`CREATE TRIGGER fail_shell_replacement BEFORE UPDATE OF metadata ON task_session_messages BEGIN SELECT RAISE(ABORT,'injected replacement failure'); END`)
				require.NoError(t, err)
				require.Error(t, write())
				var retainedHeads int
				require.NoError(t, repo.db.Get(&retainedHeads, `SELECT COUNT(*) FROM task_message_shell_output`))
				require.Equal(t, 1, retainedHeads, "a failed replacement must retain its canonical baseline")
				_, err = repo.db.Exec(`DROP TRIGGER fail_shell_replacement`)
				require.NoError(t, err)
				require.NoError(t, write())
				message, err = repo.GetMessage(ctx, "shell-message")
				require.NoError(t, err)
				require.NoError(t, repo.RehydrateMessagePayload(ctx, message))
				output, ok := models.ExtractShellExecOutput(message.Metadata)
				require.True(t, ok)
				require.True(t, stdout == output.Stdout, "ordinary replacement must own the detail body")
				require.NotContains(t, message.Metadata, models.ShellOutputStorageKey)
				var heads int
				require.NoError(t, repo.db.Get(&heads, `SELECT COUNT(*) FROM task_message_shell_output`))
				require.Zero(t, heads)
			})
		}
	}
}

func TestIncrementalShellProjectedMetadataUpdateKeepsChunks(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedIncrementalShellMessage(t, repo)
	patch, err := streams.EncodeShellOutputPatch(nil, streams.ShellExecOutput{Stdout: "retained"}, "epoch", true)
	require.NoError(t, err)
	event := incrementalShellTestEvent(t, 1, patch)
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, event, 1)
	require.NoError(t, err)
	_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, event, completedRetentionEventEffect(event))
	require.NoError(t, err)
	message, err := repo.GetMessage(ctx, "shell-message")
	require.NoError(t, err)
	message.Metadata = models.ProjectMessageMetadata(message.Metadata)
	message.Metadata["status"] = "complete"
	_, err = repo.UpdateMessageWithConversationReceipt(ctx, message)
	require.NoError(t, err)
	message, err = repo.GetMessage(ctx, "shell-message")
	require.NoError(t, err)
	require.NoError(t, repo.RehydrateMessagePayload(ctx, message))
	output, ok := models.ExtractShellExecOutput(message.Metadata)
	require.True(t, ok)
	require.Equal(t, "retained", output.Stdout)
}
