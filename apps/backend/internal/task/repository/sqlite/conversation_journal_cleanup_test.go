package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestConversationJournalCleanupRollsBackAndCanRetry(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const (
		taskID    = "task-conversation-cleanup"
		sessionID = "session-conversation-cleanup"
		turnID    = "turn-conversation-cleanup"
	)
	seedForMsgTest(t, repo, taskID, sessionID, turnID)
	message := &models.Message{
		ID:            "message-conversation-cleanup",
		TaskSessionID: sessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		AuthorType:    models.MessageAuthorUser,
		Type:          models.MessageTypeMessage,
		Content:       "source payload survives journal cleanup",
		CreatedAt:     time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC),
	}
	if err := repo.CreateMessage(ctx, message); err != nil {
		t.Fatalf("create source message: %v", err)
	}

	for _, statement := range []string{
		`CREATE TABLE conversation_session_events (id TEXT)`,
		`CREATE TABLE conversation_message_versions (id TEXT)`,
		`CREATE TABLE conversation_turn_versions (id TEXT)`,
		`CREATE TABLE conversation_session_streams (id TEXT)`,
		`CREATE TABLE conversation_journal_meta (id TEXT)`,
		`CREATE TRIGGER conversation_message_insert AFTER INSERT ON task_session_messages BEGIN INSERT INTO conversation_message_versions(id) VALUES (NEW.id); END`,
	} {
		if _, err := repo.db.Exec(statement); err != nil {
			t.Fatalf("create legacy fixture with %q: %v", statement, err)
		}
	}

	repo.failConversationJournalCleanupAfter = "statement-1"
	if err := repo.cleanupLegacyConversationJournal(); err == nil {
		t.Fatal("cleanup with failpoint succeeded")
	}
	assertConversationJournalObjectsExist(t, repo)
	got, err := repo.GetMessage(ctx, message.ID)
	if err != nil {
		t.Fatalf("read source message after rollback: %v", err)
	}
	if got.Content != message.Content {
		t.Fatalf("source content after rollback = %q, want %q", got.Content, message.Content)
	}

	repo.failConversationJournalCleanupAfter = ""
	if err := repo.cleanupLegacyConversationJournal(); err != nil {
		t.Fatalf("retry cleanup: %v", err)
	}
	assertConversationJournalObjectsAbsent(t, repo)
	got, err = repo.GetMessage(ctx, message.ID)
	if err != nil {
		t.Fatalf("read source message after cleanup: %v", err)
	}
	if got.Content != message.Content {
		t.Fatalf("source content after cleanup = %q, want %q", got.Content, message.Content)
	}
	if err := repo.cleanupLegacyConversationJournal(); err != nil {
		t.Fatalf("repeated cleanup: %v", err)
	}
}

func assertConversationJournalObjectsExist(t *testing.T, repo *Repository) {
	t.Helper()
	assertConversationJournalObjects(t, repo, true)
}

func assertConversationJournalObjectsAbsent(t *testing.T, repo *Repository) {
	t.Helper()
	assertConversationJournalObjects(t, repo, false)
}

func assertConversationJournalObjects(t *testing.T, repo *Repository, wantPresent bool) {
	t.Helper()
	for _, name := range []string{
		"conversation_session_events",
		"conversation_message_versions",
		"conversation_turn_versions",
		"conversation_session_streams",
		"conversation_journal_meta",
		"conversation_message_insert",
	} {
		var count int
		if err := repo.db.Get(&count, `SELECT COUNT(*) FROM sqlite_master WHERE name = ?`, name); err != nil {
			t.Fatalf("inspect legacy object %q: %v", name, err)
		}
		present := count != 0
		if present != wantPresent {
			t.Fatalf("legacy object %q present = %v, want %v", name, present, wantPresent)
		}
	}
}
