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

func TestClaimMessageAttachments_AllowsTaskScopedClaimForSameTaskSession(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-attachments")
	now := time.Now().UTC()
	attachment := &models.TaskMessageAttachment{
		ID: "attachment-task-scoped", OwnerID: "owner-1", WorkspaceID: "workspace-attachments",
		Name: "notes.txt", MimeType: "text/plain", Kind: "resource", DeliveryMode: "path",
		SizeBytes: 16, StorageKey: "attachment-task-scoped", State: models.AttachmentStateStaged,
		ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	}
	if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
		t.Fatal(err)
	}
	if err := repo.ClaimMessageAttachments(ctx, []string{attachment.ID}, "owner-1", "workspace-attachments", "task-1", ""); err != nil {
		t.Fatalf("task-scoped claim: %v", err)
	}
	if err := repo.ClaimMessageAttachments(ctx, []string{attachment.ID}, "owner-1", "workspace-attachments", "task-1", "session-1"); err != nil {
		t.Fatalf("session claim after task-scoped claim: %v", err)
	}
	got, err := repo.GetMessageAttachment(ctx, attachment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != models.AttachmentStateClaimed || got.TaskID != "task-1" || got.SessionID != "" {
		t.Fatalf("claimed attachment = %+v", got)
	}
}

func TestClaimMessageAttachments_RejectsDifferentSessionAfterSessionClaim(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-attachments")
	now := time.Now().UTC()
	attachment := &models.TaskMessageAttachment{
		ID: "attachment-session-scoped", OwnerID: "owner-1", WorkspaceID: "workspace-attachments",
		Name: "notes.txt", MimeType: "text/plain", Kind: "resource", DeliveryMode: "path",
		SizeBytes: 16, StorageKey: "attachment-session-scoped", State: models.AttachmentStateStaged,
		ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	}
	if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
		t.Fatal(err)
	}
	if err := repo.ClaimMessageAttachments(ctx, []string{attachment.ID}, "owner-1", "workspace-attachments", "task-1", "session-1"); err != nil {
		t.Fatalf("initial claim: %v", err)
	}
	if err := repo.ClaimMessageAttachments(ctx, []string{attachment.ID}, "owner-1", "workspace-attachments", "task-1", "session-2"); err == nil {
		t.Fatal("expected a different session claim to fail")
	}
}

func TestClaimMessageAttachments_AllowsSeparateSubmissionsToReachTheLimit(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-attachments")
	now := time.Now().UTC()
	first := &models.TaskMessageAttachment{
		ID: "attachment-first", OwnerID: "owner-1", WorkspaceID: "workspace-attachments",
		Name: "first.bin", MimeType: "application/octet-stream", Kind: "resource", DeliveryMode: "path",
		SizeBytes: models.MaxMessageAttachmentBytes * 3 / 5, StorageKey: "attachment-first", State: models.AttachmentStateStaged,
		ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	}
	second := *first
	second.ID = "attachment-second"
	second.Name = "second.bin"
	second.StorageKey = second.ID
	for _, attachment := range []*models.TaskMessageAttachment{first, &second} {
		if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
			t.Fatal(err)
		}
	}

	if err := repo.ClaimMessageAttachments(ctx, []string{first.ID}, "owner-1", "workspace-attachments", "task-1", "session-1"); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if err := repo.ClaimMessageAttachments(ctx, []string{second.ID}, "owner-1", "workspace-attachments", "task-1", "session-1"); err != nil {
		t.Fatalf("second claim: %v", err)
	}
}

func TestClaimMessageAttachments_RejectsExpiredStagedDescriptor(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-attachments")
	now := time.Now().UTC()
	attachment := &models.TaskMessageAttachment{
		ID: "attachment-expired", OwnerID: "owner-1", WorkspaceID: "workspace-attachments",
		Name: "expired.bin", MimeType: "application/octet-stream", Kind: "resource", DeliveryMode: "path",
		SizeBytes: 1, StorageKey: "attachment-expired", State: models.AttachmentStateStaged,
		ExpiresAt: now.Add(-time.Minute), CreatedAt: now.Add(-time.Hour),
	}
	if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
		t.Fatal(err)
	}
	if err := repo.ClaimMessageAttachments(ctx, []string{attachment.ID}, "owner-1", "workspace-attachments", "task-1", "session-1"); err == nil {
		t.Fatal("expected expired attachment claim to fail")
	}
	got, err := repo.GetMessageAttachment(ctx, attachment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != models.AttachmentStateStaged {
		t.Fatalf("expired attachment state = %q, want staged", got.State)
	}
}

func TestDeleteClaimedMessageAttachments_RemovesOnlyMatchingClaims(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-attachments")
	now := time.Now().UTC()
	attachments := []*models.TaskMessageAttachment{
		{ID: "release-one", OwnerID: "owner-1", WorkspaceID: "workspace-attachments", TaskID: "task-1", SessionID: "session-1", Name: "one", MimeType: "text/plain", Kind: "resource", DeliveryMode: "path", SizeBytes: 1, StorageKey: "release-one", State: models.AttachmentStateClaimed, ExpiresAt: now.Add(time.Hour), CreatedAt: now},
		{ID: "release-other-task", OwnerID: "owner-1", WorkspaceID: "workspace-attachments", TaskID: "task-2", SessionID: "session-1", Name: "two", MimeType: "text/plain", Kind: "resource", DeliveryMode: "path", SizeBytes: 1, StorageKey: "release-other-task", State: models.AttachmentStateClaimed, ExpiresAt: now.Add(time.Hour), CreatedAt: now},
	}
	for _, attachment := range attachments {
		if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
			t.Fatal(err)
		}
	}
	released, err := repo.DeleteClaimedMessageAttachments(ctx, []string{"release-one", "release-other-task"}, "owner-1", "task-1", "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(released) != 1 || released[0].ID != "release-one" {
		t.Fatalf("released = %+v", released)
	}
	if _, err := repo.GetMessageAttachment(ctx, "release-one"); err == nil {
		t.Fatal("released attachment still exists")
	}
	if _, err := repo.GetMessageAttachment(ctx, "release-other-task"); err != nil {
		t.Fatalf("unmatched claim was removed: %v", err)
	}
}

func TestDeleteMessageAttachmentsByTask_RemovesRegistryRows(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-attachments")
	now := time.Now().UTC()
	attachment := &models.TaskMessageAttachment{
		ID: "task-cleanup-attachment", OwnerID: "owner-1", WorkspaceID: "workspace-attachments", TaskID: "task-cleanup",
		Name: "cleanup.bin", MimeType: "application/octet-stream", Kind: "resource", DeliveryMode: "path", SizeBytes: 1,
		StorageKey: "task-cleanup-attachment", State: models.AttachmentStateClaimed, ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	}
	if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
		t.Fatal(err)
	}
	removed, err := repo.DeleteMessageAttachmentsByTask(ctx, "task-cleanup")
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0].ID != attachment.ID {
		t.Fatalf("removed = %+v", removed)
	}
	if _, err := repo.GetMessageAttachment(ctx, attachment.ID); err == nil {
		t.Fatal("task attachment still exists")
	}
}
