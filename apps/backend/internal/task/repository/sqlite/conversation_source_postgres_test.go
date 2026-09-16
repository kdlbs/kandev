package sqlite

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestConversationPostgresSource(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	const (
		taskID    = "task-conversation-source-pg"
		sessionID = "session-conversation-source-pg"
		turnID    = "turn-conversation-source-pg"
	)
	seedPostgresConversationSource(t, repo, taskID, sessionID, turnID)
	taskFilter := taskID

	if got := readPostgresConversationRevision(t, repo, sessionID); got != 1 {
		t.Fatalf("seed revision = %d, want 1", got)
	}
	firstAt := time.Date(2026, 9, 16, 12, 0, 0, 123456, time.UTC)
	insertPostgresConversationMessage(t, repo, "message-source-pg-a", sessionID, taskID, turnID, "user", "first", firstAt, 1)
	insertPostgresConversationMessage(t, repo, "message-source-pg-b", sessionID, taskID, turnID, "agent", "second", firstAt, 0)
	if got := readPostgresConversationRevision(t, repo, sessionID); got != 3 {
		t.Fatalf("insert revision = %d, want 3", got)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		UPDATE task_session_turns SET completed_at = ?, updated_at = ? WHERE id = ?
	`), firstAt, firstAt, turnID); err != nil {
		t.Fatalf("complete turn: %v", err)
	}
	if got := readPostgresConversationRevision(t, repo, sessionID); got != 4 {
		t.Fatalf("turn completion revision = %d, want 4", got)
	}

	if _, err := repo.db.Exec(repo.db.Rebind(`
		UPDATE task_session_messages SET content = ?, updated_at = ? WHERE id = ?
	`), "updated", firstAt.Add(time.Second), "message-source-pg-a"); err != nil {
		t.Fatalf("direct message update: %v", err)
	}
	if got := readPostgresConversationRevision(t, repo, sessionID); got != 5 {
		t.Fatalf("update revision = %d, want 5", got)
	}

	page, err := repo.ReadConversationMessagesPage(ctx, models.ConversationMessagePageRequest{
		SessionID: sessionID,
		TaskID:    &taskFilter,
		Authors:   []string{"user"},
		Sort:      "asc",
		Limit:     1,
	})
	if err != nil {
		t.Fatalf("read filtered message page: %v", err)
	}
	if page.Revision != 5 || len(page.Messages) != 1 || page.Messages[0].ID != "message-source-pg-a" || page.Messages[0].Content != "updated" {
		t.Fatalf("filtered page = %+v, want updated user message at revision 5", page)
	}
	if page.HasMore || page.CursorID != "" {
		t.Fatalf("filtered page continuation = hasMore %v cursor %q, want no continuation", page.HasMore, page.CursorID)
	}

	allPage, err := repo.ReadConversationMessagesPage(ctx, models.ConversationMessagePageRequest{
		SessionID: sessionID,
		Sort:      "asc",
		Limit:     1,
	})
	if err != nil {
		t.Fatalf("read first message page: %v", err)
	}
	if len(allPage.Messages) != 1 || allPage.Messages[0].ID != "message-source-pg-a" || !allPage.HasMore || allPage.CursorID != "message-source-pg-a" {
		t.Fatalf("first page = %+v, want first message with continuation", allPage)
	}
	allPage, err = repo.ReadConversationMessagesPage(ctx, models.ConversationMessagePageRequest{
		SessionID: sessionID,
		Sort:      "asc",
		CursorID:  allPage.CursorID,
		Limit:     1,
	})
	if err != nil {
		t.Fatalf("read second message page: %v", err)
	}
	if len(allPage.Messages) != 1 || allPage.Messages[0].ID != "message-source-pg-b" || allPage.HasMore {
		t.Fatalf("second page = %+v, want final message", allPage)
	}

	if _, err := repo.db.Exec(repo.db.Rebind(`DELETE FROM task_session_messages WHERE id = ?`), "message-source-pg-b"); err != nil {
		t.Fatalf("direct message delete: %v", err)
	}
	if got := readPostgresConversationRevision(t, repo, sessionID); got != 6 {
		t.Fatalf("delete revision = %d, want 6", got)
	}

	const movedSessionID = "session-conversation-source-pg-moved"
	const movedTurnID = "turn-conversation-source-pg-moved"
	seedPostgresConversationSource(t, repo, taskID+"-moved", movedSessionID, movedTurnID)
	if _, err := repo.db.Exec(repo.db.Rebind(`
		UPDATE task_session_messages SET task_session_id = ?, task_id = ? WHERE id = ?
	`), movedSessionID, taskID+"-moved", "message-source-pg-a"); err != nil {
		t.Fatalf("move message between sessions: %v", err)
	}
	if got := readPostgresConversationRevision(t, repo, sessionID); got != 7 {
		t.Fatalf("source-session move revision = %d, want 7", got)
	}
	if got := readPostgresConversationRevision(t, repo, movedSessionID); got != 2 {
		t.Fatalf("destination-session move revision = %d, want 2", got)
	}

	if _, err := repo.db.Exec(repo.db.Rebind(`
		UPDATE task_session_turns SET task_session_id = ? WHERE id = ?
	`), movedSessionID, turnID); err != nil {
		t.Fatalf("move turn between sessions: %v", err)
	}
	if got := readPostgresConversationRevision(t, repo, sessionID); got != 8 {
		t.Fatalf("source-session turn move revision = %d, want 8", got)
	}
	if got := readPostgresConversationRevision(t, repo, movedSessionID); got != 3 {
		t.Fatalf("destination-session turn move revision = %d, want 3", got)
	}

	const (
		snapshotTaskID    = "task-conversation-source-pg-snapshot"
		snapshotSessionID = "session-conversation-source-pg-snapshot"
		snapshotTurnID    = "turn-conversation-source-pg-snapshot"
	)
	seedPostgresConversationSource(t, repo, snapshotTaskID, snapshotSessionID, snapshotTurnID)
	snapshotAt := firstAt.Add(2 * time.Minute)
	insertPostgresConversationMessage(t, repo, "message-source-pg-snapshot", snapshotSessionID, snapshotTaskID, snapshotTurnID, "agent", "before snapshot", snapshotAt, 0)
	secondDB := openSecondPostgresConnection(t, dsn, db)
	readTx, err := repo.beginConversationSourceRead(ctx)
	if err != nil {
		t.Fatalf("begin repeatable-read source transaction: %v", err)
	}
	snapshotState, err := readConversationState(ctx, readTx, repo.db.DriverName(), snapshotSessionID)
	if err != nil {
		_ = readTx.Rollback()
		t.Fatalf("read repeatable-read source state: %v", err)
	}
	var snapshotContent string
	if err := readTx.QueryRowxContext(ctx, readTx.Rebind(`SELECT content FROM task_session_messages WHERE id = ?`), "message-source-pg-snapshot").Scan(&snapshotContent); err != nil {
		_ = readTx.Rollback()
		t.Fatalf("read repeatable-read source row: %v", err)
	}
	if _, err := secondDB.Exec(secondDB.Rebind(`UPDATE task_session_messages SET content = ? WHERE id = ?`), "after snapshot", "message-source-pg-snapshot"); err != nil {
		_ = readTx.Rollback()
		t.Fatalf("update source row after snapshot: %v", err)
	}
	currentState, err := readConversationState(ctx, readTx, repo.db.DriverName(), snapshotSessionID)
	if err != nil {
		_ = readTx.Rollback()
		t.Fatalf("read repeatable-read state after concurrent commit: %v", err)
	}
	if snapshotState.Revision != 2 || currentState.Revision != snapshotState.Revision || snapshotContent != "before snapshot" {
		_ = readTx.Rollback()
		t.Fatalf("repeatable-read snapshot = state before %+v, state after %+v, content %q", snapshotState, currentState, snapshotContent)
	}
	if err := readTx.Commit(); err != nil {
		t.Fatalf("commit repeatable-read source transaction: %v", err)
	}
	if got := readPostgresConversationRevision(t, repo, snapshotSessionID); got != 3 {
		t.Fatalf("post-snapshot revision = %d, want 3", got)
	}

	rollbackTx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin rollback transaction: %v", err)
	}
	if _, err := rollbackTx.Exec(repo.db.Rebind(`
		INSERT INTO task_session_messages
			(id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at, updated_at, prompt_seq)
		VALUES (?, ?, ?, ?, 'agent', '', ?, 0, 'message', '{}', ?, ?, 0)
	`), "message-source-pg-rolled-back", movedSessionID, taskID+"-moved", movedTurnID, "rolled back", firstAt, firstAt); err != nil {
		t.Fatalf("insert rollback message: %v", err)
	}
	if err := rollbackTx.Rollback(); err != nil {
		t.Fatalf("rollback source mutation: %v", err)
	}
	if got := readPostgresConversationRevision(t, repo, movedSessionID); got != 3 {
		t.Fatalf("rolled-back revision = %d, want 3", got)
	}
	var rolledBackCount int
	if err := repo.db.Get(&rolledBackCount, repo.db.Rebind(`SELECT COUNT(*) FROM task_session_messages WHERE id = ?`), "message-source-pg-rolled-back"); err != nil {
		t.Fatalf("count rolled-back message: %v", err)
	}
	if rolledBackCount != 0 {
		t.Fatalf("rolled-back message count = %d, want 0", rolledBackCount)
	}

	if _, err := repo.db.Exec(repo.db.Rebind(`DELETE FROM task_sessions WHERE id = ?`), movedSessionID); err != nil {
		t.Fatalf("delete session cascade: %v", err)
	}
	for _, table := range []string{"task_session_messages", "task_session_turns", "conversation_session_revisions"} {
		var count int
		if err := repo.db.Get(&count, repo.db.Rebind(`SELECT COUNT(*) FROM `+table+` WHERE `+map[string]string{
			"task_session_messages":          "task_session_id",
			"task_session_turns":             "task_session_id",
			"conversation_session_revisions": "session_id",
		}[table]+` = ?`), movedSessionID); err != nil {
			t.Fatalf("count %s after session cascade: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("%s rows after session cascade = %d, want 0", table, count)
		}
	}

	var revisionRows, sessionCount int
	if err := repo.db.Get(&revisionRows, `SELECT COUNT(*) FROM conversation_session_revisions`); err != nil {
		t.Fatalf("count revision rows: %v", err)
	}
	if err := repo.db.Get(&sessionCount, `SELECT COUNT(*) FROM task_sessions`); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if revisionRows > sessionCount {
		t.Fatalf("revision rows = %d, session rows = %d, revision table is not bounded by sessions", revisionRows, sessionCount)
	}

	turnPage, err := repo.ReadConversationTurnsPage(ctx, models.ConversationTurnPageRequest{
		SessionID: sessionID,
		Sort:      "asc",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("read moved turn page: %v", err)
	}
	if len(turnPage.Turns) != 0 {
		t.Fatalf("old session turn page after move = %+v, want empty", turnPage.Turns)
	}
}

func TestConversationPostgresReceipts(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	const (
		taskID    = "task-conversation-receipts-pg"
		sessionID = "session-conversation-receipts-pg"
		turnID    = "turn-conversation-receipts-pg"
	)
	seedPostgresConversationSource(t, repo, taskID, sessionID, turnID)

	message := &models.Message{
		ID:            "message-conversation-receipts-pg",
		TaskSessionID: sessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		AuthorType:    models.MessageAuthorAgent,
		Type:          models.MessageTypeMessage,
		Content:       "receipt one",
		CreatedAt:     time.Date(2026, 9, 16, 12, 0, 0, 123456, time.UTC),
	}
	receipt, err := repo.CreateMessageWithConversationReceipt(ctx, message)
	if err != nil {
		t.Fatalf("create message receipt: %v", err)
	}
	assertPostgresConversationReceipt(t, receipt, sessionID, 1, 2, true, models.ConversationMutationUpsert, models.ConversationEntityMessage, message.ID)
	if receipt.Operations[0].Message == nil || receipt.Operations[0].Message.Content != message.Content {
		t.Fatalf("create receipt payload = %+v, want source content", receipt.Operations[0])
	}

	message.Content = "receipt two"
	receipt, err = repo.UpdateMessageWithConversationReceipt(ctx, message)
	if err != nil {
		t.Fatalf("update message receipt: %v", err)
	}
	assertPostgresConversationReceipt(t, receipt, sessionID, 2, 3, true, models.ConversationMutationUpsert, models.ConversationEntityMessage, message.ID)
	if receipt.Operations[0].Message == nil || receipt.Operations[0].Message.Content != "receipt two" {
		t.Fatalf("update receipt payload = %+v, want updated source content", receipt.Operations[0])
	}

	receipt, err = repo.DeleteMessageWithConversationReceipt(ctx, message.ID)
	if err != nil {
		t.Fatalf("delete message receipt: %v", err)
	}
	assertPostgresConversationReceipt(t, receipt, sessionID, 3, 4, true, models.ConversationMutationRemove, models.ConversationEntityMessage, message.ID)
	if receipt.Operations[0].Message != nil {
		t.Fatalf("delete receipt unexpectedly includes message payload: %+v", receipt.Operations[0])
	}

	newTurn := &models.Turn{
		ID:            "turn-conversation-receipts-pg-created",
		TaskSessionID: sessionID,
		TaskID:        taskID,
		StartedAt:     time.Date(2026, 9, 16, 12, 1, 0, 0, time.UTC),
		CreatedAt:     time.Date(2026, 9, 16, 12, 1, 0, 0, time.UTC),
	}
	receipt, err = repo.CreateTurnWithConversationReceipt(ctx, newTurn)
	if err != nil {
		t.Fatalf("create turn receipt: %v", err)
	}
	assertPostgresConversationReceipt(t, receipt, sessionID, 4, 5, true, models.ConversationMutationUpsert, models.ConversationEntityTurn, newTurn.ID)

	receipt, err = repo.CompleteTurnWithConversationReceipt(ctx, newTurn.ID)
	if err != nil {
		t.Fatalf("complete turn receipt: %v", err)
	}
	assertPostgresConversationReceipt(t, receipt, sessionID, 5, 6, true, models.ConversationMutationUpsert, models.ConversationEntityTurn, newTurn.ID)
	if receipt.Operations[0].Turn == nil || receipt.Operations[0].Turn.CompletedAt == nil {
		t.Fatalf("completed turn receipt = %+v, want completed source turn", receipt.Operations[0])
	}

	badMessage := &models.Message{
		ID:            "message-conversation-receipts-pg-rollback",
		TaskSessionID: sessionID,
		TaskID:        taskID,
		TurnID:        "turn-conversation-receipts-pg-missing",
		AuthorType:    models.MessageAuthorAgent,
		Content:       "must roll back",
	}
	if _, err := repo.CreateMessageWithConversationReceipt(ctx, badMessage); err == nil {
		t.Fatal("message receipt with missing turn succeeded")
	}
	if got := readPostgresConversationRevision(t, repo, sessionID); got != 6 {
		t.Fatalf("failed receipt revision = %d, want 6", got)
	}
	var failedMessageCount int
	if err := repo.db.Get(&failedMessageCount, repo.db.Rebind(`SELECT COUNT(*) FROM task_session_messages WHERE id = ?`), badMessage.ID); err != nil {
		t.Fatalf("count rolled-back receipt message: %v", err)
	}
	if failedMessageCount != 0 {
		t.Fatalf("rolled-back receipt message count = %d, want 0", failedMessageCount)
	}

	secondDB := openSecondPostgresConnection(t, dsn, db)
	secondRepo, err := NewWithDB(secondDB, secondDB, nil)
	if err != nil {
		t.Fatalf("init second postgres repository: %v", err)
	}
	start := make(chan struct{})
	results := make(chan postgresReceiptResult, 2)
	for index, writer := range []*Repository{repo, secondRepo} {
		index, writer := index, writer
		go func() {
			<-start
			concurrentMessage := &models.Message{
				ID:            fmt.Sprintf("message-conversation-receipts-pg-concurrent-%d", index),
				TaskSessionID: sessionID,
				TaskID:        taskID,
				TurnID:        turnID,
				AuthorType:    models.MessageAuthorAgent,
				Content:       fmt.Sprintf("concurrent receipt %d", index),
			}
			concurrentReceipt, receiptErr := writer.CreateMessageWithConversationReceipt(context.Background(), concurrentMessage)
			results <- postgresReceiptResult{receipt: concurrentReceipt, err: receiptErr}
		}()
	}
	close(start)
	concurrentCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	concurrentReceipts := make([]*models.ConversationMutationReceipt, 0, 2)
	for len(concurrentReceipts) < 2 {
		select {
		case result := <-results:
			if result.err != nil {
				t.Fatalf("concurrent receipt write: %v", result.err)
			}
			concurrentReceipts = append(concurrentReceipts, result.receipt)
		case <-concurrentCtx.Done():
			t.Fatal("timed out waiting for concurrent receipt writes")
		}
	}
	sort.Slice(concurrentReceipts, func(left, right int) bool {
		return concurrentReceipts[left].BaseRevision < concurrentReceipts[right].BaseRevision
	})
	if concurrentReceipts[0].BaseRevision != 6 || concurrentReceipts[0].Revision != 7 || !concurrentReceipts[0].Complete {
		t.Fatalf("first concurrent receipt = %+v, want interval 6..7", concurrentReceipts[0])
	}
	if concurrentReceipts[1].BaseRevision != 7 || concurrentReceipts[1].Revision != 8 || !concurrentReceipts[1].Complete {
		t.Fatalf("second concurrent receipt = %+v, want interval 7..8", concurrentReceipts[1])
	}

	// A single receipt cannot describe two direct source changes. The receipt
	// must expose the larger transaction interval so the subscriber requests a
	// source reconciliation instead of applying an incomplete delta.
	intervalTx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin multi-row receipt transaction: %v", err)
	}
	base, err := repo.ensureConversationRevisionTx(ctx, intervalTx, sessionID)
	if err != nil {
		_ = intervalTx.Rollback()
		t.Fatalf("lock multi-row receipt transaction: %v", err)
	}
	for index := 0; index < 2; index++ {
		id := fmt.Sprintf("message-conversation-receipts-pg-multi-%d", index)
		if _, err := intervalTx.Exec(repo.db.Rebind(`
			INSERT INTO task_session_messages
				(id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at, updated_at, prompt_seq)
			VALUES (?, ?, ?, ?, 'agent', '', ?, 0, 'message', '{}', ?, ?, 0)
		`), id, sessionID, taskID, turnID, "multi-row", time.Now().UTC(), time.Now().UTC()); err != nil {
			_ = intervalTx.Rollback()
			t.Fatalf("insert multi-row receipt message %d: %v", index, err)
		}
	}
	intervalReceipt := &models.ConversationMutationReceipt{}
	if err := repo.populateConversationMessageReceipt(ctx, intervalTx, intervalReceipt, base, &models.Message{
		ID: "message-conversation-receipts-pg-multi-0", TaskSessionID: sessionID, TaskID: taskID,
		TurnID: turnID, AuthorType: models.MessageAuthorAgent,
	}, models.ConversationMutationUpsert); err != nil {
		_ = intervalTx.Rollback()
		t.Fatalf("build incomplete receipt: %v", err)
	}
	if intervalReceipt.BaseRevision != 8 || intervalReceipt.Revision != 10 || intervalReceipt.Complete {
		_ = intervalTx.Rollback()
		t.Fatalf("incomplete receipt = %+v, want interval 8..10 with complete=false", intervalReceipt)
	}
	if err := intervalTx.Commit(); err != nil {
		t.Fatalf("commit multi-row receipt transaction: %v", err)
	}
}

type postgresReceiptResult struct {
	receipt *models.ConversationMutationReceipt
	err     error
}

func assertPostgresConversationReceipt(
	t *testing.T,
	receipt *models.ConversationMutationReceipt,
	sessionID string,
	base, revision int64,
	complete bool,
	kind models.ConversationMutationKind,
	entity models.ConversationEntityKind,
	id string,
) {
	t.Helper()
	if receipt == nil {
		t.Fatal("conversation receipt is nil")
	}
	if receipt.SessionID != sessionID || receipt.BaseRevision != base || receipt.Revision != revision || receipt.Complete != complete || len(receipt.Operations) != 1 {
		t.Fatalf("conversation receipt = %+v, want session %s interval %d..%d complete=%v", receipt, sessionID, base, revision, complete)
	}
	operation := receipt.Operations[0]
	if operation.Kind != kind || operation.Entity != entity || operation.ID != id || operation.SessionID != sessionID {
		t.Fatalf("conversation receipt operation = %+v, want %s/%s/%s", operation, kind, entity, id)
	}
}

func TestConversationPostgresCleanup(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	const (
		taskID    = "task-conversation-cleanup-pg"
		sessionID = "session-conversation-cleanup-pg"
		turnID    = "turn-conversation-cleanup-pg"
		messageID = "message-conversation-cleanup-pg"
	)
	seedPostgresConversationSource(t, repo, taskID, sessionID, turnID)
	sourceAt := time.Date(2026, 9, 16, 12, 0, 0, 123456, time.UTC)
	insertPostgresConversationMessage(t, repo, messageID, sessionID, taskID, turnID, "agent", "source bytes survive cleanup", sourceAt, 0)
	wantSource := readPostgresConversationSentinel(t, repo, messageID)
	wantIndexes := readPostgresConversationSourceIndexes(t, repo)

	if _, err := repo.db.ExecContext(ctx, postgresLegacyConversationJournalDDL); err != nil {
		t.Fatalf("create comparison-base legacy postgres fixture: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, postgresUnrelatedCleanupObjectDDL); err != nil {
		t.Fatalf("create unrelated postgres cleanup fixture: %v", err)
	}
	assertPostgresLegacyConversationObjects(t, repo, true)
	assertPostgresUnrelatedCleanupObject(t, repo, true)

	repo.failConversationJournalCleanupAfter = "statement-6"
	if err := repo.cleanupLegacyConversationJournal(); err == nil {
		t.Fatal("cleanup failure injection succeeded")
	}
	assertPostgresLegacyConversationObjects(t, repo, true)
	assertPostgresUnrelatedCleanupObject(t, repo, true)
	assertPostgresConversationSentinel(t, repo, messageID, wantSource)
	if got := readPostgresConversationSourceIndexes(t, repo); !reflect.DeepEqual(got, wantIndexes) {
		t.Fatalf("source indexes after rolled-back cleanup = %#v, want %#v", got, wantIndexes)
	}

	repo.failConversationJournalCleanupAfter = ""
	if err := repo.cleanupLegacyConversationJournal(); err != nil {
		t.Fatalf("retry comparison-base cleanup: %v", err)
	}
	assertPostgresLegacyConversationObjects(t, repo, false)
	assertPostgresUnrelatedCleanupObject(t, repo, true)
	assertPostgresConversationSentinel(t, repo, messageID, wantSource)
	if got := readPostgresConversationSourceIndexes(t, repo); !reflect.DeepEqual(got, wantIndexes) {
		t.Fatalf("source indexes after cleanup = %#v, want %#v", got, wantIndexes)
	}
	if err := repo.cleanupLegacyConversationJournal(); err != nil {
		t.Fatalf("repeated fresh-schema cleanup: %v", err)
	}
}

type postgresConversationSentinel struct {
	Content   string
	Metadata  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func readPostgresConversationSentinel(t *testing.T, repo *Repository, messageID string) postgresConversationSentinel {
	t.Helper()
	var sentinel postgresConversationSentinel
	if err := repo.db.QueryRowx(repo.db.Rebind(`
		SELECT content, metadata, created_at, updated_at
		FROM task_session_messages WHERE id = ?
	`), messageID).Scan(&sentinel.Content, &sentinel.Metadata, &sentinel.CreatedAt, &sentinel.UpdatedAt); err != nil {
		t.Fatalf("read source sentinel %s: %v", messageID, err)
	}
	return sentinel
}

func assertPostgresConversationSentinel(t *testing.T, repo *Repository, messageID string, want postgresConversationSentinel) {
	t.Helper()
	got := readPostgresConversationSentinel(t, repo, messageID)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("source sentinel after cleanup = %#v, want %#v", got, want)
	}
}

func readPostgresConversationSourceIndexes(t *testing.T, repo *Repository) map[string]string {
	t.Helper()
	rows, err := repo.db.Queryx(`
		SELECT tablename, indexname, indexdef
		FROM pg_indexes
		WHERE schemaname = current_schema()
		  AND tablename IN ('task_session_messages', 'task_session_turns', 'conversation_session_revisions')
		ORDER BY tablename, indexname
	`)
	if err != nil {
		t.Fatalf("read conversation source indexes: %v", err)
	}
	defer func() { _ = rows.Close() }()
	indexes := make(map[string]string)
	for rows.Next() {
		var table, name, definition string
		if err := rows.Scan(&table, &name, &definition); err != nil {
			t.Fatalf("scan conversation source index: %v", err)
		}
		indexes[table+":"+name] = definition
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate conversation source indexes: %v", err)
	}
	return indexes
}

func assertPostgresLegacyConversationObjects(t *testing.T, repo *Repository, wantPresent bool) {
	t.Helper()
	for _, table := range []string{
		"conversation_session_streams",
		"conversation_session_events",
		"conversation_message_versions",
		"conversation_turn_versions",
		"conversation_journal_meta",
	} {
		assertPostgresCleanupObject(t, repo, "table", table, wantPresent)
	}
	for _, function := range []string{
		"conversation_message_journal",
		"conversation_turn_journal",
		"conversation_session_delete_journal",
	} {
		assertPostgresCleanupObject(t, repo, "function", function, wantPresent)
	}
	for _, trigger := range []string{
		"conversation_message_journal_trigger",
		"conversation_turn_journal_trigger",
		"conversation_session_delete_trigger",
	} {
		assertPostgresCleanupObject(t, repo, "trigger", trigger, wantPresent)
	}
}

func assertPostgresUnrelatedCleanupObject(t *testing.T, repo *Repository, wantPresent bool) {
	t.Helper()
	assertPostgresCleanupObject(t, repo, "table", "conversation_cleanup_unrelated", wantPresent)
	assertPostgresCleanupObject(t, repo, "function", "conversation_cleanup_unrelated", wantPresent)
	assertPostgresCleanupObject(t, repo, "trigger", "conversation_cleanup_unrelated_trigger", wantPresent)
}

func assertPostgresCleanupObject(t *testing.T, repo *Repository, kind, name string, wantPresent bool) {
	t.Helper()
	var present bool
	var err error
	switch kind {
	case "table":
		err = repo.db.Get(&present, `
			SELECT EXISTS (
				SELECT 1 FROM pg_class c
				JOIN pg_namespace n ON n.oid = c.relnamespace
				WHERE n.nspname = current_schema()
				  AND c.relname = $1 AND c.relkind IN ('r', 'p')
			)`, name)
	case "function":
		err = repo.db.Get(&present, `
			SELECT EXISTS (
				SELECT 1 FROM pg_proc p
				JOIN pg_namespace n ON n.oid = p.pronamespace
				WHERE n.nspname = current_schema() AND p.proname = $1 AND p.pronargs = 0
			)`, name)
	case "trigger":
		err = repo.db.Get(&present, `
			SELECT EXISTS (
				SELECT 1
				FROM pg_trigger tr
				JOIN pg_class c ON c.oid = tr.tgrelid
				JOIN pg_namespace n ON n.oid = c.relnamespace
				WHERE n.nspname = current_schema() AND tr.tgname = $1 AND NOT tr.tgisinternal
			)`, name)
	default:
		t.Fatalf("unsupported postgres cleanup object kind %q", kind)
	}
	if err != nil {
		t.Fatalf("inspect postgres %s %q: %v", kind, name, err)
	}
	if present != wantPresent {
		t.Fatalf("postgres %s %q present = %v, want %v", kind, name, present, wantPresent)
	}
}

// postgresLegacyConversationJournalDDL is copied from the comparison base's
// conversationJournalTables declaration. The fixture intentionally retains
// the original PostgreSQL types and indexes so cleanup is tested against the
// upgrade shape, not a SQLite approximation.
const postgresLegacyConversationJournalDDL = `
CREATE TABLE IF NOT EXISTS conversation_session_streams (
	session_id TEXT PRIMARY KEY,
	watermark BIGINT NOT NULL DEFAULT 0,
	terminal BOOLEAN NOT NULL DEFAULT FALSE,
	updated_at TIMESTAMP NOT NULL
);
CREATE TABLE IF NOT EXISTS conversation_session_events (
	session_id TEXT NOT NULL,
	sequence BIGINT NOT NULL,
	event_id TEXT NOT NULL UNIQUE,
	protocol_version INTEGER NOT NULL DEFAULT 1,
	event_type TEXT NOT NULL,
	task_id TEXT,
	payload TEXT NOT NULL,
	created_at TIMESTAMP NOT NULL,
	PRIMARY KEY (session_id, sequence)
);
CREATE INDEX IF NOT EXISTS idx_conversation_events_created
	ON conversation_session_events(created_at);
CREATE TABLE IF NOT EXISTS conversation_message_versions (
	session_id TEXT NOT NULL,
	message_id TEXT NOT NULL,
	row_sequence BIGINT NOT NULL,
	task_id TEXT,
	author_type TEXT,
	created_at TIMESTAMP NOT NULL,
	tombstone BOOLEAN NOT NULL DEFAULT FALSE,
	payload TEXT NOT NULL,
	PRIMARY KEY (session_id, message_id, row_sequence)
);
CREATE INDEX IF NOT EXISTS idx_conversation_message_snapshot
	ON conversation_message_versions(session_id, row_sequence, created_at, message_id);
CREATE TABLE IF NOT EXISTS conversation_turn_versions (
	session_id TEXT NOT NULL,
	turn_id TEXT NOT NULL,
	row_sequence BIGINT NOT NULL,
	task_id TEXT,
	started_at TIMESTAMP NOT NULL,
	tombstone BOOLEAN NOT NULL DEFAULT FALSE,
	payload TEXT NOT NULL,
	PRIMARY KEY (session_id, turn_id, row_sequence)
);
CREATE INDEX IF NOT EXISTS idx_conversation_turn_snapshot
	ON conversation_turn_versions(session_id, row_sequence, started_at, turn_id);
CREATE TABLE IF NOT EXISTS conversation_journal_meta (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
CREATE OR REPLACE FUNCTION conversation_message_journal() RETURNS TRIGGER AS $$
BEGIN
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE OR REPLACE FUNCTION conversation_turn_journal() RETURNS TRIGGER AS $$
BEGIN
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE OR REPLACE FUNCTION conversation_session_delete_journal() RETURNS TRIGGER AS $$
BEGIN
	RETURN OLD;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS conversation_message_journal_trigger ON task_session_messages;
CREATE TRIGGER conversation_message_journal_trigger AFTER INSERT OR UPDATE OR DELETE ON task_session_messages
FOR EACH ROW EXECUTE FUNCTION conversation_message_journal();
DROP TRIGGER IF EXISTS conversation_turn_journal_trigger ON task_session_turns;
CREATE TRIGGER conversation_turn_journal_trigger AFTER INSERT OR UPDATE OR DELETE ON task_session_turns
FOR EACH ROW EXECUTE FUNCTION conversation_turn_journal();
DROP TRIGGER IF EXISTS conversation_session_delete_trigger ON task_sessions;
CREATE TRIGGER conversation_session_delete_trigger AFTER DELETE ON task_sessions
FOR EACH ROW EXECUTE FUNCTION conversation_session_delete_journal();
`

const postgresUnrelatedCleanupObjectDDL = `
CREATE TABLE conversation_cleanup_unrelated (id TEXT PRIMARY KEY);
CREATE OR REPLACE FUNCTION conversation_cleanup_unrelated() RETURNS TRIGGER AS $$
BEGIN
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER conversation_cleanup_unrelated_trigger AFTER INSERT ON task_sessions
FOR EACH ROW EXECUTE FUNCTION conversation_cleanup_unrelated();
`

func seedPostgresConversationSource(t *testing.T, repo *Repository, taskID, sessionID, turnID string) {
	t.Helper()
	seedPostgresTask(t, repo, taskID)
	now := time.Date(2026, 9, 16, 11, 0, 0, 0, time.UTC)
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_sessions (id, task_id, started_at, updated_at)
		VALUES (?, ?, ?, ?)
	`), sessionID, taskID, now, now); err != nil {
		t.Fatalf("seed session %s: %v", sessionID, err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`), turnID, sessionID, taskID, now, now, now); err != nil {
		t.Fatalf("seed turn %s: %v", turnID, err)
	}
}

func insertPostgresConversationMessage(t *testing.T, repo *Repository, id, sessionID, taskID, turnID, author, content string, createdAt time.Time, promptSeq int) {
	t.Helper()
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_session_messages
			(id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at, updated_at, prompt_seq)
		VALUES (?, ?, ?, ?, ?, '', ?, 0, 'message', '{}', ?, ?, ?)
	`), id, sessionID, taskID, turnID, author, content, createdAt, createdAt, promptSeq); err != nil {
		t.Fatalf("insert message %s: %v", id, err)
	}
}

func readPostgresConversationRevision(t *testing.T, repo *Repository, sessionID string) int64 {
	t.Helper()
	revision, err := repo.ReadConversationRevision(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("read revision for %s: %v", sessionID, err)
	}
	if !revision.Exists {
		t.Fatalf("session %s unexpectedly missing", sessionID)
	}
	return revision.Revision
}
