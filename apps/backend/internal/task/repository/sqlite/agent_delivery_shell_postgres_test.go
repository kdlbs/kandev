package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestIncrementalShellPostgresToolStart(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	seedIncrementalShellMessage(t, repo)
	_, err = db.Exec(`DELETE FROM task_session_messages WHERE id='shell-message'`)
	require.NoError(t, err)
	agent := streams.AgentEvent{Type: streams.EventTypeToolCall, ToolCallID: "tool", ToolTitle: "command", ToolStatus: "pending", TurnID: "shell-turn", NormalizedPayload: streams.NewShellExec("cmd", "/tmp", "", 0, false)}
	event := incrementalShellTestEvent(t, 1, nil)
	event.EventType = agent.Type
	event.Payload, err = json.Marshal(agent)
	require.NoError(t, err)
	ctx := context.Background()
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, event, 1)
	require.NoError(t, err)
	for range 2 {
		_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, event, completedRetentionEventEffect(event))
		require.NoError(t, err)
	}
	message, err := repo.GetMessageByToolCallID(ctx, "shell-session", "tool")
	require.NoError(t, err)
	require.Equal(t, "command", message.Content)
	require.Equal(t, "shell-turn", message.TurnID)
	var count int
	require.NoError(t, db.Get(&count, `SELECT COUNT(*) FROM task_session_messages WHERE task_session_id='shell-session'`))
	require.Equal(t, 1, count)
}

func TestIncrementalShellPostgresProjection(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	require.NoError(t, repo.initSchema())
	seedIncrementalShellMessage(t, repo)
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(2)
	first, err := db.Conn(context.Background())
	require.NoError(t, err)
	second, err := db.Conn(context.Background())
	require.NoError(t, err)
	var firstPID, secondPID int
	require.NoError(t, first.QueryRowContext(context.Background(), `SELECT pg_backend_pid()`).Scan(&firstPID))
	require.NoError(t, second.QueryRowContext(context.Background(), `SELECT pg_backend_pid()`).Scan(&secondPID))
	require.NotEqual(t, firstPID, secondPID)
	require.NoError(t, first.Close())
	require.NoError(t, second.Close())
	patch, err := streams.EncodeShellOutputPatch(nil, streams.ShellExecOutput{Stdout: "雪\noutput"}, "epoch", true)
	require.NoError(t, err)
	event := incrementalShellTestEvent(t, 1, patch)
	_, err = repo.ReceiveAgentDeliveryEvent(context.Background(), event, 1)
	require.NoError(t, err)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			_, e := repo.ProjectCanonicalAgentDeliveryEvent(context.Background(), event, completedRetentionEventEffect(event))
			errs <- e
		})
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	message, err := repo.GetMessage(context.Background(), "shell-message")
	require.NoError(t, err)
	require.NoError(t, repo.RehydrateMessagePayload(context.Background(), message))
	snapshot, ok := models.ExtractShellExecOutput(message.Metadata)
	require.True(t, ok)
	require.Equal(t, "雪\noutput", snapshot.Stdout)
}

func TestIncrementalShellPostgresOrdinaryReplacement(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	seedIncrementalShellMessage(t, repo)
	ctx := context.Background()
	for index, receipt := range []bool{false, true} {
		patch, encodeErr := streams.EncodeShellOutputPatch(nil, streams.ShellExecOutput{Stdout: "foreground"}, fmt.Sprint(index), true)
		require.NoError(t, encodeErr)
		event := incrementalShellTestEvent(t, int64(index+1), patch)
		_, err = repo.ReceiveAgentDeliveryEvent(ctx, event, event.Sequence)
		require.NoError(t, err)
		_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, event, completedRetentionEventEffect(event))
		require.NoError(t, err)
		message, readErr := repo.GetMessage(ctx, "shell-message")
		require.NoError(t, readErr)
		message.Metadata = models.ProjectMessageMetadata(message.Metadata)
		normalized := streams.NewShellExec("test-command", "/tmp", "", 0, true)
		normalized.ShellExec().Output = &streams.ShellExecOutput{Stdout: "background"}
		message.Metadata["normalized"] = normalized
		if receipt {
			_, err = repo.UpdateMessageWithConversationReceipt(ctx, message)
		} else {
			err = repo.UpdateMessage(ctx, message)
		}
		require.NoError(t, err)
		message, err = repo.GetMessage(ctx, "shell-message")
		require.NoError(t, err)
		require.NoError(t, repo.RehydrateMessagePayload(ctx, message))
		output, ok := models.ExtractShellExecOutput(message.Metadata)
		require.True(t, ok)
		require.Equal(t, "background", output.Stdout)
		var heads int
		require.NoError(t, repo.db.Get(&heads, `SELECT COUNT(*) FROM task_message_shell_output`))
		require.Zero(t, heads)
	}
}
