package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// The durable queue cleanup path carries no user identity: ReleaseForCleanup
// must remove claimed rows and bytes scoped by task/session alone.
func TestReleaseForCleanupRemovesClaimedRowsAndBytes(t *testing.T) {
	svc, repo, root, _ := newAttachmentTestService(t)
	ctx := context.Background()
	released := stageTestAttachment(t, svc, "user-a", "released.png", "x")
	kept := stageTestAttachment(t, svc, "user-a", "kept.png", "y")
	if err := svc.Claim(ctx, "user-a", "ws-att", "task-1", "sess-1", []string{released.ID, kept.ID}); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if err := svc.ReleaseForCleanup(ctx, "task-1", "sess-1", []string{released.ID}); err != nil {
		t.Fatalf("ReleaseForCleanup: %v", err)
	}
	if _, err := repo.GetMessageAttachment(ctx, released.ID); !errors.Is(err, ErrAttachmentNotFound) {
		t.Fatalf("released row = %v, want ErrAttachmentNotFound", err)
	}
	if _, err := os.Stat(filepath.Join(root, "attachments", released.StorageKey)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("released bytes still present: %v", err)
	}
	if _, err := repo.GetMessageAttachment(ctx, kept.ID); err != nil {
		t.Fatalf("unreferenced release must not touch other rows: %v", err)
	}
}

// ResolveMessageAttachmentOwner recovers the claim owner so a legacy durable
// cleanup (persisted before the owner column) can replay under its identity.
func TestResolveMessageAttachmentOwnerRecoversClaimOwner(t *testing.T) {
	attachmentSvc, _, _, _ := newAttachmentTestService(t)
	svc := &Service{attachmentSvc: attachmentSvc}
	ctx := context.Background()
	first := stageTestAttachment(t, attachmentSvc, "user-a", "first.png", "x")
	second := stageTestAttachment(t, attachmentSvc, "user-a", "second.png", "y")
	if err := attachmentSvc.Claim(ctx, "user-a", "ws-att", "task-1", "sess-1", []string{first.ID, second.ID}); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	owner, err := svc.ResolveMessageAttachmentOwner(ctx, "task-1", "sess-1", []v1.MessageAttachment{
		{AttachmentID: first.ID}, {AttachmentID: second.ID},
	})
	if err != nil {
		t.Fatalf("ResolveMessageAttachmentOwner: %v", err)
	}
	if owner != "user-a" {
		t.Fatalf("owner = %q, want user-a", owner)
	}
	if _, err := svc.ResolveMessageAttachmentOwner(ctx, "task-1", "sess-other", []v1.MessageAttachment{
		{AttachmentID: first.ID},
	}); err == nil || !strings.Contains(err.Error(), "ownership is invalid") {
		t.Fatalf("foreign-session resolve = %v, want ownership error", err)
	}
}
func TestResolveMessageAttachmentOwnerRecoversStagedClaimOwner(t *testing.T) {
	attachmentSvc, repo, _, _ := newAttachmentTestService(t)
	svc := &Service{attachmentSvc: attachmentSvc}
	ctx := context.Background()
	attachment := &models.TaskMessageAttachment{
		ID: "staged-owner-recovery", OwnerID: "user-staged", WorkspaceID: "ws-att",
		TaskID: "task-1", SessionID: "sess-1", Name: "staged.png",
		MimeType: "image/png", Kind: "image", DeliveryMode: "path",
		SizeBytes: 1, StorageKey: "staged-owner-recovery",
		State: models.AttachmentStateStaged, ExpiresAt: time.Now().Add(time.Hour),
		CreatedAt: time.Now(),
	}
	if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
		t.Fatal(err)
	}

	owner, err := svc.ResolveMessageAttachmentOwner(ctx, "task-1", "sess-1", []v1.MessageAttachment{
		{AttachmentID: attachment.ID},
	})
	if err != nil {
		t.Fatalf("ResolveMessageAttachmentOwner: %v", err)
	}
	if owner != "user-staged" {
		t.Fatalf("owner = %q, want user-staged", owner)
	}
}
