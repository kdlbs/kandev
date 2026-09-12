package sqlite

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
)

func TestConversationJournalVersionsMutationsAndTerminalDeletion(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-journal", "session-journal", "turn-journal")
	message := &models.Message{
		ID: "message-journal", TaskSessionID: "session-journal", TaskID: "task-journal",
		TurnID: "turn-journal", AuthorType: models.MessageAuthorAgent,
		Type: models.MessageTypeMessage, Content: "<kandev-system>hidden</kandev-system>first",
	}
	if err := repo.CreateMessage(ctx, message); err != nil {
		t.Fatalf("create message: %v", err)
	}
	message.Content = "<kandev-system>hidden-update</kandev-system>updated"
	if err := repo.UpdateMessage(ctx, message); err != nil {
		t.Fatalf("update message: %v", err)
	}
	if err := repo.DeleteMessage(ctx, message.ID); err != nil {
		t.Fatalf("delete message: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, "session-journal")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if err := repo.DeleteTaskSession(ctx, session); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	var terminal bool
	var watermark int64
	if err := repo.db.QueryRow(`SELECT watermark, terminal FROM conversation_session_streams WHERE session_id = 'session-journal'`).Scan(&watermark, &terminal); err != nil {
		t.Fatalf("read stream: %v", err)
	}
	if !terminal || watermark != 6 {
		t.Fatalf("stream = watermark %d terminal %v, want 6 true", watermark, terminal)
	}
	rows, err := repo.db.Query(`SELECT event_type FROM conversation_session_events WHERE session_id = 'session-journal' ORDER BY sequence`)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var eventTypes []string
	for rows.Next() {
		var eventType string
		if err := rows.Scan(&eventType); err != nil {
			t.Fatal(err)
		}
		eventTypes = append(eventTypes, eventType)
	}
	want := []string{"session.turn.started", "message.added", "message.updated", "message.deleted", "session.turn.removed", "session.removed"}
	if len(eventTypes) != len(want) {
		t.Fatalf("event types = %v, want %v", eventTypes, want)
	}
	for index := range want {
		if eventTypes[index] != want[index] {
			t.Fatalf("event types = %v, want %v", eventTypes, want)
		}
	}
	var payloadBytes []byte
	if err := repo.db.Get(&payloadBytes, `SELECT payload FROM conversation_session_events WHERE session_id = 'session-journal' AND event_type = 'message.added'`); err != nil {
		t.Fatalf("read message event payload: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		t.Fatalf("decode message event payload: %v", err)
	}
	if payload["type"] != "message.added" || payload["session_id"] != "session-journal" {
		t.Fatalf("message event identity = %#v", payload)
	}
	if content, ok := payload["content"].(string); !ok || content != "first" || strings.Contains(content, "hidden") {
		t.Fatalf("journal leaked system content: %#v", payload["content"])
	}
	createdAt, ok := payload["created_at"].(string)
	if !ok {
		t.Fatalf("message created_at = %#v", payload["created_at"])
	}
	if _, err := time.Parse(time.RFC3339Nano, createdAt); err != nil {
		t.Fatalf("message created_at is not RFC3339: %q: %v", createdAt, err)
	}
	var tombstone bool
	if err := repo.db.QueryRow(`SELECT tombstone FROM conversation_message_versions WHERE session_id = 'session-journal' AND message_id = 'message-journal' ORDER BY row_sequence DESC LIMIT 1`).Scan(&tombstone); err != nil {
		t.Fatalf("read message tombstone: %v", err)
	}
	if !tombstone {
		t.Fatal("latest message version is not a tombstone")
	}
}

func TestConversationJournalRollsBackWithSourceMutation(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-journal-rollback", "session-journal-rollback", "turn-journal-rollback")
	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	_, err = tx.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO task_session_messages
			(id, task_session_id, task_id, turn_id, author_type, content, type, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'agent', 'rolled back', 'message', '{}', ?, ?)
	`), "message-rollback", "session-journal-rollback", "task-journal-rollback", "turn-journal-rollback", now, now)
	if err != nil {
		_ = tx.Rollback()
		t.Fatalf("insert source row: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	var count int
	if err := repo.db.Get(&count, `SELECT COUNT(*) FROM conversation_session_events WHERE session_id = 'session-journal-rollback' AND event_type = 'message.added'`); err != nil {
		t.Fatalf("count rolled-back event: %v", err)
	}
	if count != 0 {
		t.Fatalf("rolled-back mutation left %d event rows", count)
	}
}

func TestConversationJournalStripsMultiLineAndMultiBlockSystemContent(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-journal-strip", "session-journal-strip", "turn-journal-strip")
	content := "pre <kandev-system>hidden one\nline two</kandev-system> mid\n<kandev-system>hidden2</kandev-system> post"
	message := &models.Message{
		ID: "message-strip", TaskSessionID: "session-journal-strip", TaskID: "task-journal-strip",
		TurnID: "turn-journal-strip", AuthorType: models.MessageAuthorUser,
		Type: models.MessageTypeMessage, Content: content,
	}
	if err := repo.CreateMessage(ctx, message); err != nil {
		t.Fatalf("create message: %v", err)
	}
	want := sysprompt.StripSystemContent(content)

	var versionContent string
	if err := repo.db.Get(&versionContent, `SELECT json_extract(payload, '$.content') FROM conversation_message_versions WHERE message_id = 'message-strip'`); err != nil {
		t.Fatalf("read message version content: %v", err)
	}
	if versionContent != want {
		t.Fatalf("version content = %q, want %q", versionContent, want)
	}
	var eventContent string
	if err := repo.db.Get(&eventContent, `SELECT json_extract(payload, '$.content') FROM conversation_session_events WHERE session_id = 'session-journal-strip' AND event_type = 'message.added'`); err != nil {
		t.Fatalf("read message event content: %v", err)
	}
	if eventContent != want {
		t.Fatalf("event content = %q, want %q", eventContent, want)
	}

	// The update trigger strips the same way.
	message.Content = "<kandev-system>a</kandev-system>\nupdated <kandev-system>b</kandev-system><kandev-system>c</kandev-system> end"
	if err := repo.UpdateMessage(ctx, message); err != nil {
		t.Fatalf("update message: %v", err)
	}
	wantUpdated := sysprompt.StripSystemContent(message.Content)
	if err := repo.db.Get(&versionContent, `SELECT json_extract(payload, '$.content') FROM conversation_message_versions WHERE message_id = 'message-strip' ORDER BY row_sequence DESC LIMIT 1`); err != nil {
		t.Fatalf("read updated message version content: %v", err)
	}
	if versionContent != wantUpdated {
		t.Fatalf("updated version content = %q, want %q", versionContent, wantUpdated)
	}
	if err := repo.db.Get(&eventContent, `SELECT json_extract(payload, '$.content') FROM conversation_session_events WHERE session_id = 'session-journal-strip' AND event_type = 'message.updated'`); err != nil {
		t.Fatalf("read updated message event content: %v", err)
	}
	if eventContent != wantUpdated {
		t.Fatalf("updated event content = %q, want %q", eventContent, wantUpdated)
	}
}

func TestConversationJournalStripIgnoresClosingTagWithoutOpeningTag(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-journal-closing-tag", "session-journal-closing-tag", "turn-journal-closing-tag")
	content := "visible</kandev-system>tail"
	message := &models.Message{
		ID: "message-closing-tag", TaskSessionID: "session-journal-closing-tag",
		TaskID: "task-journal-closing-tag", TurnID: "turn-journal-closing-tag",
		AuthorType: models.MessageAuthorUser, Type: models.MessageTypeMessage, Content: content,
	}
	if err := repo.CreateMessage(ctx, message); err != nil {
		t.Fatalf("create message: %v", err)
	}
	var eventContent string
	if err := repo.db.Get(&eventContent, `SELECT json_extract(payload, '$.content') FROM conversation_session_events WHERE session_id = 'session-journal-closing-tag' AND event_type = 'message.added'`); err != nil {
		t.Fatalf("read message event content: %v", err)
	}
	if eventContent != content {
		t.Fatalf("event content = %q, want %q", eventContent, content)
	}
}

func TestConversationJournalStripFailsClosedPastDepthLimit(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-journal-strip-deep", "session-journal-strip-deep", "turn-journal-strip-deep")
	content := strings.Repeat("<kandev-system>secret</kandev-system>", 65) + "visible"
	message := &models.Message{
		ID: "message-strip-deep", TaskSessionID: "session-journal-strip-deep", TaskID: "task-journal-strip-deep",
		TurnID: "turn-journal-strip-deep", AuthorType: models.MessageAuthorUser,
		Type: models.MessageTypeMessage, Content: content,
	}
	if err := repo.CreateMessage(ctx, message); err != nil {
		t.Fatalf("create message: %v", err)
	}

	for _, table := range []string{"conversation_message_versions", "conversation_session_events"} {
		var payload string
		query := "SELECT payload FROM " + table + " WHERE session_id = ? ORDER BY row_sequence DESC LIMIT 1"
		if table == "conversation_session_events" {
			query = "SELECT payload FROM " + table + " WHERE session_id = ? AND event_type = 'message.added' ORDER BY sequence DESC LIMIT 1"
		}
		if err := repo.db.Get(&payload, repo.db.Rebind(query), "session-journal-strip-deep"); err != nil {
			t.Fatalf("read %s payload: %v", table, err)
		}
		if strings.Contains(payload, "secret") || strings.Contains(payload, "<kandev-system>") {
			t.Fatalf("%s persisted system content past sanitizer depth: %s", table, payload)
		}
	}
}
func TestConversationJournalPreservesPresentationMetadata(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedForMsgTest(t, repo, "task-presentation-metadata", "session-presentation-metadata", "turn-presentation-metadata")
	message := &models.Message{
		ID: "message-presentation-metadata", TaskSessionID: "session-presentation-metadata",
		TaskID: "task-presentation-metadata", TurnID: "turn-presentation-metadata",
		AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeClarificationRequest,
		Content: "Question", RequestsInput: true,
		Metadata: map[string]any{
			"pending_id":  "pending-1",
			"question":    map[string]any{"id": "question-1", "prompt": "Choose"},
			"raw_content": "must-not-be-copied",
		},
	}
	if err := repo.CreateMessage(context.Background(), message); err != nil {
		t.Fatalf("create message: %v", err)
	}

	var payload string
	if err := repo.db.Get(&payload, `SELECT payload FROM conversation_session_events WHERE session_id = ? AND event_type = 'message.added'`, message.TaskSessionID); err != nil {
		t.Fatalf("read event payload: %v", err)
	}
	var event map[string]any
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		t.Fatalf("decode event payload: %v", err)
	}
	if event["message_type"] != string(models.MessageTypeClarificationRequest) || event["requests_input"] != float64(1) {
		t.Fatalf("message presentation fields = %#v", event)
	}
	metadata, ok := event["metadata"].(map[string]any)
	if !ok || metadata["pending_id"] != "pending-1" {
		t.Fatalf("message metadata = %#v", event["metadata"])
	}
	if _, exists := metadata["raw_content"]; exists {
		t.Fatalf("private message metadata leaked: %#v", metadata)
	}
}

func TestConversationJournalStripKeepsRawTextWhenNoWellFormedBlock(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-journal-strip-raw", "session-journal-strip-raw", "turn-journal-strip-raw")
	// Shapes where the Go helper has no full match: an unterminated opening
	// tag and a stray closing tag before the first opening tag. The SQLite
	// strip must keep the raw text (never NULL), byte-identical to Go.
	cases := map[string]string{
		"unterminated-open": "visible <kandev-system>never closed",
		"stray-close":       "</kandev-system>before <kandev-system>closed</kandev-system> after",
		"deep-nesting":      "<kandev-system>" + strings.Repeat("<kandev-system>", 70) + "x" + strings.Repeat("</kandev-system>", 70) + "</kandev-system>tail",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			id := "message-strip-raw-" + name
			message := &models.Message{
				ID: id, TaskSessionID: "session-journal-strip-raw", TaskID: "task-journal-strip-raw",
				TurnID: "turn-journal-strip-raw", AuthorType: models.MessageAuthorUser,
				Type: models.MessageTypeMessage, Content: content,
			}
			if err := repo.CreateMessage(ctx, message); err != nil {
				t.Fatalf("create message: %v", err)
			}
			var versionContent *string
			if err := repo.db.Get(&versionContent, `SELECT json_extract(payload, '$.content') FROM conversation_message_versions WHERE message_id = ?`, id); err != nil {
				t.Fatalf("read message version content: %v", err)
			}
			if versionContent == nil {
				t.Fatalf("strip returned NULL for %q", content)
			}
			want := sysprompt.StripSystemContent(content)
			if name != "deep-nesting" && *versionContent != want {
				t.Fatalf("version content = %q, want %q", *versionContent, want)
			}
			if strings.Contains(*versionContent, "<kandev-system>") && name == "unterminated-open" {
				// Unterminated opener text is preserved verbatim.
				if *versionContent != content {
					t.Fatalf("unterminated content = %q, want raw %q", *versionContent, content)
				}
			}
		})
	}
}

func TestConversationJournalSanitizeMigrationRewritesLegacyRows(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-journal-migrate", "session-journal-migrate", "turn-journal-migrate")
	legacy := "<kandev-system>old\nsecret</kandev-system>kept <kandev-system>second</kandev-system> text"
	message := &models.Message{
		ID: "message-legacy", TaskSessionID: "session-journal-migrate", TaskID: "task-journal-migrate",
		TurnID: "turn-journal-migrate", AuthorType: models.MessageAuthorUser,
		Type: models.MessageTypeMessage, Content: "placeholder",
	}
	if err := repo.CreateMessage(ctx, message); err != nil {
		t.Fatalf("create message: %v", err)
	}
	// Simulate rows written before the sanitize migration existed.
	if _, err := repo.db.Exec(`
		UPDATE conversation_message_versions SET payload = json_set(payload, '$.content', ?) WHERE message_id = 'message-legacy';
	`, legacy); err != nil {
		t.Fatalf("seed legacy version payload: %v", err)
	}
	if _, err := repo.db.Exec(`
		UPDATE conversation_session_events SET payload = json_set(payload, '$.content', ?) WHERE session_id = 'session-journal-migrate' AND event_type = 'message.added';
	`, legacy); err != nil {
		t.Fatalf("seed legacy event payload: %v", err)
	}

	if err := repo.initSQLiteConversationJournalTriggers(); err != nil {
		t.Fatalf("re-init triggers (runs sanitize migration): %v", err)
	}
	want := sysprompt.StripSystemContent(legacy)
	var versionContent string
	if err := repo.db.Get(&versionContent, `SELECT json_extract(payload, '$.content') FROM conversation_message_versions WHERE message_id = 'message-legacy'`); err != nil {
		t.Fatalf("read migrated version content: %v", err)
	}
	if versionContent != want {
		t.Fatalf("migrated version content = %q, want %q", versionContent, want)
	}
	var eventContent string
	if err := repo.db.Get(&eventContent, `SELECT json_extract(payload, '$.content') FROM conversation_session_events WHERE session_id = 'session-journal-migrate' AND event_type = 'message.added'`); err != nil {
		t.Fatalf("read migrated event content: %v", err)
	}
	if eventContent != want {
		t.Fatalf("migrated event content = %q, want %q", eventContent, want)
	}
}

func TestConversationJournalTurnDeletePreservesHistoryWithTombstone(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedForMsgTest(t, repo, "task-journal-turn-del", "session-journal-turn-del", "turn-journal-turn-del")

	now := time.Now().UTC()
	if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE task_session_turns SET completed_at = ?, updated_at = ? WHERE id = ?`), now, now, "turn-journal-turn-del"); err != nil {
		t.Fatalf("complete turn: %v", err)
	}
	var cutoffBefore int64
	if err := repo.db.Get(&cutoffBefore, `SELECT watermark FROM conversation_session_streams WHERE session_id = 'session-journal-turn-del'`); err != nil {
		t.Fatalf("read pre-delete cutoff: %v", err)
	}

	if _, err := repo.db.Exec(repo.db.Rebind(`DELETE FROM task_session_turns WHERE id = ?`), "turn-journal-turn-del"); err != nil {
		t.Fatalf("delete turn: %v", err)
	}
	var cutoffAfter int64
	if err := repo.db.Get(&cutoffAfter, `SELECT watermark FROM conversation_session_streams WHERE session_id = 'session-journal-turn-del'`); err != nil {
		t.Fatalf("read deletion cutoff: %v", err)
	}
	if cutoffAfter != cutoffBefore+1 {
		t.Fatalf("turn delete watermark = %d, want %d", cutoffAfter, cutoffBefore+1)
	}

	var versions int
	if err := repo.db.Get(&versions, `SELECT COUNT(*) FROM conversation_turn_versions WHERE turn_id = 'turn-journal-turn-del'`); err != nil {
		t.Fatalf("count turn versions: %v", err)
	}
	if versions != 3 {
		t.Fatalf("turn version count = %d, want two live versions plus tombstone", versions)
	}
	var tombstoneSequence int64
	if err := repo.db.Get(&tombstoneSequence, `
		SELECT row_sequence FROM conversation_turn_versions
		WHERE turn_id = 'turn-journal-turn-del' AND tombstone = TRUE
	`); err != nil {
		t.Fatalf("read deletion tombstone: %v", err)
	}
	if tombstoneSequence != cutoffAfter {
		t.Fatalf("tombstone sequence = %d, want deletion cutoff %d", tombstoneSequence, cutoffAfter)
	}

	assertTurnLiveAtCutoff(t, repo, "session-journal-turn-del", "turn-journal-turn-del", cutoffBefore, true)
	assertTurnLiveAtCutoff(t, repo, "session-journal-turn-del", "turn-journal-turn-del", cutoffAfter, false)

	var removalEvent string
	if err := repo.db.Get(&removalEvent, `SELECT event_type FROM conversation_session_events WHERE session_id = 'session-journal-turn-del' ORDER BY sequence DESC LIMIT 1`); err != nil {
		t.Fatalf("read last event type: %v", err)
	}
	if removalEvent != "session.turn.removed" {
		t.Fatalf("last event type = %q, want session.turn.removed", removalEvent)
	}
}

func assertTurnLiveAtCutoff(t *testing.T, repo *Repository, sessionID, turnID string, cutoff int64, want bool) {
	t.Helper()
	var count int
	if err := repo.db.Get(&count, repo.db.Rebind(`
		WITH ranked AS (
			SELECT tombstone,
				ROW_NUMBER() OVER (PARTITION BY turn_id ORDER BY row_sequence DESC) AS version_rank
			FROM conversation_turn_versions
			WHERE session_id = ? AND turn_id = ? AND row_sequence <= ?
		)
		SELECT COUNT(*) FROM ranked WHERE version_rank = 1 AND tombstone = FALSE
	`), sessionID, turnID, cutoff); err != nil {
		t.Fatalf("query turn at cutoff %d: %v", cutoff, err)
	}
	if got := count == 1; got != want {
		t.Fatalf("turn live at cutoff %d = %v, want %v", cutoff, got, want)
	}
}

func TestConversationJournalBackfillsExistingRowsIdempotently(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedForMsgTest(t, repo, "task-backfill", "session-backfill", "turn-backfill")
	message := &models.Message{
		ID: "message-backfill", TaskSessionID: "session-backfill", TaskID: "task-backfill",
		TurnID: "turn-backfill", AuthorType: models.MessageAuthorUser,
		Type: models.MessageTypeMessage, Content: "<kandev-system>hidden</kandev-system>existing",
	}
	if err := repo.CreateMessage(context.Background(), message); err != nil {
		t.Fatalf("create source message: %v", err)
	}
	if _, err := repo.db.Exec(`
		DELETE FROM conversation_session_events;
		DELETE FROM conversation_message_versions;
		DELETE FROM conversation_turn_versions;
		DELETE FROM conversation_session_streams;
	`); err != nil {
		t.Fatalf("clear journal: %v", err)
	}

	if err := repo.backfillConversationJournal(); err != nil {
		t.Fatalf("backfill conversation journal: %v", err)
	}
	if err := repo.backfillConversationJournal(); err != nil {
		t.Fatalf("repeat conversation journal backfill: %v", err)
	}
	var eventCount int
	if err := repo.db.Get(&eventCount, `SELECT COUNT(*) FROM conversation_session_events WHERE session_id = 'session-backfill'`); err != nil {
		t.Fatalf("count backfill events: %v", err)
	}
	if eventCount != 2 {
		t.Fatalf("backfill event count = %d, want 2", eventCount)
	}
	var content string
	if err := repo.db.Get(&content, `SELECT json_extract(payload, '$.content') FROM conversation_message_versions WHERE message_id = 'message-backfill'`); err != nil {
		t.Fatalf("read backfill message content: %v", err)
	}
	if content != "existing" {
		t.Fatalf("backfill content = %q, want existing", content)
	}
}

func TestConversationJournalSenderTaskIDRequiresString(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-journal-sender", "session-journal-sender", "turn-journal-sender")
	if err := repo.CreateMessage(ctx, &models.Message{
		ID: "message-sender-numeric", TaskSessionID: "session-journal-sender", TaskID: "task-journal-sender",
		TurnID: "turn-journal-sender", AuthorType: models.MessageAuthorUser,
		Type: models.MessageTypeMessage, Content: "numeric",
		Metadata: map[string]any{"sender_task_id": 42},
	}); err != nil {
		t.Fatalf("create numeric-sender message: %v", err)
	}
	if err := repo.CreateMessage(ctx, &models.Message{
		ID: "message-sender-string", TaskSessionID: "session-journal-sender", TaskID: "task-journal-sender",
		TurnID: "turn-journal-sender", AuthorType: models.MessageAuthorAgent,
		Type: models.MessageTypeMessage, Content: "string",
		Metadata: map[string]any{"sender_task_id": "task-sender-1"},
	}); err != nil {
		t.Fatalf("create string-sender message: %v", err)
	}

	var numericSender any
	if err := repo.db.Get(&numericSender, `SELECT json_extract(payload, '$.sender_task_id') FROM conversation_message_versions WHERE message_id = 'message-sender-numeric'`); err != nil {
		t.Fatalf("read numeric-sender payload: %v", err)
	}
	if numericSender != nil {
		t.Fatalf("numeric sender_task_id stored as %v, want NULL", numericSender)
	}
	var stringSender string
	if err := repo.db.Get(&stringSender, `SELECT json_extract(payload, '$.sender_task_id') FROM conversation_message_versions WHERE message_id = 'message-sender-string'`); err != nil {
		t.Fatalf("read string-sender payload: %v", err)
	}
	if stringSender != "task-sender-1" {
		t.Fatalf("string sender_task_id = %q, want task-sender-1", stringSender)
	}
}

// TestBackfillSkipsStaleMessageImageWithSameContent pins the round-9 fix: a
// concurrent update that only touches metadata (same content) must not let the
// stale pre-update backfill image win a later version.
func TestBackfillSkipsStaleMessageImageWithSameContent(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-backfill-stale", "session-backfill-stale", "turn-backfill-stale")
	now := time.Now().UTC().Truncate(time.Microsecond)
	message := &models.Message{
		ID: "message-stale", TaskSessionID: "session-backfill-stale", TaskID: "task-backfill-stale",
		TurnID: "turn-backfill-stale", AuthorType: models.MessageAuthorUser,
		Type: models.MessageTypeMessage, Content: "same-content",
		Metadata: map[string]any{"sender_task_id": "sender-1"}, CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.CreateMessage(ctx, message); err != nil {
		t.Fatalf("create message: %v", err)
	}
	// A concurrent metadata-only update journals its own version.
	message.Metadata = map[string]any{"sender_task_id": "sender-2"}
	message.UpdatedAt = now.Add(time.Minute)
	if err := repo.UpdateMessage(ctx, message); err != nil {
		t.Fatalf("update message: %v", err)
	}
	var versionsAfterUpdate int
	if err := repo.db.Get(&versionsAfterUpdate, `SELECT COUNT(*) FROM conversation_message_versions WHERE message_id = 'message-stale'`); err != nil {
		t.Fatalf("count versions: %v", err)
	}

	tx, err := repo.db.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	// The backfill seed reflects the pre-update image (same content, older
	// timestamp): it must be skipped entirely.
	stale := conversationJournalMessageSeed{
		ID: "message-stale", TaskSessionID: "session-backfill-stale", TaskID: "task-backfill-stale",
		TurnID: "turn-backfill-stale", AuthorType: string(models.MessageAuthorUser),
		Content: "same-content", MessageType: string(models.MessageTypeMessage),
		Metadata: `{"sender_task_id":"sender-1"}`, CreatedAt: now, UpdatedAt: now, PromptIndex: message.PromptIndex,
	}
	if err := repo.backfillConversationMessage(tx, stale); err != nil {
		_ = tx.Rollback()
		t.Fatalf("backfill stale image: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var versions int
	if err := repo.db.Get(&versions, `SELECT COUNT(*) FROM conversation_message_versions WHERE message_id = 'message-stale'`); err != nil {
		t.Fatalf("count versions: %v", err)
	}
	if versions != versionsAfterUpdate {
		t.Fatalf("stale backfill added a version: %d -> %d", versionsAfterUpdate, versions)
	}
}

// TestConversationJournalSanitizeMigrationSkipsMalformedRows pins the
// round-9 fix: a pre-existing malformed payload must not abort the boot-time
// sanitize migration; the row is preserved verbatim so the mirror path can
// turn it into a durable poison record.
func TestConversationJournalSanitizeMigrationSkipsMalformedRows(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-journal-malformed", "session-journal-malformed", "turn-journal-malformed")
	message := &models.Message{
		ID: "message-malformed", TaskSessionID: "session-journal-malformed", TaskID: "task-journal-malformed",
		TurnID: "turn-journal-malformed", AuthorType: models.MessageAuthorUser,
		Type: models.MessageTypeMessage, Content: "placeholder",
	}
	if err := repo.CreateMessage(ctx, message); err != nil {
		t.Fatalf("create message: %v", err)
	}
	malformed := `{"content":"not valid json`
	if _, err := repo.db.Exec(`
		UPDATE conversation_message_versions SET payload = ? WHERE message_id = 'message-malformed';
		UPDATE conversation_session_events SET payload = ? WHERE session_id = 'session-journal-malformed' AND event_type = 'message.added';
	`, malformed, malformed); err != nil {
		t.Fatalf("seed malformed rows: %v", err)
	}

	if err := repo.initSQLiteConversationJournalTriggers(); err != nil {
		t.Fatalf("re-init triggers (runs sanitize migration): %v", err)
	}
	var versionPayload string
	if err := repo.db.Get(&versionPayload, `SELECT payload FROM conversation_message_versions WHERE message_id = 'message-malformed'`); err != nil {
		t.Fatalf("read version payload: %v", err)
	}
	if versionPayload != malformed {
		t.Fatalf("malformed version payload was rewritten to %q", versionPayload)
	}
	var eventPayload string
	if err := repo.db.Get(&eventPayload, `SELECT payload FROM conversation_session_events WHERE session_id = 'session-journal-malformed' AND event_type = 'message.added'`); err != nil {
		t.Fatalf("read event payload: %v", err)
	}
	if eventPayload != malformed {
		t.Fatalf("malformed event payload was rewritten to %q", eventPayload)
	}
}

func TestConversationJournalTurnCompletionIncludesHadOutput(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-journal-empty", "session-journal-empty", "turn-journal-empty")

	if err := repo.CompleteTurn(ctx, "turn-journal-empty"); err != nil {
		t.Fatalf("complete turn: %v", err)
	}

	var raw string
	if err := repo.db.Get(&raw, `
		SELECT payload FROM conversation_session_events
		WHERE session_id = 'session-journal-empty' AND event_type = 'session.turn.completed'
	`); err != nil {
		t.Fatalf("read completed turn event: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("decode completed turn event: %v", err)
	}
	if hadOutput, ok := payload["had_output"].(bool); !ok || hadOutput {
		t.Fatalf("had_output = %#v, want false", payload["had_output"])
	}
}
