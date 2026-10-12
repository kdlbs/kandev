package sqlite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresGitPushDismissalConcurrentFirstWrite(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	const taskID, sessionID, turnID = "dismiss-task", "dismiss-session", "dismiss-turn"
	const messageID, key = "dismiss-message", "git_operation_error_dismissed_at"
	seedPostgresConversationSource(t, repo, taskID, sessionID, turnID)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	message := &models.Message{
		ID: messageID, TaskID: taskID, TaskSessionID: sessionID, TurnID: turnID,
		AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeError,
		Content:  "Git push failed",
		Metadata: map[string]any{"git_operation_error": true, "operation": "push", "error_output": "diagnostic"},
	}
	if err := repo.CreateMessage(ctx, message); err != nil {
		t.Fatal(err)
	}
	base := readPostgresConversationRevision(t, repo, sessionID)
	secondDB := openSecondPostgresConnection(t, dsn, db)
	secondRepo, err := NewWithDB(secondDB, secondDB, nil)
	if err != nil {
		t.Fatal(err)
	}
	results := concurrentDismissalWrites(ctx, []*Repository{repo, secondRepo}, messageID, sessionID, key)
	writes, winner := 0, ""
	for range 2 {
		select {
		case result := <-results:
			if result.err != nil {
				t.Fatal(result.err)
			}
			timestamp, _ := result.message.Metadata[key].(string)
			if timestamp == "" || (winner != "" && winner != timestamp) {
				t.Fatalf("dismissal timestamp = %q, previous winner = %q", timestamp, winner)
			}
			winner = timestamp
			if result.changed {
				writes++
				assertPostgresConversationReceipt(t, result.receipt, sessionID, base, base+1, true,
					models.ConversationMutationUpsert, models.ConversationEntityMessage, messageID)
			} else if result.receipt != nil {
				t.Fatal("unchanged dismissal returned a mutation receipt")
			}
		case <-ctx.Done():
			t.Fatal("concurrent dismissals did not finish")
		}
	}
	stored, err := repo.GetMessage(ctx, messageID)
	if err != nil {
		t.Fatal(err)
	}
	if writes != 1 || readPostgresConversationRevision(t, repo, sessionID) != base+1 {
		t.Fatalf("winning writes = %d, want one write and one revision increment", writes)
	}
	if stored.Metadata[key] != winner || stored.Content != message.Content || stored.Metadata["error_output"] != "diagnostic" {
		t.Fatalf("persisted dismissal lost the winner or diagnostics: %+v", stored)
	}
}

type dismissalWriteResult struct {
	message *models.Message
	receipt *models.ConversationMutationReceipt
	changed bool
	err     error
}

func concurrentDismissalWrites(ctx context.Context, writers []*Repository, messageID, sessionID, key string) <-chan dismissalWriteResult {
	start := make(chan struct{})
	results := make(chan dismissalWriteResult, len(writers))
	for index, writer := range writers {
		go func() {
			<-start
			message, receipt, changed, err := writer.SetMessageMetadataStringIfEmptyWithConversationReceipt(
				ctx, messageID, sessionID, key, fmt.Sprintf("2026-10-08T12:00:0%dZ", index),
			)
			results <- dismissalWriteResult{message, receipt, changed, err}
		}()
	}
	close(start)
	return results
}
