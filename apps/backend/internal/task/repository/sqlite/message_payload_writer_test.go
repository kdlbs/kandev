package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestGuardedMessageUpdateWritesRowOnce(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		name  string
		apply func(*testing.T, *Repository, *models.Message)
	}{
		{
			name: "ordinary update",
			apply: func(t *testing.T, repo *Repository, message *models.Message) {
				message.Content = "updated ordinary message"
				if err := repo.UpdateMessage(ctx, message); err != nil {
					t.Fatalf("UpdateMessage: %v", err)
				}
			},
		},
		{
			name: "receipt update",
			apply: func(t *testing.T, repo *Repository, message *models.Message) {
				message.Content = "updated receipt message"
				if _, err := repo.UpdateMessageWithConversationReceipt(ctx, message); err != nil {
					t.Fatalf("UpdateMessageWithConversationReceipt: %v", err)
				}
			},
		},
		{
			name: "agent plan update",
			apply: func(t *testing.T, repo *Repository, message *models.Message) {
				message.Content = "updated agent plan"
				if _, _, created, err := repo.UpsertAgentPlanMessageWithConversationReceipt(ctx, message); err != nil {
					t.Fatalf("UpsertAgentPlanMessageWithConversationReceipt: %v", err)
				} else if created {
					t.Fatal("agent plan update unexpectedly created a message")
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			taskID := "task-message-writes"
			sessionID := "session-message-writes"
			turnID := "turn-message-writes"
			seedForMsgTest(t, repo, taskID, sessionID, turnID)
			message := &models.Message{
				ID:            "message-writes",
				TaskID:        taskID,
				TaskSessionID: sessionID,
				TurnID:        turnID,
				AuthorType:    models.MessageAuthorAgent,
				Type:          models.MessageTypeMessage,
				Content:       "initial message",
				Metadata:      map[string]any{},
			}
			if test.name == "agent plan update" {
				message.Type = models.MessageTypeAgentPlan
				message.Metadata = map[string]any{
					"tool_call_id":            "agent-plan:" + message.ID,
					"agent_plan_tool_call_id": "source-" + message.ID,
				}
				if _, _, created, err := repo.UpsertAgentPlanMessageWithConversationReceipt(ctx, message); err != nil {
					t.Fatalf("create initial agent plan: %v", err)
				} else if !created {
					t.Fatal("initial agent plan was not created")
				}
			} else if err := repo.CreateMessage(ctx, message); err != nil {
				t.Fatalf("CreateMessage: %v", err)
			}
			if test.name != "agent plan update" {
				var err error
				message, err = repo.GetMessage(ctx, message.ID)
				if err != nil {
					t.Fatalf("GetMessage: %v", err)
				}
			}
			if _, err := repo.db.ExecContext(ctx, `CREATE TABLE message_row_update_audit (message_id TEXT NOT NULL)`); err != nil {
				t.Fatalf("create message update audit: %v", err)
			}
			if _, err := repo.db.ExecContext(ctx, `
				CREATE TRIGGER audit_message_row_update
				AFTER UPDATE ON task_session_messages
				WHEN OLD.id = 'message-writes'
				BEGIN
					INSERT INTO message_row_update_audit(message_id) VALUES (OLD.id);
				END
			`); err != nil {
				t.Fatalf("install message update audit: %v", err)
			}

			test.apply(t, repo, message)

			var updates int
			if err := repo.db.GetContext(ctx, &updates, `SELECT COUNT(*) FROM message_row_update_audit WHERE message_id = ?`, message.ID); err != nil {
				t.Fatalf("read message update audit: %v", err)
			}
			if updates != 1 {
				t.Fatalf("message-row updates = %d, want 1", updates)
			}
			stored, err := repo.GetMessage(ctx, message.ID)
			if err != nil {
				t.Fatalf("read updated message: %v", err)
			}
			if stored.Content != message.Content {
				t.Fatalf("stored content = %q, want %q", stored.Content, message.Content)
			}
		})
	}
}

func TestGuardedMessageUpdateMissingRow(t *testing.T) {
	repo := newRepoForSessionTests(t)
	err := repo.UpdateMessage(context.Background(), &models.Message{ID: "missing-message"})
	if err == nil || err.Error() != "message not found: missing-message" {
		t.Fatalf("UpdateMessage error = %v, want message not found compatibility error", err)
	}
}

func TestPostgresGuardedMessageUpdateMissingRow(t *testing.T) {
	repo := openPostgresRepo(t)
	ctx := context.Background()
	base := time.Now().UTC()
	seedPostgresSession(t, repo, "task-missing-message-pg", "session-missing-message-pg", "turn-missing-message-pg", base)
	_, err := repo.UpdateMessageWithConversationReceipt(ctx, &models.Message{
		ID:            "missing-message-pg",
		TaskID:        "task-missing-message-pg",
		TaskSessionID: "session-missing-message-pg",
		AuthorType:    models.MessageAuthorAgent,
		Type:          models.MessageTypeMessage,
		Metadata:      map[string]any{},
	})
	if err == nil || err.Error() != "message not found: missing-message-pg" {
		t.Fatalf("UpdateMessageWithConversationReceipt error = %v, want message not found compatibility error", err)
	}
}

func TestGuardedMessageUpdateRollback(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForSessionTests(t)
	seedForMsgTest(t, repo, "task-message-rollback", "session-message-rollback", "turn-message-rollback")
	message := &models.Message{
		ID:            "message-rollback",
		TaskID:        "task-message-rollback",
		TaskSessionID: "session-message-rollback",
		TurnID:        "turn-message-rollback",
		AuthorType:    models.MessageAuthorAgent,
		Type:          models.MessageTypeMessage,
		Content:       "before",
		Metadata:      map[string]any{},
	}
	if err := repo.CreateMessage(ctx, message); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	beforeRevision, err := repo.ReadConversationRevision(ctx, message.TaskSessionID)
	if err != nil {
		t.Fatalf("read initial conversation revision: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `
		CREATE TRIGGER reject_message_update
		BEFORE UPDATE ON task_session_messages
		WHEN NEW.content = 'rejected'
		BEGIN
			SELECT RAISE(ABORT, 'forced message update failure');
		END
	`); err != nil {
		t.Fatalf("install rejection trigger: %v", err)
	}
	message.Content = "rejected"
	if err := repo.UpdateMessage(ctx, message); err == nil {
		t.Fatal("UpdateMessage succeeded despite the rejection trigger")
	}
	stored, err := repo.GetMessage(ctx, message.ID)
	if err != nil {
		t.Fatalf("read message after rollback: %v", err)
	}
	if stored.Content != "before" {
		t.Fatalf("stored content after rollback = %q, want before", stored.Content)
	}
	afterRevision, err := repo.ReadConversationRevision(ctx, message.TaskSessionID)
	if err != nil {
		t.Fatalf("read conversation revision after rollback: %v", err)
	}
	if afterRevision.Revision != beforeRevision.Revision {
		t.Fatalf("conversation revision after rollback = %d, want %d", afterRevision.Revision, beforeRevision.Revision)
	}
}

func TestReceiptUpdatePreservesPayloadRemovalAfterStaleRead(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForSessionTests(t)
	seedForMsgTest(t, repo, "task-receipt-retention", "session-receipt-retention", "turn-receipt-retention")
	largeOutput := strings.Repeat("x", largeMessagePayloadThresholdBytes+1)
	message := newShellMessage("message-receipt-retention", "session-receipt-retention", largeOutput, "")
	message.TaskID = "task-receipt-retention"
	message.TurnID = "turn-receipt-retention"
	message.Type = models.MessageTypeToolExecute
	if err := repo.CreateMessage(ctx, message); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	stale, err := repo.GetMessage(ctx, message.ID)
	if err != nil {
		t.Fatalf("read stale message: %v", err)
	}
	storedUpdatedAt := stale.UpdatedAt
	storedPayloadDigest := stale.PayloadDigest
	storedPayloadSize := stale.PayloadSize
	if storedPayloadDigest == "" || storedPayloadSize <= int64(largeMessagePayloadThresholdBytes) {
		t.Fatalf("seed did not externalize payload: digest=%q size=%d", storedPayloadDigest, storedPayloadSize)
	}
	_, err = repo.db.ExecContext(ctx, `UPDATE task_session_messages SET metadata = ? WHERE id = ?`,
		`{"tool_call_id":"call-1","normalized":{"kind":"shell_exec","shell_exec":{"command":"echo","output":{"exit_code":1}}},"payload_retention":{"version":1,"removed_at":"2026-10-01T00:00:00Z"}}`,
		message.ID,
	)
	if err != nil {
		t.Fatalf("mark payload removed: %v", err)
	}
	stale.Content = "updated through receipt"
	stale.Metadata["result"] = largeOutput
	stale.Metadata["status"] = "complete"
	stale.UpdatedAt = storedUpdatedAt.Add(24 * time.Hour)
	receipt, err := repo.UpdateMessageWithConversationReceipt(ctx, stale)
	if err != nil {
		t.Fatalf("UpdateMessageWithConversationReceipt: %v", err)
	}
	if receipt == nil || !receipt.Complete || receipt.Revision != receipt.BaseRevision+1 {
		t.Fatalf("receipt = %+v, want one complete revision", receipt)
	}
	stored, err := repo.GetMessage(ctx, message.ID)
	if err != nil {
		t.Fatalf("read receipt-updated message: %v", err)
	}
	if stored.Content != "updated through receipt" {
		t.Fatalf("stored content = %q, want receipt update", stored.Content)
	}
	if !models.ToolPayloadRemoved(stored.Metadata) || stored.Type != models.MessageTypeToolExecute {
		t.Fatalf("retained payload state changed: type=%q metadata=%+v", stored.Type, stored.Metadata)
	}
	if stored.UpdatedAt != storedUpdatedAt || stale.UpdatedAt != storedUpdatedAt {
		t.Fatalf("updated_at stored=%s caller=%s, want %s", stored.UpdatedAt, stale.UpdatedAt, storedUpdatedAt)
	}
	if stored.PayloadDigest != storedPayloadDigest || stored.PayloadSize != storedPayloadSize {
		t.Fatalf("payload identity changed: digest=%q size=%d, want %q/%d", stored.PayloadDigest, stored.PayloadSize, storedPayloadDigest, storedPayloadSize)
	}
	if _, ok := stored.Metadata["result"]; ok {
		t.Fatalf("removed result was restored: %+v", stored.Metadata)
	}
	if stored.Metadata["status"] != "complete" {
		t.Fatalf("status = %v, want complete", stored.Metadata["status"])
	}
	if len(receipt.Operations) != 1 || receipt.Operations[0].Message == nil || !models.ToolPayloadRemoved(receipt.Operations[0].Message.Metadata) {
		t.Fatalf("receipt operation did not retain removal state: %+v", receipt.Operations)
	}
}
