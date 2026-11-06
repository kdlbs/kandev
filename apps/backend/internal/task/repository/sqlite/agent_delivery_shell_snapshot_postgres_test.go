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

func TestIncrementalShellPostgresSnapshotDuringReplacement(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	db.SetMaxOpenConns(4)
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	seedIncrementalShellMessage(t, repo)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	patch, err := streams.EncodeShellOutputPatch(nil, streams.ShellExecOutput{Stdout: "before"}, "epoch", true)
	require.NoError(t, err)
	first := incrementalShellTestEvent(t, 1, patch)
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, first, 1)
	require.NoError(t, err)
	_, err = repo.ProjectCanonicalAgentDeliveryEvent(ctx, first, completedRetentionEventEffect(first))
	require.NoError(t, err)
	message, err := repo.GetMessage(ctx, "shell-message")
	require.NoError(t, err)
	writer, err := db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = writer.Rollback() }()
	_, err = writer.ExecContext(ctx, `LOCK TABLE task_message_shell_output IN ACCESS EXCLUSIVE MODE`)
	require.NoError(t, err)
	read := make(chan error, 1)
	go func() { read <- repo.RehydrateMessagePayload(ctx, message) }()
	require.Eventually(t, func() bool {
		var waiting bool
		queryErr := db.GetContext(ctx, &waiting, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE relation='task_message_shell_output'::regclass AND mode='AccessShareLock' AND NOT granted)`)
		return queryErr == nil && waiting
	}, 5*time.Second, 10*time.Millisecond, "reader must pause after loading message metadata")
	base, err := streams.ApplyShellOutputPatch(nil, patch)
	require.NoError(t, err)
	patch, err = streams.EncodeShellOutputPatch(base, streams.ShellExecOutput{Stdout: "after"}, "epoch", false)
	require.NoError(t, err)
	next := incrementalShellTestEvent(t, 2, patch)
	var agent streams.AgentEvent
	require.NoError(t, json.Unmarshal(next.Payload, &agent))
	agent.ToolStatus = "complete"
	require.NoError(t, repo.persistCanonicalShellOutputTx(ctx, writer, next, agent))
	require.NoError(t, writer.Commit())
	require.NoError(t, <-read)
	output, ok := models.ExtractShellExecOutput(message.Metadata)
	require.True(t, ok)
	require.Equal(t, "before", output.Stdout)
	require.Equal(t, "in_progress", message.Metadata["status"])
}
