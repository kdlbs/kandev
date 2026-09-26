package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresMessageAttachmentLifecycle(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-attachments-pg", Name: "Attachments PG"}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-pg", WorkspaceID: "workspace-attachments-pg", Title: "Attachment PG"}); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	t.Run("claim and release", func(t *testing.T) {
		now := time.Now().UTC()
		attachment := &models.TaskMessageAttachment{
			ID: "attachment-release-pg", OwnerID: "owner-pg", WorkspaceID: "workspace-attachments-pg",
			Name: "release.bin", MimeType: "application/octet-stream", Kind: "resource", DeliveryMode: "path",
			SizeBytes: 1, StorageKey: "attachment-release-pg", State: models.AttachmentStateStaged,
			ExpiresAt: now.Add(time.Hour), CreatedAt: now,
		}
		if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
			t.Fatalf("create attachment: %v", err)
		}
		if err := repo.ClaimMessageAttachments(ctx, []string{attachment.ID}, "owner-pg", "workspace-attachments-pg", "task-pg", "session-pg"); err != nil {
			t.Fatalf("claim attachment: %v", err)
		}
		released, err := repo.DeleteClaimedMessageAttachments(ctx, []string{attachment.ID}, "owner-pg", "task-pg", "session-pg")
		if err != nil {
			t.Fatalf("release attachment: %v", err)
		}
		if len(released) != 1 || released[0].ID != attachment.ID {
			t.Fatalf("released = %+v", released)
		}
	})
	t.Run("task/session cleanup without owner", func(t *testing.T) {
		now := time.Now().UTC()
		attachment := &models.TaskMessageAttachment{
			ID: "attachment-cleanup-session-pg", OwnerID: "owner-pg", WorkspaceID: "workspace-attachments-pg",
			TaskID: "task-cleanup-session-pg", SessionID: "session-cleanup-pg", Name: "cleanup-session.bin",
			MimeType: "application/octet-stream", Kind: "resource", DeliveryMode: "path", SizeBytes: 1,
			StorageKey: "attachment-cleanup-session-pg", State: models.AttachmentStateClaimed,
			ExpiresAt: now.Add(time.Hour), CreatedAt: now,
		}
		if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
			t.Fatalf("create attachment: %v", err)
		}
		removed, err := repo.DeleteClaimedMessageAttachmentsByTaskSession(
			ctx, []string{attachment.ID}, attachment.TaskID, attachment.SessionID,
		)
		if err != nil {
			t.Fatalf("cleanup release: %v", err)
		}
		if len(removed) != 1 || removed[0].ID != attachment.ID {
			t.Fatalf("removed = %+v", removed)
		}
	})

	t.Run("owned transfer during active compensation", func(t *testing.T) {
		taskID := "task-transfer-pg"
		oldSessionID := "session-transfer-old-pg"
		newSessionID := "session-transfer-new-pg"
		if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Attachment transfer"}); err != nil {
			t.Fatalf("create task: %v", err)
		}
		for _, sessionID := range []string{oldSessionID, newSessionID} {
			if err := repo.CreateTaskSession(ctx, &models.TaskSession{
				ID: sessionID, TaskID: taskID, State: models.TaskSessionStateWaitingForInput,
			}); err != nil {
				t.Fatalf("create session %s: %v", sessionID, err)
			}
		}
		queueRepo, err := messagequeue.NewSQLiteRepository(db, db)
		if err != nil {
			t.Fatalf("create queue repository: %v", err)
		}
		compensations := queueRepo.(interface {
			UpsertSessionTransferCompensation(context.Context, messagequeue.SessionTransferCompensation) error
		})
		if err := compensations.UpsertSessionTransferCompensation(ctx, messagequeue.SessionTransferCompensation{
			OperationID: "attachment-transfer-pg",
			TaskID:      taskID, FromSessionID: oldSessionID, ToSessionID: newSessionID,
		}); err != nil {
			t.Fatalf("create compensation: %v", err)
		}
		now := time.Now().UTC()
		attachment := &models.TaskMessageAttachment{
			ID: "attachment-transfer-pg", OwnerID: "owner-pg", WorkspaceID: "workspace-attachments-pg",
			TaskID: taskID, SessionID: oldSessionID, Name: "transfer.bin",
			MimeType: "application/octet-stream", Kind: "resource", DeliveryMode: "path", SizeBytes: 1,
			StorageKey: "attachment-transfer-pg", State: models.AttachmentStateClaimed,
			ExpiresAt: now.Add(time.Hour), CreatedAt: now,
		}
		if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
			t.Fatalf("create transfer attachment: %v", err)
		}
		if err := repo.TransferMessageAttachments(
			ctx, taskID, oldSessionID, newSessionID, []string{attachment.ID},
		); err != nil {
			t.Fatalf("transfer attachment: %v", err)
		}
		stored, err := repo.GetMessageAttachment(ctx, attachment.ID)
		if err != nil {
			t.Fatalf("load transferred attachment: %v", err)
		}
		if stored.SessionID != newSessionID {
			t.Fatalf("transferred session = %q, want %q", stored.SessionID, newSessionID)
		}
	})

	t.Run("task cleanup", func(t *testing.T) {
		now := time.Now().UTC()
		attachment := &models.TaskMessageAttachment{
			ID: "attachment-task-cleanup-pg", OwnerID: "owner-pg", WorkspaceID: "workspace-attachments-pg",
			TaskID: "task-cleanup-pg", SessionID: "session-pg", Name: "cleanup.bin",
			MimeType: "application/octet-stream", Kind: "resource", DeliveryMode: "path", SizeBytes: 1,
			StorageKey: "attachment-task-cleanup-pg", State: models.AttachmentStateClaimed,
			ExpiresAt: now.Add(time.Hour), CreatedAt: now,
		}
		if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
			t.Fatalf("create attachment: %v", err)
		}
		removed, err := repo.DeleteMessageAttachmentsByTask(ctx, attachment.TaskID)
		if err != nil {
			t.Fatalf("delete task attachments: %v", err)
		}
		if len(removed) != 1 || removed[0].ID != attachment.ID {
			t.Fatalf("removed = %+v", removed)
		}
	})

	t.Run("expiry", func(t *testing.T) {
		now := time.Now().UTC()
		attachment := &models.TaskMessageAttachment{
			ID: "attachment-expired-pg", OwnerID: "owner-pg", WorkspaceID: "workspace-attachments-pg",
			Name: "expired.bin", MimeType: "application/octet-stream", Kind: "resource", DeliveryMode: "path",
			SizeBytes: 1, StorageKey: "attachment-expired-pg", State: models.AttachmentStateStaged,
			ExpiresAt: now.Add(-time.Minute), CreatedAt: now.Add(-time.Hour),
		}
		if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
			t.Fatalf("create attachment: %v", err)
		}
		expired, err := repo.MarkExpiredMessageAttachments(ctx, now)
		if err != nil {
			t.Fatalf("mark expired attachments: %v", err)
		}
		if len(expired) != 1 || expired[0].ID != attachment.ID || expired[0].State != models.AttachmentStateExpired {
			t.Fatalf("expired = %+v", expired)
		}
	})
}
