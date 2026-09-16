package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestConversationSourceRevisionTracksMessageLifecycle(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const sessionID = "session-source-revision"
	seedForMsgTest(t, repo, "task-source-revision", sessionID, "turn-source-revision")

	// seedForMsgTest creates the turn that owns the message, so the source
	// revision already includes that persisted turn.
	assertConversationRevision(t, repo, sessionID, 1)

	message := &models.Message{
		ID:            "message-source-revision",
		TaskSessionID: sessionID,
		TaskID:        "task-source-revision",
		TurnID:        "turn-source-revision",
		AuthorType:    models.MessageAuthorAgent,
		Type:          models.MessageTypeMessage,
		Content:       "first",
		CreatedAt:     time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC),
	}
	if err := repo.CreateMessage(ctx, message); err != nil {
		t.Fatalf("create message: %v", err)
	}
	assertConversationRevision(t, repo, sessionID, 2)

	message.Content = "updated"
	if err := repo.UpdateMessage(ctx, message); err != nil {
		t.Fatalf("update message: %v", err)
	}
	assertConversationRevision(t, repo, sessionID, 3)

	if err := repo.DeleteMessage(ctx, message.ID); err != nil {
		t.Fatalf("delete message: %v", err)
	}
	assertConversationRevision(t, repo, sessionID, 4)
}

func TestConversationSourceMessagePageReturnsConsistentRevision(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const (
		taskID    = "task-source-page"
		sessionID = "session-source-page"
		turnID    = "turn-source-page"
	)
	seedForMsgTest(t, repo, taskID, sessionID, turnID)
	insertAgentMsg(t, repo, "message-source-page-a", sessionID, turnID, "user", "a", time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC))
	insertAgentMsg(t, repo, "message-source-page-b", sessionID, turnID, "agent", "b", time.Date(2026, 9, 16, 12, 0, 1, 0, time.UTC))

	reader, ok := any(repo).(interface {
		ReadConversationMessagesPage(context.Context, models.ConversationMessagePageRequest) (models.ConversationMessagePage, error)
	})
	if !ok {
		t.Fatal("repository does not implement source-backed message pages")
	}
	page, err := reader.ReadConversationMessagesPage(ctx, models.ConversationMessagePageRequest{
		SessionID: sessionID,
		Sort:      "asc",
		Limit:     1,
	})
	if err != nil {
		t.Fatalf("read message page: %v", err)
	}
	if page.Revision != 3 {
		t.Fatalf("page revision = %d, want 3", page.Revision)
	}
	if len(page.Messages) != 1 || page.Messages[0].ID != "message-source-page-a" {
		t.Fatalf("page messages = %#v", page.Messages)
	}
	if !page.HasMore || page.CursorID != "message-source-page-a" {
		t.Fatalf("page continuation = hasMore %v cursor %q", page.HasMore, page.CursorID)
	}
}

func TestConversationSourceTurnPageUsesKeysetCursor(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const (
		taskID    = "task-source-turn-page"
		sessionID = "session-source-turn-page"
	)
	seedForMsgTest(t, repo, taskID, sessionID, "turn-source-turn-page-a")
	secondStarted := time.Now().UTC().Add(time.Second)
	for _, turn := range []*models.Turn{
		{ID: "turn-source-turn-page-b", TaskSessionID: sessionID, TaskID: taskID, StartedAt: secondStarted, CreatedAt: secondStarted},
	} {
		// The seed turn already owns the first ID. Only insert the second row.
		if turn.ID == "turn-source-turn-page-b" {
			if err := repo.CreateTurn(ctx, turn); err != nil {
				t.Fatalf("create second turn: %v", err)
			}
		}
	}

	page, err := repo.ReadConversationTurnsPage(ctx, models.ConversationTurnPageRequest{
		SessionID: sessionID,
		Sort:      "asc",
		Limit:     1,
	})
	if err != nil {
		t.Fatalf("read first turn page: %v", err)
	}
	if len(page.Turns) != 1 || page.Turns[0].ID != "turn-source-turn-page-a" {
		t.Fatalf("first turn page = %#v (id=%q)", page.Turns, page.Turns[0].ID)
	}
	if !page.HasMore || page.CursorID != "turn-source-turn-page-a" {
		t.Fatalf("first turn continuation = hasMore %v cursor %q", page.HasMore, page.CursorID)
	}

	page, err = repo.ReadConversationTurnsPage(ctx, models.ConversationTurnPageRequest{
		SessionID: sessionID,
		Sort:      "asc",
		CursorID:  page.CursorID,
		Limit:     1,
	})
	if err != nil {
		t.Fatalf("read second turn page: %v", err)
	}
	if len(page.Turns) != 1 || page.Turns[0].ID != "turn-source-turn-page-b" || page.HasMore {
		t.Fatalf("second turn page = %#v, hasMore=%v", page.Turns, page.HasMore)
	}
}

func TestConversationSourceReadsRejectCursorFromAnotherSession(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-source-cursor-a", "session-source-cursor-a", "turn-source-cursor-a")
	seedForMsgTest(t, repo, "task-source-cursor-b", "session-source-cursor-b", "turn-source-cursor-b")
	insertAgentMsg(t, repo, "message-source-cursor-b", "session-source-cursor-b", "turn-source-cursor-b", "agent", "b", time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC))

	_, err := repo.ReadConversationMessagesPage(ctx, models.ConversationMessagePageRequest{
		SessionID: "session-source-cursor-a",
		Sort:      "asc",
		CursorID:  "message-source-cursor-b",
		Limit:     1,
	})
	if err == nil {
		t.Fatal("message cursor from another session was accepted")
	}

	_, err = repo.ReadConversationTurnsPage(ctx, models.ConversationTurnPageRequest{
		SessionID: "session-source-cursor-a",
		Sort:      "asc",
		CursorID:  "turn-source-cursor-b",
		Limit:     1,
	})
	if err == nil {
		t.Fatal("turn cursor from another session was accepted")
	}
}

func TestConversationSourceRevisionMissingRowMeansZero(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const sessionID = "session-source-zero"
	seedForMsgTest(t, repo, "task-source-zero", sessionID, "turn-source-zero")
	if _, err := repo.db.Exec(`DELETE FROM conversation_session_revisions WHERE session_id = ?`, sessionID); err != nil {
		t.Fatalf("delete revision row: %v", err)
	}

	revision, err := repo.ReadConversationRevision(ctx, sessionID)
	if err != nil {
		t.Fatalf("read zero revision: %v", err)
	}
	if !revision.Exists || revision.Revision != 0 {
		t.Fatalf("revision = %+v, want existing revision zero", revision)
	}
}

func TestConversationSourceRevisionRowCascadesWithSession(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const sessionID = "session-source-cascade"
	seedForMsgTest(t, repo, "task-source-cascade", sessionID, "turn-source-cascade")
	if _, err := repo.db.ExecContext(ctx, `DELETE FROM task_sessions WHERE id = ?`, sessionID); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	var count int
	if err := repo.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM conversation_session_revisions WHERE session_id = ?`, sessionID).Scan(&count); err != nil {
		t.Fatalf("count revision rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("revision row count = %d, want 0", count)
	}
}

func TestConversationMutationReceiptCapturesMessageTransaction(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const sessionID = "session-source-receipt-message"
	seedForMsgTest(t, repo, "task-source-receipt-message", sessionID, "turn-source-receipt-message")
	message := &models.Message{
		ID:            "message-source-receipt",
		TaskSessionID: sessionID,
		TaskID:        "task-source-receipt-message",
		TurnID:        "turn-source-receipt-message",
		AuthorType:    models.MessageAuthorAgent,
		Content:       "receipt",
		CreatedAt:     time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC),
	}

	receipt, err := repo.CreateMessageWithConversationReceipt(ctx, message)
	if err != nil {
		t.Fatalf("create message with receipt: %v", err)
	}
	if receipt.BaseRevision != 1 || receipt.Revision != 2 || !receipt.Complete || len(receipt.Operations) != 1 {
		t.Fatalf("create receipt = %+v", receipt)
	}
	operation := receipt.Operations[0]
	if operation.Kind != models.ConversationMutationUpsert || operation.Entity != models.ConversationEntityMessage || operation.ID != message.ID || operation.Message == nil {
		t.Fatalf("create operation = %+v", operation)
	}

	message.Content = "updated receipt"
	receipt, err = repo.UpdateMessageWithConversationReceipt(ctx, message)
	if err != nil {
		t.Fatalf("update message with receipt: %v", err)
	}
	if receipt.BaseRevision != 2 || receipt.Revision != 3 || receipt.Operations[0].Message == nil || receipt.Operations[0].Message.Content != "updated receipt" {
		t.Fatalf("update receipt = %+v", receipt)
	}

	receipt, err = repo.DeleteMessageWithConversationReceipt(ctx, message.ID)
	if err != nil {
		t.Fatalf("delete message with receipt: %v", err)
	}
	if receipt.BaseRevision != 3 || receipt.Revision != 4 || receipt.Operations[0].Kind != models.ConversationMutationRemove || receipt.Operations[0].Message != nil {
		t.Fatalf("delete receipt = %+v", receipt)
	}
}

func TestConversationMutationReceiptCapturesTurnTransaction(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const sessionID = "session-source-receipt-turn"
	seedForMsgTest(t, repo, "task-source-receipt-turn", sessionID, "turn-source-receipt-turn-seed")
	turn := &models.Turn{
		ID:            "turn-source-receipt-turn",
		TaskSessionID: sessionID,
		TaskID:        "task-source-receipt-turn",
		StartedAt:     time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC),
		CreatedAt:     time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC),
	}
	receipt, err := repo.CreateTurnWithConversationReceipt(ctx, turn)
	if err != nil {
		t.Fatalf("create turn with receipt: %v", err)
	}
	if receipt.BaseRevision != 1 || receipt.Revision != 2 || receipt.Operations[0].Turn == nil {
		t.Fatalf("create turn receipt = %+v", receipt)
	}

	receipt, err = repo.CompleteTurnWithConversationReceipt(ctx, turn.ID)
	if err != nil {
		t.Fatalf("complete turn with receipt: %v", err)
	}
	if receipt.BaseRevision != 2 || receipt.Revision != 3 || receipt.Operations[0].Turn == nil || receipt.Operations[0].Turn.CompletedAt == nil {
		t.Fatalf("complete turn receipt = %+v", receipt)
	}
}

func assertConversationRevision(t *testing.T, repo *Repository, sessionID string, want int64) {
	t.Helper()
	var got int64
	if err := repo.db.QueryRow(
		`SELECT revision FROM conversation_session_revisions WHERE session_id = ?`,
		sessionID,
	).Scan(&got); err != nil {
		t.Fatalf("read conversation revision: %v", err)
	}
	if got != want {
		t.Fatalf("conversation revision = %d, want %d", got, want)
	}
}
