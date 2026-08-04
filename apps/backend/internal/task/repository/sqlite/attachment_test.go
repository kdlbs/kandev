package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestClaimMessageAttachments_IsIdempotentForSameTaskSession(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-attachments")
	now := time.Now().UTC()
	attachment := &models.TaskMessageAttachment{
		ID: "attachment-idempotent", OwnerID: "owner-1", WorkspaceID: "workspace-attachments",
		Name: "bundle.zip", MimeType: "application/zip", Kind: "resource", DeliveryMode: "path",
		SizeBytes: 16, StorageKey: "attachment-idempotent", State: models.AttachmentStateStaged,
		ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	}
	if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
		t.Fatal(err)
	}
	if err := repo.ClaimMessageAttachments(ctx, []string{attachment.ID}, "owner-1", "workspace-attachments", "task-1", "session-1"); err != nil {
		t.Fatalf("initial claim: %v", err)
	}
	if err := repo.ClaimMessageAttachments(ctx, []string{attachment.ID}, "owner-1", "workspace-attachments", "task-1", "session-1"); err != nil {
		t.Fatalf("idempotent claim: %v", err)
	}
	got, err := repo.GetMessageAttachment(ctx, attachment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != models.AttachmentStateClaimed || got.TaskID != "task-1" || got.SessionID != "session-1" {
		t.Fatalf("claimed attachment = %+v", got)
	}
}
