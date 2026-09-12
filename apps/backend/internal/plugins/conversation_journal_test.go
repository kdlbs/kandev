package plugins

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/events"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func TestConversationMessagesAtUsesImmutableCutoff(t *testing.T) {
	database, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	_, err = database.Exec(`
		CREATE TABLE conversation_message_versions (
			session_id TEXT NOT NULL, message_id TEXT NOT NULL, row_sequence INTEGER NOT NULL,
			task_id TEXT, author_type TEXT, created_at TIMESTAMP NOT NULL,
			tombstone BOOLEAN NOT NULL, payload TEXT NOT NULL
		);`)
	require.NoError(t, err)

	createdAt := "2026-09-07T12:00:00Z"
	insertJournalMessageVersion(t, database, 1, false, map[string]any{
		"message_id": "message-1", "turn_id": "turn-1", "task_id": "task-1",
		"author_type": "agent", "author_id": "agent-1", "content": "before",
		"requests_input": 0, "message_type": "message", "created_at": createdAt,
		"updated_at": createdAt, "prompt_index": 0, "metadata_json": `{}`,
	})
	insertJournalMessageVersion(t, database, 2, false, map[string]any{
		"message_id": "message-1", "turn_id": "turn-1", "task_id": "task-1",
		"author_type": "agent", "author_id": "agent-1", "content": "after",
		"requests_input": 0, "message_type": "message", "created_at": createdAt,
		"updated_at": "2026-09-07T12:01:00Z", "prompt_index": 0, "metadata_json": `{}`,
	})
	insertJournalMessageVersion(t, database, 3, true, map[string]any{"message_id": "message-1"})

	service := NewService(nil, NewRegistry(), nil, testLogger(t))
	service.SetConversationJournalDB(database)

	atInsert, more, err := service.conversationMessagesAt(context.Background(), "session-1", 1, nil, nil, "asc", "", 20)
	require.NoError(t, err)
	require.False(t, more)
	require.Len(t, atInsert, 1)
	require.Equal(t, "before", atInsert[0].Content)
	require.Equal(t, "session-1", atInsert[0].TaskSessionID)

	atUpdate, _, err := service.conversationMessagesAt(context.Background(), "session-1", 2, nil, nil, "asc", "", 20)
	require.NoError(t, err)
	require.Len(t, atUpdate, 1)
	require.Equal(t, "after", atUpdate[0].Content)

	afterDelete, _, err := service.conversationMessagesAt(context.Background(), "session-1", 3, nil, nil, "asc", "", 20)
	require.NoError(t, err)
	require.Empty(t, afterDelete)
}

func TestSessionEventMaintenanceDispatchesUnsignaledCommittedRowsOnce(t *testing.T) {
	database, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	_, err = database.Exec(`
		CREATE TABLE conversation_session_streams (
			session_id TEXT PRIMARY KEY, watermark INTEGER NOT NULL
		);
		CREATE TABLE conversation_session_events (
			session_id TEXT NOT NULL, sequence INTEGER NOT NULL, event_id TEXT NOT NULL,
			protocol_version INTEGER NOT NULL, event_type TEXT NOT NULL, task_id TEXT,
			payload TEXT NOT NULL, created_at TIMESTAMP NOT NULL
		);
		INSERT INTO conversation_session_streams(session_id, watermark) VALUES ('session-1', 1);
		INSERT INTO conversation_session_events(
			session_id, sequence, event_id, protocol_version, event_type, task_id, payload, created_at
		) VALUES (
			'session-1', 1, 'session-1:1', 1, 'message.deleted', 'task-1',
			'{"type":"message.deleted","session_id":"session-1","task_id":"task-1","message_id":"message-1"}',
			'2026-09-07T12:00:00Z'
		);`)
	require.NoError(t, err)

	service := NewService(nil, NewRegistry(), nil, testLogger(t))
	service.SetConversationJournalDB(database)
	var dispatched []SessionEvent
	service.SetSessionEventSink(func(event SessionEvent) {
		dispatched = append(dispatched, event)
	})

	require.NoError(t, service.maintainSessionEvents(context.Background(), time.Now().UTC()))
	require.Len(t, dispatched, 1)
	require.Equal(t, uint64(1), dispatched[0].Sequence)
	require.NoError(t, service.maintainSessionEvents(context.Background(), time.Now().UTC()))
	require.Len(t, dispatched, 1)
}

func TestPruneConversationJournalKeepsLatestRowsAndRecentEvents(t *testing.T) {
	database, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	_, err = database.Exec(`
		CREATE TABLE conversation_message_versions (
			session_id TEXT NOT NULL, message_id TEXT NOT NULL, row_sequence INTEGER NOT NULL,
			task_id TEXT, author_type TEXT, created_at TIMESTAMP NOT NULL,
			tombstone BOOLEAN NOT NULL, payload TEXT NOT NULL
		);
		CREATE TABLE conversation_turn_versions (
			session_id TEXT NOT NULL, turn_id TEXT NOT NULL, row_sequence INTEGER NOT NULL,
			task_id TEXT, started_at TIMESTAMP NOT NULL, tombstone BOOLEAN NOT NULL, payload TEXT NOT NULL
		);
		CREATE TABLE conversation_session_events (
			session_id TEXT NOT NULL, sequence INTEGER NOT NULL, event_id TEXT NOT NULL,
			protocol_version INTEGER NOT NULL, event_type TEXT NOT NULL, task_id TEXT,
			payload TEXT NOT NULL, created_at TIMESTAMP NOT NULL
		);`)
	require.NoError(t, err)
	old := time.Now().UTC().Add(-2 * SessionEventRetention)
	recent := time.Now().UTC()
	_, err = database.Exec(`
		INSERT INTO conversation_message_versions VALUES
			('session-1', 'message-1', 1, 'task-1', 'agent', ?, FALSE, '{}'),
			('session-1', 'message-1', 2, 'task-1', 'agent', ?, FALSE, '{}');
		INSERT INTO conversation_turn_versions VALUES
			('session-1', 'turn-1', 1, 'task-1', ?, FALSE, '{}'),
			('session-1', 'turn-1', 2, 'task-1', ?, FALSE, '{}');
		INSERT INTO conversation_session_events VALUES
			('session-1', 1, 'session-1:1', 1, 'message.added', 'task-1', '{}', ?),
			('session-1', 2, 'session-1:2', 1, 'message.updated', 'task-1', '{}', ?);`,
		old, recent, old, recent, old, recent)
	require.NoError(t, err)

	service := NewService(nil, NewRegistry(), nil, testLogger(t))
	service.SetConversationJournalDB(database)
	require.NoError(t, service.pruneConversationJournal(context.Background(), time.Now().UTC().Add(-SessionEventRetention), nil))

	var count int
	require.NoError(t, database.Get(&count, "SELECT COUNT(*) FROM conversation_message_versions"))
	require.Equal(t, 1, count)
	require.NoError(t, database.Get(&count, "SELECT COUNT(*) FROM conversation_turn_versions"))
	require.Equal(t, 1, count)
	require.NoError(t, database.Get(&count, "SELECT COUNT(*) FROM conversation_session_events"))
	require.Equal(t, 1, count)
}
func TestPruneConversationJournalPurgestDeadSessionPartitions(t *testing.T) {
	database, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	database.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	_, err = database.Exec(`
		CREATE TABLE conversation_session_streams (
			session_id TEXT PRIMARY KEY, watermark INTEGER NOT NULL DEFAULT 0,
			terminal BOOLEAN NOT NULL DEFAULT FALSE, updated_at TIMESTAMP NOT NULL
		);
		CREATE TABLE conversation_message_versions (
			session_id TEXT NOT NULL, message_id TEXT NOT NULL, row_sequence INTEGER NOT NULL,
			task_id TEXT, author_type TEXT, created_at TIMESTAMP NOT NULL,
			tombstone BOOLEAN NOT NULL, payload TEXT NOT NULL
		);
		CREATE TABLE conversation_turn_versions (
			session_id TEXT NOT NULL, turn_id TEXT NOT NULL, row_sequence INTEGER NOT NULL,
			task_id TEXT, started_at TIMESTAMP NOT NULL, tombstone BOOLEAN NOT NULL, payload TEXT NOT NULL
		);
		CREATE TABLE conversation_session_events (
			session_id TEXT NOT NULL, sequence INTEGER NOT NULL, event_id TEXT NOT NULL,
			protocol_version INTEGER NOT NULL, event_type TEXT NOT NULL, task_id TEXT,
			payload TEXT NOT NULL, created_at TIMESTAMP NOT NULL
		);
		CREATE TABLE task_sessions (
			id TEXT PRIMARY KEY, task_id TEXT NOT NULL, started_at TIMESTAMP NOT NULL
		);`)
	require.NoError(t, err)
	old := time.Now().UTC().Add(-2 * SessionEventRetention)
	recent := time.Now().UTC()
	for _, seed := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO conversation_session_streams VALUES ('session-dead', 5, TRUE, ?)`, []any{old}},
		{`INSERT INTO conversation_session_streams VALUES ('session-live', 5, TRUE, ?)`, []any{recent}},
		{`INSERT INTO conversation_session_streams VALUES ('session-retained', 5, TRUE, ?)`, []any{recent}},
		{`INSERT INTO conversation_message_versions VALUES ('session-dead', 'm-1', 1, 'task-1', 'agent', ?, FALSE, '{}')`, []any{old}},
		{`INSERT INTO conversation_message_versions VALUES ('session-dead', 'm-1', 2, 'task-1', 'agent', ?, FALSE, '{}')`, []any{recent}},
		{`INSERT INTO conversation_turn_versions VALUES ('session-dead', 'turn-1', 3, 'task-1', ?, FALSE, '{}')`, []any{old}},
		{`INSERT INTO conversation_session_events VALUES ('session-dead', 1, 'session-dead:1', 1, 'message.added', 'task-1', '{}', ?)`, []any{old}},
		{`INSERT INTO conversation_session_events VALUES ('session-dead', 5, 'session-dead:5', 1, 'session.removed', 'task-1', '{}', ?)`, []any{recent}},
		{`INSERT INTO task_sessions VALUES ('session-live', 'task-1', ?)`, []any{recent}},
	} {
		if _, err := database.Exec(seed.sql, seed.args...); err != nil {
			t.Fatalf("seed: %v\n%s", err, seed.sql)
		}
	}

	service := NewService(nil, NewRegistry(), nil, testLogger(t))
	service.SetConversationJournalDB(database)
	retained := map[string]struct{}{"session-retained": {}}
	require.NoError(t, service.pruneConversationJournal(context.Background(), time.Now().UTC(), retained))

	var streams int
	require.NoError(t, database.Get(&streams, "SELECT COUNT(*) FROM conversation_session_streams"))
	require.Equal(t, 2, streams) // live + retained survive; dead purged
	var versions int
	require.NoError(t, database.Get(&versions, "SELECT COUNT(*) FROM conversation_message_versions"))
	require.Equal(t, 0, versions)
	require.NoError(t, database.Get(&versions, "SELECT COUNT(*) FROM conversation_turn_versions"))
	require.Equal(t, 0, versions)
	require.NoError(t, database.Get(&versions, "SELECT COUNT(*) FROM conversation_session_events"))
	require.Equal(t, 0, versions)
	require.NoError(t, database.Get(&streams, "SELECT COUNT(*) FROM conversation_session_streams WHERE session_id = 'session-dead'"))
	require.Equal(t, 0, streams)
}

func insertJournalMessageVersion(t *testing.T, database *sqlx.DB, sequence int, tombstone bool, payload map[string]any) {

	t.Helper()
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	_, err = database.Exec(`
		INSERT INTO conversation_message_versions(
			session_id, message_id, row_sequence, task_id, author_type, created_at, tombstone, payload
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"session-1", "message-1", sequence, "task-1", "agent", time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC), tombstone, string(raw),
	)
	require.NoError(t, err)
}
func TestSanitizeConversationTurnEventPreservesPublicMetadata(t *testing.T) {
	raw := json.RawMessage(`{
		"type": "session.turn.completed",
		"session_id": "session-1",
		"task_id": "task-1",
		"id": "turn-1",
		"started_at": "2026-09-07T12:00:00Z",
		"completed_at": "2026-09-07T12:01:00Z",
		"metadata": {
			"runtime_config_snapshot": {
				"model": "mock-fast",
				"config_baseline": {"effort": "medium"}
			}
		},
		"private": "must-not-be-copied"
	}`)

	sanitized := sanitizeConversationEventPayload(conversationTurnCompleted, raw)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(sanitized, &payload))
	require.Equal(t, map[string]any{
		"runtime_config_snapshot": map[string]any{
			"model":           "mock-fast",
			"config_baseline": map[string]any{"effort": "medium"},
		},
	}, payload["metadata"])
	require.NotContains(t, payload, "private")
}
func TestSanitizeConversationMessageEventPreservesPresentationMetadata(t *testing.T) {
	raw := json.RawMessage(`{
		"type": "message.added",
		"session_id": "session-1",
		"task_id": "task-1",
		"message_id": "message-1",
		"author_type": "agent",
		"message_type": "clarification_request",
		"requests_input": true,
		"metadata": {
			"pending_id": "pending-1",
			"question": {"id": "question-1", "prompt": "Choose <kandev-system>hidden</kandev-system>"},
			"secret": "must-not-be-copied",
			"raw_content": "must-not-be-copied"
		},
		"content": "Question"
	}`)

	sanitized := sanitizeConversationEventPayload(events.MessageAdded, raw)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(sanitized, &payload))
	require.Equal(t, "clarification_request", payload["message_type"])
	require.Equal(t, true, payload["requests_input"])
	require.Equal(t, map[string]any{
		"pending_id": "pending-1",
		"question":   map[string]any{"id": "question-1", "prompt": "Choose"},
	}, payload["metadata"])
}

func TestSyncCommittedSessionEventsStripsSystemContent(t *testing.T) {
	database, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	_, err = database.Exec(`
		CREATE TABLE conversation_session_streams (
			session_id TEXT PRIMARY KEY, watermark INTEGER NOT NULL
		);
		CREATE TABLE conversation_session_events (
			session_id TEXT NOT NULL, sequence INTEGER NOT NULL, event_id TEXT NOT NULL,
			protocol_version INTEGER NOT NULL, event_type TEXT NOT NULL, task_id TEXT,
			payload TEXT NOT NULL, created_at TIMESTAMP NOT NULL
		);
		INSERT INTO conversation_session_streams(session_id, watermark) VALUES ('session-1', 1);
		INSERT INTO conversation_session_events VALUES (
			'session-1', 1, 'session-1:1', 1, 'message.added', 'task-1',
			'{"type":"message.added","session_id":"session-1","task_id":"task-1","message_id":"message-1","author_type":"user","author_id":"agent-1","metadata":{"sender_task_id":"sender-1","secret":"no"},"content":"visible <kandev-system>secret</kandev-system>"}',
			'2026-09-07T12:00:00Z'
		);`)
	require.NoError(t, err)

	service := NewService(nil, NewRegistry(), nil, testLogger(t))
	service.SetConversationJournalDB(database)
	var payload map[string]any
	events, err := service.SyncCommittedSessionEvents(context.Background(), "session-1")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(events[0].Payload, &payload))
	require.Equal(t, "visible", payload["content"])
	require.Equal(t, "sender-1", payload["sender_task_id"])
	require.NotContains(t, payload, "author_id")
	require.Equal(t, map[string]any{"sender_task_id": "sender-1"}, payload["metadata"])
}

func TestSyncCommittedSessionEventsPoisonsMalformedPayloadWithoutRetainingRawBytes(t *testing.T) {
	const sessionID = "malformed-journal-session"
	database, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	_, err = database.Exec(`
		CREATE TABLE conversation_session_streams (
			session_id TEXT PRIMARY KEY, watermark INTEGER NOT NULL
		);
		CREATE TABLE conversation_session_events (
			session_id TEXT NOT NULL, sequence INTEGER NOT NULL, event_id TEXT NOT NULL,
			protocol_version INTEGER NOT NULL, event_type TEXT NOT NULL, task_id TEXT,
			payload TEXT NOT NULL, created_at TIMESTAMP NOT NULL
		);
		INSERT INTO conversation_session_streams(session_id, watermark) VALUES ('` + sessionID + `', 1);
		INSERT INTO conversation_session_events VALUES (
			'` + sessionID + `', 1, '` + sessionID + `:1', 1, 'message.added', 'task-1',
			'{"content":"private <kandev-system>secret</kandev-system>"',
			'2026-09-07T12:00:00Z'
		);`)
	require.NoError(t, err)

	service := NewService(nil, NewRegistry(), nil, testLogger(t))
	service.SetConversationJournalDB(database)
	events, err := service.SyncCommittedSessionEvents(context.Background(), sessionID)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "{}", string(events[0].Payload))
	require.NotContains(t, string(events[0].Payload), "private")
	require.NotContains(t, string(events[0].Payload), "kandev-system")
	_, projectionErr := ProjectSessionEvent(events[0])
	require.Error(t, projectionErr)
}

func TestConversationMessagesAtErrorsWhenPageCursorWasTombstoned(t *testing.T) {
	database, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	_, err = database.Exec(`
		CREATE TABLE conversation_message_versions (
			session_id TEXT NOT NULL, message_id TEXT NOT NULL, row_sequence INTEGER NOT NULL,
			task_id TEXT, author_type TEXT, created_at TIMESTAMP NOT NULL,
			tombstone BOOLEAN NOT NULL, payload TEXT NOT NULL
		);`)
	require.NoError(t, err)
	createdAt := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	insertJournalMessageVersion(t, database, 1, false, map[string]any{
		"message_id": "message-1", "turn_id": "turn-1", "task_id": "task-1",
		"author_type": "agent", "content": "first", "message_type": "message",
		"created_at": createdAt.Format(time.RFC3339Nano),
		"updated_at": createdAt.Format(time.RFC3339Nano),
	})
	insertJournalMessageVersion(t, database, 2, true, map[string]any{"message_id": "message-1"})

	service := NewService(nil, NewRegistry(), nil, testLogger(t))
	service.SetConversationJournalDB(database)

	_, _, err = service.conversationMessagesAt(
		context.Background(), "session-1", 2, nil, nil, "asc", "message-1", 20,
	)
	require.ErrorIs(t, err, errConversationCursorGone)
}

// TestSyncCommittedSessionEventsPoisonsMalformedPayloadOnDurableLog pins that
// a malformed primary-journal row is mirrored into the FILE-backed durable log
// as a poison record with a non-nil, non-raw payload (the session_events
// payload column is BLOB NOT NULL, so nil would fail the append).
func TestSyncCommittedSessionEventsPoisonsMalformedPayloadOnDurableLog(t *testing.T) {
	database, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	_, err = database.Exec(`
		CREATE TABLE conversation_session_streams (
			session_id TEXT PRIMARY KEY, watermark INTEGER NOT NULL
		);
		CREATE TABLE conversation_session_events (
			session_id TEXT NOT NULL, sequence INTEGER NOT NULL, event_id TEXT NOT NULL,
			protocol_version INTEGER NOT NULL, event_type TEXT NOT NULL, task_id TEXT,
			payload TEXT NOT NULL, created_at TIMESTAMP NOT NULL
		);
		INSERT INTO conversation_session_streams(session_id, watermark) VALUES ('sess-durable-poison', 1);
		INSERT INTO conversation_session_events VALUES (
			'sess-durable-poison', 1, 'sess-durable-poison:1', 1, 'message.added', 'task-1',
			'{"content":"private <kandev-system>secret</kandev-system>"',
			'2026-09-07T12:00:00Z'
		);`)
	require.NoError(t, err)

	dir := t.TempDir()
	service := NewService(nil, NewRegistry(), nil, testLogger(t))
	require.NoError(t, service.SetPluginsDir(dir))
	t.Cleanup(func() { _ = service.Close() })
	service.SetConversationJournalDB(database)

	events, err := service.SyncCommittedSessionEvents(context.Background(), "sess-durable-poison")
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "{}", string(events[0].Payload))
	log := service.SessionEvents()
	record, ok := log.Poison("sess-durable-poison", events[0].ID)
	require.True(t, ok)
	require.NotEmpty(t, record.LastError)

	// A restart of the durable log keeps the poison and the non-raw payload.
	reopened, err := NewSessionEventLog(filepath.Join(dir, ".host", "session-events.sqlite"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = reopened.Close() })
	restored, ok := reopened.Poison("sess-durable-poison", events[0].ID)
	require.True(t, ok)
	require.Equal(t, record.Attempts, restored.Attempts)
}
