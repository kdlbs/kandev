package plugins

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

// TestPostgresConversationJournalProjectsRFC3339Times covers the real
// PostgreSQL trigger output at both consumers: immutable snapshot decoding and
// ordered ProjectSessionEvent validation. It skips when KANDEV_TEST_POSTGRES_DSN
// is unavailable.
func TestPostgresConversationJournalProjectsRFC3339Times(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repository, err := tasksqlite.NewWithDB(database, database, nil)
	require.NoError(t, err)
	_, err = database.Exec(`SET TIME ZONE 'America/New_York'`)
	require.NoError(t, err)

	const (
		taskID    = "task-pg-journal-projection"
		sessionID = "session-pg-journal-projection"
		turnID    = "turn-pg-journal-projection"
	)
	startedAt := time.Date(2026, 9, 7, 12, 0, 0, 123456000, time.UTC)
	insertPostgresConversationTurn(t, database, taskID, sessionID, turnID, startedAt)
	messageAt := time.Date(2026, 9, 7, 12, 1, 0, 654321000, time.UTC)
	require.NoError(t, repository.CreateMessage(context.Background(), &models.Message{
		ID:            "message-pg-journal-projection",
		TaskSessionID: sessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		AuthorType:    models.MessageAuthorAgent,
		Type:          models.MessageTypeMessage,
		Content:       "visible",
		CreatedAt:     messageAt,
		UpdatedAt:     messageAt,
	}))

	service := NewService(nil, NewRegistry(), nil, testLogger(t))
	service.SetConversationJournalDB(database)
	var cutoff uint64
	require.NoError(t, database.Get(&cutoff,
		`SELECT watermark FROM conversation_session_streams WHERE session_id = $1`, sessionID))

	turns, err := service.conversationTurnsAt(context.Background(), sessionID, cutoff, nil)
	require.NoError(t, err)
	require.Len(t, turns, 1)
	require.True(t, turns[0].StartedAt.Equal(startedAt), "started_at = %s", turns[0].StartedAt)
	require.True(t, turns[0].UpdatedAt.Equal(startedAt), "updated_at = %s", turns[0].UpdatedAt)
	messages, more, err := service.conversationMessagesAt(
		context.Background(), sessionID, cutoff, nil, nil, "asc", "", 20,
	)
	require.NoError(t, err)
	require.False(t, more)
	require.Len(t, messages, 1)
	require.True(t, messages[0].CreatedAt.Equal(messageAt), "created_at = %s", messages[0].CreatedAt)
	require.True(t, messages[0].UpdatedAt.Equal(messageAt), "updated_at = %s", messages[0].UpdatedAt)

	events, err := service.SyncCommittedSessionEvents(context.Background(), sessionID)
	require.NoError(t, err)
	require.Len(t, events, 2)
	projectedTimes := make(map[string]map[string]any, len(events))
	for _, event := range events {
		projected, projectionErr := ProjectSessionEvent(event)
		require.NoError(t, projectionErr)
		var payload map[string]any
		require.NoError(t, json.Unmarshal(projected, &payload))
		projectedTimes[event.EventType] = payload
	}
	require.Equal(
		t,
		startedAt.Format(time.RFC3339Nano),
		projectedTimes["session.turn.started"]["started_at"],
	)
	require.Equal(
		t,
		startedAt.Format(time.RFC3339Nano),
		projectedTimes["session.turn.started"]["updated_at"],
	)
	require.Equal(
		t,
		messageAt.Format(time.RFC3339Nano),
		projectedTimes["message.added"]["created_at"],
	)
	require.Equal(
		t,
		messageAt.Format(time.RFC3339Nano),
		projectedTimes["message.added"]["updated_at"],
	)
}
func insertPostgresConversationTurn(
	t *testing.T,
	database *sqlx.DB,
	taskID string,
	sessionID string,
	turnID string,
	startedAt time.Time,
) {
	t.Helper()
	_, err := database.Exec(
		`INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		 VALUES ($1, '', 'test task', $2, $2)`,
		taskID,
		startedAt,
	)
	require.NoError(t, err)
	_, err = database.Exec(
		`INSERT INTO task_sessions (id, task_id, started_at, updated_at)
		 VALUES ($1, $2, $3, $3)`,
		sessionID,
		taskID,
		startedAt,
	)
	require.NoError(t, err)
	_, err = database.Exec(
		`INSERT INTO task_session_turns (
			id, task_session_id, task_id, started_at, created_at, updated_at
		 ) VALUES ($1, $2, $3, $4, $4, $4)`,
		turnID,
		sessionID,
		taskID,
		startedAt,
	)
	require.NoError(t, err)
}
