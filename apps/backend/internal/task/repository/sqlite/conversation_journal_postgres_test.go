package sqlite

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresConversationJournalRoundTrip exercises the PostgreSQL journal
// triggers end to end: message insert/update sanitization through
// conversation_visible_content, the meta schema-version marker, and the
// version-guarded one-time migration. Skips unless KANDEV_TEST_POSTGRES_DSN is
// set (CI runs it in postgres-boot).
func TestPostgresConversationJournalRoundTrip(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	ctx := context.Background()
	seedPostgresConversationJournal(t, repo, "task-pg-journal", "session-pg-journal", "turn-pg-journal")

	content := "pre <kandev-system>hidden one\nline two</kandev-system> mid <kandev-system>two</kandev-system> post"
	message := &models.Message{
		ID: "message-pg-journal", TaskSessionID: "session-pg-journal", TaskID: "task-pg-journal",
		TurnID: "turn-pg-journal", AuthorType: models.MessageAuthorUser,
		Type: models.MessageTypeMessage, Content: content,
		Metadata: map[string]interface{}{
			"pending_id": "pending-pg", "status": "pending", "raw_content": "secret",
		},
	}
	if err := repo.CreateMessage(ctx, message); err != nil {
		t.Fatalf("create message: %v", err)
	}
	want := sysprompt.StripSystemContent(content)

	var eventPayload string
	if err := repo.db.Get(&eventPayload, `SELECT payload FROM conversation_session_events WHERE session_id = 'session-pg-journal' AND event_type = 'message.added'`); err != nil {
		t.Fatalf("read event payload: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(eventPayload), &payload); err != nil {
		t.Fatalf("decode event payload: %v", err)
	}
	metadata, ok := payload["metadata"].(map[string]interface{})
	if !ok || metadata["pending_id"] != "pending-pg" || metadata["status"] != "pending" {
		t.Fatalf("postgres journal metadata = %#v, want allowlisted presentation metadata", payload["metadata"])
	}
	if _, ok := metadata["raw_content"]; ok {
		t.Fatalf("postgres journal retained internal raw_content metadata: %#v", metadata)
	}
	got, ok := payload["content"].(string)
	if !ok || got != want || strings.Contains(got, "hidden") {
		t.Fatalf("postgres journal content = %q, want %q", got, want)
	}

	// The schema marker must be recorded so the one-time migration is not
	// re-run (and the full-corpus rewrite never repeats on later boots).
	version, err := repo.readConversationJournalMetaInt("schema.version")
	if err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version < journalSchemaVersion {
		t.Fatalf("schema version = %d, want >= %d", version, journalSchemaVersion)
	}
	backfilled, err := repo.readConversationJournalMetaFlag(journalBackfillKey)
	if err != nil {
		t.Fatalf("read backfill flag: %v", err)
	}
	if !backfilled {
		t.Fatal("backfill flag not recorded after init")
	}
}

func TestPostgresConversationJournalStripsPastSQLiteDepthLimit(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	seedPostgresConversationJournal(t, repo, "task-pg-strip-deep", "session-pg-strip-deep", "turn-pg-strip-deep")
	content := strings.Repeat("<kandev-system>secret</kandev-system>", 65) + "visible"
	message := &models.Message{
		ID: "message-pg-strip-deep", TaskSessionID: "session-pg-strip-deep", TaskID: "task-pg-strip-deep",
		TurnID: "turn-pg-strip-deep", AuthorType: models.MessageAuthorUser,
		Type: models.MessageTypeMessage, Content: content,
	}
	if err := repo.CreateMessage(context.Background(), message); err != nil {
		t.Fatalf("create message: %v", err)
	}

	for _, table := range []string{"conversation_message_versions", "conversation_session_events"} {
		var payload string
		orderColumn := "row_sequence"
		if table == "conversation_session_events" {
			orderColumn = "sequence"
		}
		query := "SELECT payload FROM " + table + " WHERE session_id = $1 ORDER BY " + orderColumn + " DESC LIMIT 1"
		if err := repo.db.Get(&payload, query, "session-pg-strip-deep"); err != nil {
			t.Fatalf("read %s payload: %v", table, err)
		}
		if strings.Contains(payload, "secret") || strings.Contains(payload, "<kandev-system>") {
			t.Fatalf("%s persisted system content: %s", table, payload)
		}
	}
}

func TestPostgresConversationJournalTurnDeletePreservesCutoffHistory(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	const (
		taskID    = "task-pg-turn-delete"
		sessionID = "session-pg-turn-delete"
		turnID    = "turn-pg-turn-delete"
	)
	seedPostgresConversationJournal(t, repo, taskID, sessionID, turnID)
	now := time.Now().UTC()
	if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE task_session_turns SET completed_at = ?, updated_at = ? WHERE id = ?`), now, now, turnID); err != nil {
		t.Fatalf("complete turn: %v", err)
	}
	var cutoffBefore int64
	if err := repo.db.Get(&cutoffBefore, repo.db.Rebind(`SELECT watermark FROM conversation_session_streams WHERE session_id = ?`), sessionID); err != nil {
		t.Fatalf("read pre-delete cutoff: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`DELETE FROM task_session_turns WHERE id = ?`), turnID); err != nil {
		t.Fatalf("delete turn: %v", err)
	}
	var cutoffAfter int64
	if err := repo.db.Get(&cutoffAfter, repo.db.Rebind(`SELECT watermark FROM conversation_session_streams WHERE session_id = ?`), sessionID); err != nil {
		t.Fatalf("read deletion cutoff: %v", err)
	}
	assertTurnLiveAtCutoff(t, repo, sessionID, turnID, cutoffBefore, true)
	assertTurnLiveAtCutoff(t, repo, sessionID, turnID, cutoffAfter, false)
}

func seedPostgresConversationJournal(t *testing.T, repo *Repository, taskID, sessionID, turnID string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, '', 'test task', ?, ?)
	`), taskID, now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_sessions (id, task_id, started_at, updated_at)
		VALUES (?, ?, ?, ?)
	`), sessionID, taskID, now, now); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`), turnID, sessionID, taskID, now, now, now); err != nil {
		t.Fatalf("seed turn: %v", err)
	}
}
