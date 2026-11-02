package sqlite

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestPostgresMessageReplacementKeepsExistingBehavior(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	ctx := context.Background()
	seedPostgresTask(t, repo, "retention-pg")
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "retention-session-pg", TaskID: "retention-pg"}))
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "retention-turn-pg", TaskSessionID: "retention-session-pg", TaskID: "retention-pg"}))
	message := &models.Message{ID: "retention-message-pg", TaskID: "retention-pg", TaskSessionID: "retention-session-pg", TurnID: "retention-turn-pg", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeToolExecute, Metadata: map[string]any{"payload_retention": map[string]any{"version": 1}}}
	require.NoError(t, repo.CreateMessage(ctx, message))
	message.Metadata = map[string]any{"result": "replacement"}
	require.NoError(t, repo.UpdateMessage(ctx, message))
	stored, err := repo.GetMessage(ctx, message.ID)
	require.NoError(t, err)
	require.Equal(t, "replacement", stored.Metadata["result"])
	require.False(t, models.ToolPayloadRemoved(stored.Metadata))
}

func TestPostgresIncrementalMessageReplacementPreservesRemoval(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	seedIncrementalShellMessage(t, repo)
	ctx := context.Background()
	patch, err := streams.EncodeShellOutputPatch(nil, streams.ShellExecOutput{Stdout: "retained"}, "epoch", true)
	require.NoError(t, err)
	event := incrementalShellTestEvent(t, 1, patch)
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, event, 1)
	require.NoError(t, err)
	_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, event, completedRetentionEventEffect(event))
	require.NoError(t, err)
	message, err := repo.GetMessage(ctx, "shell-message")
	require.NoError(t, err)
	raw, err := json.Marshal(message.Metadata)
	require.NoError(t, err)
	reduced, err := models.ReduceToolPayload("tool_execute", raw, time.Now().UTC())
	require.NoError(t, err)
	require.Empty(t, reduced.Reason)
	_, err = db.Exec(`UPDATE task_session_messages SET metadata=$1 WHERE id='shell-message'`, string(reduced.Metadata))
	require.NoError(t, err)
	_, err = db.Exec(`DELETE FROM task_message_shell_output WHERE message_id='shell-message'`)
	require.NoError(t, err)
	message.Metadata = models.ProjectMessageMetadata(message.Metadata)
	normalized := streams.NewShellExec("cmd", "", "", 0, false)
	normalized.ShellExec().Output = &streams.ShellExecOutput{Stdout: "resurrected"}
	message.Metadata["normalized"] = normalized
	require.NoError(t, repo.UpdateMessage(ctx, message))
	stored, err := repo.GetMessage(ctx, message.ID)
	require.NoError(t, err)
	require.True(t, models.ToolPayloadRemoved(stored.Metadata))
	_, body := models.ExtractShellExecOutput(stored.Metadata)
	require.False(t, body)
}
