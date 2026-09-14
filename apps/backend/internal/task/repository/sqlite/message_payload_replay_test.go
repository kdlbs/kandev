package sqlite

import (
	"context"
	"encoding/json"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestUpdateMessagePreservesPayloadRemovalAfterStaleRead(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-ret", "session-ret", "turn-ret")
	message := &models.Message{ID: "ret", TaskID: "task-ret", TaskSessionID: "session-ret", TurnID: "turn-ret", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeToolExecute, Metadata: map[string]any{"tool_call_id": "tool-1", "normalized": map[string]any{"kind": "shell_exec", "shell_exec": map[string]any{"command": "ls", "output": map[string]any{"stdout": "secret"}}}}}
	require.NoError(t, repo.CreateMessage(ctx, message))
	stale, err := repo.GetMessage(ctx, message.ID)
	require.NoError(t, err)
	_, err = repo.db.ExecContext(ctx, `UPDATE task_session_messages SET metadata = ? WHERE id = ?`, `{"tool_call_id":"tool-1","future":9007199254740993,"normalized":{"kind":"shell_exec","shell_exec":{"command":"ls","output":{"exit_code":1}}},"payload_retention":{"version":1,"removed_at":"2026-09-14T00:00:00Z"}}`, message.ID)
	require.NoError(t, err)
	stale.Metadata["result"] = "secret"
	stale.Metadata["status"] = "complete"
	stale.Type = models.MessageTypeToolCall
	require.NoError(t, repo.UpdateMessage(ctx, stale))
	current, err := repo.GetMessage(ctx, message.ID)
	require.NoError(t, err)
	require.True(t, models.ToolPayloadRemoved(current.Metadata))
	require.Equal(t, models.MessageTypeToolExecute, current.Type)
	require.NotContains(t, current.Metadata, "result")
	require.Equal(t, "complete", current.Metadata["status"])
	require.True(t, models.ToolPayloadRemoved(stale.Metadata))
	require.NotContains(t, stale.Metadata, "result")
	var raw string
	require.NoError(t, repo.db.Get(&raw, `SELECT metadata FROM task_session_messages WHERE id = ?`, message.ID))
	require.Contains(t, raw, "9007199254740993")
	require.NotContains(t, raw, "secret")
	for _, projected := range []map[string]any{models.ProjectMessageMetadata(stale.Metadata), models.ProjectMessageMetadata(current.Metadata)} {
		output := projected["normalized"].(map[string]any)["shell_exec"].(map[string]any)["output"].(map[string]any)
		require.Equal(t, 1, output["exit_code"])
		require.NotContains(t, output, "stdout")
		require.NotContains(t, projected, "result")
	}
	api := stale.ToAPI()
	var projected map[string]any
	encoded, err := json.Marshal(api.Metadata)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(encoded, &projected))
	output := projected["normalized"].(map[string]any)["shell_exec"].(map[string]any)["output"].(map[string]any)
	require.Equal(t, float64(1), output["exit_code"])
	require.NotContains(t, output, "stdout")
	require.Contains(t, string(encoded), "9007199254740993")

}

func TestCreateMessageCannotReplayRemovedToolIdentity(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-ret-create", "session-ret-create", "turn-ret-create")
	message := &models.Message{ID: "removed-original", TaskID: "task-ret-create", TaskSessionID: "session-ret-create", TurnID: "turn-ret-create", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeToolExecute, Metadata: map[string]any{"tool_call_id": "same-tool", "payload_retention": map[string]any{"version": 1}}}
	require.NoError(t, repo.CreateMessage(ctx, message))
	message.ID = "replay"
	message.Metadata = map[string]any{"tool_call_id": "same-tool", "result": "secret"}
	require.Error(t, repo.CreateMessage(ctx, message))
	var count int
	require.NoError(t, repo.db.Get(&count, `SELECT COUNT(*) FROM task_session_messages WHERE task_session_id = ?`, message.TaskSessionID))
	require.Equal(t, 1, count)
	message.ID = "new-call"
	message.Metadata["tool_call_id"] = "different-tool"
	require.NoError(t, repo.CreateMessage(ctx, message))
}

func TestCreateMessageCannotReplayRemovedToolAsDifferentType(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-type", "session-type", "turn-type")
	message := &models.Message{ID: "original-type", TaskID: "task-type", TaskSessionID: "session-type", TurnID: "turn-type", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeToolExecute, Metadata: map[string]any{"tool_call_id": "same-tool", "payload_retention": map[string]any{"version": 1}}}
	require.NoError(t, repo.CreateMessage(ctx, message))
	message.ID = "retyped"
	message.Type = models.MessageTypeMessage
	message.Metadata = map[string]any{"tool_call_id": "same-tool", "result": "secret"}
	require.Error(t, repo.CreateMessage(ctx, message))
}
