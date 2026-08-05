package sqlite

import (
	"context"
	"testing"
	"time"

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
