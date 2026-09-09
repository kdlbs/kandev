package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestMessageAttachmentDefaultsListExpireAndOwnerScopedDelete(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-a")
	now := time.Date(2026, time.March, 4, 5, 6, 7, 0, time.UTC)
	attachments := []*models.TaskMessageAttachment{
		{ID: "attachment-expired", OwnerID: "owner-a", WorkspaceID: "workspace-a", Name: "old.txt", SizeBytes: 10, ExpiresAt: now.Add(-time.Minute)},
		{ID: "attachment-future", OwnerID: "owner-a", WorkspaceID: "workspace-a", Name: "future.txt", SizeBytes: 20, ExpiresAt: now.Add(time.Minute)},
	}
	for _, attachment := range attachments {
		if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
			t.Fatalf("CreateMessageAttachment(%s): %v", attachment.ID, err)
		}
		if attachment.StorageKey != attachment.ID || attachment.State != models.AttachmentStateStaged || attachment.CreatedAt.IsZero() || attachment.UpdatedAt.IsZero() {
			t.Fatalf("attachment defaults not applied: %+v", attachment)
		}
	}
	listed, err := repo.ListMessageAttachments(ctx, []string{"attachment-future", "attachment-expired", "missing"})
	if err != nil || len(listed) != 2 {
		t.Fatalf("ListMessageAttachments = %+v, %v", listed, err)
	}
	empty, err := repo.ListMessageAttachments(ctx, nil)
	if err != nil || empty != nil {
		t.Fatalf("ListMessageAttachments(nil) = %+v, %v", empty, err)
	}
	expired, err := repo.MarkExpiredMessageAttachments(ctx, now)
	if err != nil || len(expired) != 1 || expired[0].ID != "attachment-expired" || expired[0].State != models.AttachmentStateExpired {
		t.Fatalf("MarkExpiredMessageAttachments = %+v, %v", expired, err)
	}
	got, err := repo.GetMessageAttachment(ctx, "attachment-expired")
	if err != nil || got.State != models.AttachmentStateExpired {
		t.Fatalf("GetMessageAttachment(expired) = %+v, %v", got, err)
	}
	if err := repo.DeleteMessageAttachment(ctx, "attachment-future", "wrong-owner"); !errors.Is(err, models.ErrAttachmentNotFound) {
		t.Fatalf("wrong-owner delete error = %v", err)
	}
	if err := repo.DeleteMessageAttachment(ctx, "attachment-future", "owner-a"); err != nil {
		t.Fatalf("DeleteMessageAttachment: %v", err)
	}
	if _, err := repo.GetMessageAttachment(ctx, "attachment-future"); !errors.Is(err, models.ErrAttachmentNotFound) {
		t.Fatalf("GetMessageAttachment after delete error = %v", err)
	}
}

func TestClaimedAttachmentCleanupReturnsOnlyMatchingRows(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace")
	expires := time.Now().UTC().Add(time.Hour)
	for _, attachment := range []*models.TaskMessageAttachment{
		{ID: "attachment-matching", OwnerID: "owner", WorkspaceID: "workspace", Name: "matching", SizeBytes: 1, ExpiresAt: expires},
		{ID: "attachment-other", OwnerID: "owner", WorkspaceID: "workspace", Name: "other", SizeBytes: 1, ExpiresAt: expires},
	} {
		if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.ClaimMessageAttachments(ctx, []string{"attachment-matching"}, "owner", "workspace", "task", "session"); err != nil {
		t.Fatalf("ClaimMessageAttachments: %v", err)
	}
	if err := repo.ClaimMessageAttachments(ctx, []string{"attachment-matching"}, "owner", "workspace", "task", "session"); err != nil {
		t.Fatalf("idempotent ClaimMessageAttachments: %v", err)
	}
	if err := repo.ClaimMessageAttachments(ctx, []string{"attachment-matching"}, "owner", "workspace", "other-task", "session"); !errors.Is(err, models.ErrAttachmentClaimConflict) {
		t.Fatalf("conflicting claim error = %v", err)
	}
	released, err := repo.DeleteClaimedMessageAttachments(ctx, []string{"attachment-matching", "attachment-other"}, "owner", "task", "session")
	if err != nil || len(released) != 1 || released[0].ID != "attachment-matching" {
		t.Fatalf("DeleteClaimedMessageAttachments = %+v, %v", released, err)
	}
	if _, err := repo.GetMessageAttachment(ctx, "attachment-matching"); !errors.Is(err, models.ErrAttachmentNotFound) {
		t.Fatalf("released attachment still exists: %v", err)
	}
	if _, err := repo.GetMessageAttachment(ctx, "attachment-other"); err != nil {
		t.Fatalf("nonmatching attachment removed: %v", err)
	}
	if released, err := repo.DeleteClaimedMessageAttachments(ctx, nil, "owner", "task", "session"); err != nil || released != nil {
		t.Fatalf("empty claimed cleanup = %+v, %v", released, err)
	}
	if removed, err := repo.DeleteMessageAttachmentsByTask(ctx, ""); err != nil || removed != nil {
		t.Fatalf("empty task cleanup = %+v, %v", removed, err)
	}
}
