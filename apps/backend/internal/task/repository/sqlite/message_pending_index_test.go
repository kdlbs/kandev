package sqlite

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

const pendingIDLookupIndexName = "idx_messages_metadata_pending_id_lookup_ordered"

// TestPendingIDLookupIndexFreshReplayAndExistingUpgrade covers the additive
// startup path: fresh initialization creates the lookup index, replay keeps it
// idempotent, and an existing database can add it without changing messages or
// removing the session-scoped index.
func TestPendingIDLookupIndexFreshReplayAndExistingUpgrade(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-pending-index", "session-pending-index", "turn-pending-index")
	message := &models.Message{
		ID:            "message-pending-index",
		TaskID:        "task-pending-index",
		TaskSessionID: "session-pending-index",
		TurnID:        "turn-pending-index",
		AuthorType:    models.MessageAuthorAgent,
		Type:          models.MessageTypeClarificationRequest,
		Content:       "question",
		Metadata: map[string]any{
			"pending_id":  "pending-index",
			"question_id": "q1",
			"status":      "pending",
		},
		CreatedAt: time.Date(2026, 9, 5, 20, 0, 0, 0, time.UTC),
	}
	if err := repo.CreateMessage(ctx, message); err != nil {
		t.Fatalf("create message: %v", err)
	}

	assertSQLiteIndexExists(t, repo, pendingIDLookupIndexName)
	assertSQLiteIndexExists(t, repo, "idx_messages_metadata_pending_id")

	var before int
	if err := repo.db.Get(&before, `SELECT COUNT(*) FROM task_session_messages`); err != nil {
		t.Fatalf("count messages before upgrade replay: %v", err)
	}
	if _, err := repo.db.Exec("DROP INDEX " + pendingIDLookupIndexName); err != nil {
		t.Fatalf("drop lookup index to simulate legacy database: %v", err)
	}
	if err := repo.ensureMessageMetadataIndexes(); err != nil {
		t.Fatalf("upgrade legacy database: %v", err)
	}
	if err := repo.ensureMessageMetadataIndexes(); err != nil {
		t.Fatalf("replay lookup index creation: %v", err)
	}

	assertSQLiteIndexExists(t, repo, pendingIDLookupIndexName)
	assertSQLiteIndexExists(t, repo, "idx_messages_metadata_pending_id")
	var after int
	if err := repo.db.Get(&after, `SELECT COUNT(*) FROM task_session_messages`); err != nil {
		t.Fatalf("count messages after upgrade replay: %v", err)
	}
	if after != before {
		t.Fatalf("message count after index upgrade = %d, want %d", after, before)
	}
	messages, err := repo.FindMessagesByPendingID(ctx, "pending-index")
	if err != nil {
		t.Fatalf("find pending message after upgrade: %v", err)
	}
	if len(messages) != 1 || messages[0].ID != message.ID {
		t.Fatalf("pending lookup after upgrade = %v, want [%s]", messageIDs(messages), message.ID)
	}
}

// TestPendingIDLookupUsesLeadingIndexForBundleRead proves the pending-ID-only
// query shape can use the new leading expression index even when unrelated
// message history is much larger than the target bundle.
func TestPendingIDLookupUsesLeadingIndexForBundleRead(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-pending-plan", "session-pending-plan", "turn-pending-plan")
	now := time.Date(2026, 9, 5, 21, 0, 0, 0, time.UTC)
	insertPendingIndexMessage(t, repo, "message-pending-target", "session-pending-plan", "task-pending-plan", "turn-pending-plan", "target-pending", now)
	for i := 0; i < 2000; i++ {
		insertPendingIndexMessage(
			t,
			repo,
			fmt.Sprintf("message-pending-unrelated-%04d", i),
			"session-pending-plan",
			"task-pending-plan",
			"turn-pending-plan",
			fmt.Sprintf("unrelated-pending-%04d", i),
			now.Add(time.Duration(i+1)*time.Microsecond),
		)
	}
	if _, err := repo.db.Exec(`ANALYZE task_session_messages`); err != nil {
		t.Fatalf("analyze messages: %v", err)
	}

	query := "EXPLAIN QUERY PLAN\n" + findMessagesByPendingIDQuery(repo.db.DriverName())
	rows, err := repo.db.QueryxContext(ctx, repo.db.Rebind(query), "target-pending")
	if err != nil {
		t.Fatalf("explain pending lookup: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var details []string
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatalf("scan pending lookup plan: %v", err)
		}
		details = append(details, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read pending lookup plan: %v", err)
	}
	if !strings.Contains(strings.Join(details, "\n"), pendingIDLookupIndexName) {
		t.Fatalf("pending lookup plan = %v, want %s", details, pendingIDLookupIndexName)
	}
}

func TestFindMessagesByPendingIDOrdersEqualTimestampsByID(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-pending-order", "session-pending-order", "turn-pending-order")
	if _, err := repo.db.Exec("DROP INDEX " + pendingIDLookupIndexName); err != nil {
		t.Fatalf("drop ordered pending-ID index: %v", err)
	}
	if _, err := repo.db.Exec("DROP INDEX idx_messages_metadata_pending_id"); err != nil {
		t.Fatalf("drop bare pending-ID index: %v", err)
	}

	createdAt := time.Date(2026, 9, 5, 21, 30, 0, 0, time.UTC)
	insertPendingIndexMessage(t, repo, "message-z", "session-pending-order", "task-pending-order", "turn-pending-order", "same-time", createdAt)
	insertPendingIndexMessage(t, repo, "message-a", "session-pending-order", "task-pending-order", "turn-pending-order", "same-time", createdAt)

	messages, err := repo.FindMessagesByPendingID(ctx, "same-time")
	if err != nil {
		t.Fatalf("find same-time pending messages: %v", err)
	}
	if got := messageIDs(messages); strings.Join(got, ",") != "message-a,message-z" {
		t.Fatalf("same-time pending messages = %v, want [message-a message-z]", got)
	}
}

func assertSQLiteIndexExists(t *testing.T, repo *Repository, indexName string) {
	t.Helper()
	var got string
	if err := repo.db.Get(&got, `SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?`, indexName); err != nil {
		t.Fatalf("index %s is missing: %v", indexName, err)
	}
	if got != indexName {
		t.Fatalf("index name = %q, want %q", got, indexName)
	}
}

func insertPendingIndexMessage(t *testing.T, repo *Repository, id, sessionID, taskID, turnID, pendingID string, createdAt time.Time) {
	t.Helper()
	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_session_messages
			(id, task_session_id, task_id, turn_id, author_type, author_id, content,
			 requests_input, type, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'agent', '', 'question', 1, 'clarification_request', ?, ?, ?)
	`), id, sessionID, taskID, turnID,
		fmt.Sprintf(`{"pending_id":%q,"question_id":"q1","status":"pending"}`, pendingID),
		createdAt, createdAt)
	if err != nil {
		t.Fatalf("insert pending message %s: %v", id, err)
	}
}
