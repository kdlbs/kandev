package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestToolCompletionTimestamp(t *testing.T) {
	assertToolCompletionTimestamp(t, newRepoForSessionTests(t))
}

func TestToolCompletionTimestampPostgres(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	assertToolCompletionTimestamp(t, repo)
}

func assertToolCompletionTimestamp(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	_, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO tasks (id, workspace_id, title, created_at, updated_at) VALUES (?, '', 'tool clock', ?, ?)`), "tool-clock-task", now, now)
	require.NoError(t, err)
	_, err = repo.db.Exec(repo.db.Rebind(`INSERT INTO task_sessions (id, task_id, started_at, updated_at) VALUES (?, ?, ?, ?)`), "tool-clock-session", "tool-clock-task", now, now)
	require.NoError(t, err)
	_, err = repo.db.Exec(repo.db.Rebind(`INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`), "tool-clock-turn", "tool-clock-session", "tool-clock-task", now, now, now)
	require.NoError(t, err)
	message := &models.Message{
		ID: "tool-clock-message", TaskID: "tool-clock-task", TaskSessionID: "tool-clock-session",
		TurnID: "tool-clock-turn", Type: models.MessageTypeToolCall,
		Metadata:  map[string]any{"tool_call_id": "call", "status": "pending"},
		CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, repo.CreateMessage(ctx, message))
	before := time.Now().UTC()
	count, err := repo.CompletePendingToolCallsForTurn(ctx, message.TurnID)
	after := time.Now().UTC()
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	completed, err := repo.GetMessage(ctx, message.ID)
	require.NoError(t, err)
	require.Equal(t, "complete", completed.Metadata["status"])
	// PostgreSQL stores microseconds; neither driver may regress to a whole-second revision.
	require.False(t, completed.UpdatedAt.Before(before.Truncate(time.Microsecond)), "completion revision %s precedes mutation %s", completed.UpdatedAt, before)
	require.False(t, completed.UpdatedAt.After(after), "completion revision exceeds mutation interval")
}
