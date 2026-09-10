package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

func seedAttachmentTask(t *testing.T, repo *Repository, taskID, workspaceID string) {
	t.Helper()
	if err := repo.CreateTask(context.Background(), &models.Task{
		ID: taskID, WorkspaceID: workspaceID, Title: taskID,
	}); err != nil {
		t.Fatalf("CreateTask(%s): %v", taskID, err)
	}
}

func TestReleaseUnreferencedTaskAttachmentClaimsTxStagesQueueOnlyClaims(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-attachments")
	attachment := &models.TaskMessageAttachment{
		ID: "attachment-archive-queue-only", OwnerID: "owner-1", WorkspaceID: "workspace-attachments",
		Name: "notes.txt", MimeType: "text/plain", Kind: "resource", DeliveryMode: "path",
		SizeBytes: 16, StorageKey: "attachment-archive-queue-only", State: models.AttachmentStateClaimed,
		TaskID: "task-archive", SessionID: "session-archive", CreatedAt: time.Now().UTC(),
	}
	if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
		t.Fatal(err)
	}
	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.releaseUnreferencedTaskAttachmentClaimsTx(ctx, tx, attachment.TaskID, map[string]struct{}{attachment.ID: {}}); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetMessageAttachment(ctx, attachment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != models.AttachmentStateStaged || got.TaskID != attachment.TaskID || got.SessionID != attachment.SessionID {
		t.Fatalf("released archive claim = %+v", got)
	}
}

func TestReleaseUnreferencedTaskAttachmentClaimsTxPreservesDirectClaims(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-attachments")
	attachment := &models.TaskMessageAttachment{
		ID: "attachment-archive-direct", OwnerID: "owner-1", WorkspaceID: "workspace-attachments",
		Name: "notes.txt", MimeType: "text/plain", Kind: "resource", DeliveryMode: "path",
		SizeBytes: 16, StorageKey: "attachment-archive-direct", State: models.AttachmentStateClaimed,
		TaskID: "task-archive", SessionID: "session-archive", CreatedAt: time.Now().UTC(),
	}
	if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
		t.Fatal(err)
	}
	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.releaseUnreferencedTaskAttachmentClaimsTx(ctx, tx, attachment.TaskID, nil); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetMessageAttachment(ctx, attachment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != models.AttachmentStateClaimed {
		t.Fatalf("direct claim state = %q, want claimed", got.State)
	}
}

func TestDeleteMessageAttachmentsByWorkspaceTxRemovesRegistryRowsBeforeCascade(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-attachments")
	attachment := &models.TaskMessageAttachment{
		ID: "attachment-workspace-delete", OwnerID: "owner-1", WorkspaceID: "workspace-attachments",
		Name: "notes.txt", MimeType: "text/plain", Kind: "resource", DeliveryMode: "path",
		SizeBytes: 16, StorageKey: "attachment-workspace-delete", State: models.AttachmentStateClaimed,
		TaskID: "task-1", SessionID: "session-1", CreatedAt: time.Now().UTC(),
	}
	if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
		t.Fatal(err)
	}
	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	removed, err := repo.DeleteMessageAttachmentsByWorkspaceTx(ctx, tx, "workspace-attachments")
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0].ID != attachment.ID {
		_ = tx.Rollback()
		t.Fatalf("removed attachments = %+v", removed)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetMessageAttachment(ctx, attachment.ID); err == nil {
		t.Fatal("workspace attachment registry row still exists")
	}
}

func TestClaimMessageAttachments_IsIdempotentForSameTaskSession(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-attachments")
	seedAttachmentTask(t, repo, "task-1", "workspace-attachments")
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
	seedAttachmentTask(t, repo, "task-1", "workspace-attachments")
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
	seedAttachmentTask(t, repo, "task-1", "workspace-attachments")
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
	seedAttachmentTask(t, repo, "task-1", "workspace-attachments")
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
func TestDeleteTaskSessionWithAttachmentsReturnsDeletedClaimDescriptors(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-session-attachments")
	seedAttachmentTask(t, repo, "task-session-attachments", "workspace-session-attachments")
	session := &models.TaskSession{ID: "session-attachments", TaskID: "task-session-attachments"}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	attachment := &models.TaskMessageAttachment{
		ID: "attachment-session-delete", OwnerID: "owner", WorkspaceID: "workspace-session-attachments",
		Name: "session.txt", MimeType: "text/plain", Kind: "resource", DeliveryMode: "path",
		SizeBytes: 4, StorageKey: "attachment-session-delete", State: models.AttachmentStateClaimed,
		TaskID: session.TaskID, SessionID: session.ID, CreatedAt: time.Now().UTC(),
	}
	if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
		t.Fatalf("CreateMessageAttachment: %v", err)
	}
	deleted, err := repo.DeleteTaskSessionWithAttachments(ctx, session)
	if err != nil {
		t.Fatalf("DeleteTaskSessionWithAttachments: %v", err)
	}
	if len(deleted) != 1 || deleted[0].ID != attachment.ID {
		t.Fatalf("deleted attachments = %+v, want %s", deleted, attachment.ID)
	}
	if _, err := repo.GetMessageAttachment(ctx, attachment.ID); !errors.Is(err, models.ErrAttachmentNotFound) {
		t.Fatalf("attachment registry row still exists: %v", err)
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

func TestDeleteClaimedMessageAttachmentsByTaskSession_ReleasesWithoutOwner(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-attachments")
	now := time.Now().UTC()
	attachments := []*models.TaskMessageAttachment{
		{ID: "cleanup-release", OwnerID: "owner-1", WorkspaceID: "workspace-attachments", TaskID: "task-1", SessionID: "session-1", Name: "one", MimeType: "text/plain", Kind: "resource", DeliveryMode: "path", SizeBytes: 1, StorageKey: "cleanup-release", State: models.AttachmentStateClaimed, ExpiresAt: now.Add(time.Hour), CreatedAt: now},
		{ID: "cleanup-release-other", OwnerID: "owner-1", WorkspaceID: "workspace-attachments", TaskID: "task-2", SessionID: "session-1", Name: "two", MimeType: "text/plain", Kind: "resource", DeliveryMode: "path", SizeBytes: 1, StorageKey: "cleanup-release-other", State: models.AttachmentStateClaimed, ExpiresAt: now.Add(time.Hour), CreatedAt: now},
	}
	for _, attachment := range attachments {
		if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
			t.Fatal(err)
		}
	}
	// The durable queue cleanup path carries no user identity, so the
	// task/session-scoped release must succeed without an owner check.
	released, err := repo.DeleteClaimedMessageAttachmentsByTaskSession(ctx, []string{"cleanup-release", "cleanup-release-other"}, "task-1", "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(released) != 1 || released[0].ID != "cleanup-release" {
		t.Fatalf("released = %+v", released)
	}
	if _, err := repo.GetMessageAttachment(ctx, "cleanup-release"); err == nil {
		t.Fatal("released attachment still exists")
	}
	if _, err := repo.GetMessageAttachment(ctx, "cleanup-release-other"); err != nil {
		t.Fatalf("unmatched claim was removed: %v", err)
	}
	// Replaying the same cleanup is a no-op, not an error.
	released, err = repo.DeleteClaimedMessageAttachmentsByTaskSession(ctx, []string{"cleanup-release"}, "task-1", "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(released) != 0 {
		t.Fatalf("replayed release = %+v, want empty", released)
	}
}
func TestDeleteClaimedMessageAttachmentsByTaskSessionReleasesTaskScopedRows(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-attachments")
	now := time.Now().UTC()
	attachment := &models.TaskMessageAttachment{
		ID: "cleanup-task-scoped", OwnerID: "owner-1", WorkspaceID: "workspace-attachments",
		TaskID: "task-1", Name: "task-scoped", MimeType: "text/plain", Kind: "resource",
		DeliveryMode: "path", SizeBytes: 1, StorageKey: "cleanup-task-scoped",
		State: models.AttachmentStateClaimed, ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	}
	if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
		t.Fatal(err)
	}

	released, err := repo.DeleteClaimedMessageAttachmentsByTaskSession(
		ctx, []string{attachment.ID}, "task-1", "session-1",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(released) != 1 || released[0].ID != attachment.ID {
		t.Fatalf("released = %+v", released)
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

func TestDeleteMessageAttachmentsBySession_RemovesOnlySessionClaims(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-attachments")
	now := time.Now().UTC()
	attachments := []*models.TaskMessageAttachment{
		{ID: "session-cleanup", OwnerID: "owner-1", WorkspaceID: "workspace-attachments", TaskID: "task-session", SessionID: "session-delete", Name: "one", MimeType: "text/plain", Kind: "resource", DeliveryMode: "path", SizeBytes: 1, StorageKey: "session-cleanup", State: models.AttachmentStateClaimed, ExpiresAt: now.Add(time.Hour), CreatedAt: now},
		{ID: "session-keep", OwnerID: "owner-1", WorkspaceID: "workspace-attachments", TaskID: "task-session", SessionID: "session-keep", Name: "two", MimeType: "text/plain", Kind: "resource", DeliveryMode: "path", SizeBytes: 1, StorageKey: "session-keep", State: models.AttachmentStateClaimed, ExpiresAt: now.Add(time.Hour), CreatedAt: now},
	}
	for _, attachment := range attachments {
		if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
			t.Fatal(err)
		}
	}

	removed, err := repo.DeleteMessageAttachmentsBySession(ctx, "task-session", "session-delete")
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0].ID != "session-cleanup" {
		t.Fatalf("removed = %+v", removed)
	}
	if _, err := repo.GetMessageAttachment(ctx, "session-cleanup"); err == nil {
		t.Fatal("deleted session attachment still exists")
	}
	if _, err := repo.GetMessageAttachment(ctx, "session-keep"); err != nil {
		t.Fatalf("other session attachment was removed: %v", err)
	}
}

func TestTransferMessageAttachments_RebindsClaimedRows(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-attachments")
	now := time.Now().UTC()
	attachments := []*models.TaskMessageAttachment{
		{ID: "transfer-one", OwnerID: "owner-1", WorkspaceID: "workspace-attachments", TaskID: "task-transfer", SessionID: "session-old", Name: "one", MimeType: "text/plain", Kind: "resource", DeliveryMode: "path", SizeBytes: 1, StorageKey: "transfer-one", State: models.AttachmentStateClaimed, ExpiresAt: now.Add(time.Hour), CreatedAt: now},
		{ID: "transfer-other-task", OwnerID: "owner-1", WorkspaceID: "workspace-attachments", TaskID: "other-task", SessionID: "session-old", Name: "two", MimeType: "text/plain", Kind: "resource", DeliveryMode: "path", SizeBytes: 1, StorageKey: "transfer-other-task", State: models.AttachmentStateClaimed, ExpiresAt: now.Add(time.Hour), CreatedAt: now},
		{ID: "transfer-unselected", OwnerID: "owner-1", WorkspaceID: "workspace-attachments", TaskID: "task-transfer", SessionID: "session-old", Name: "unselected", MimeType: "text/plain", Kind: "resource", DeliveryMode: "path", SizeBytes: 1, StorageKey: "transfer-unselected", State: models.AttachmentStateClaimed, ExpiresAt: now.Add(time.Hour), CreatedAt: now},
	}
	for _, attachment := range attachments {
		if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
			t.Fatal(err)
		}
	}

	if err := repo.TransferMessageAttachments(
		ctx,
		"task-transfer",
		"session-old",
		"session-new",
		[]string{"transfer-one"},
	); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetMessageAttachment(ctx, "transfer-one")
	if err != nil {
		t.Fatal(err)
	}
	if got.SessionID != "session-new" {
		t.Fatalf("transferred attachment session = %q", got.SessionID)
	}
	other, err := repo.GetMessageAttachment(ctx, "transfer-other-task")
	if err != nil {
		t.Fatal(err)
	}
	if other.SessionID != "session-old" {
		t.Fatalf("unrelated attachment session = %q", other.SessionID)
	}
	unselected, err := repo.GetMessageAttachment(ctx, "transfer-unselected")
	if err != nil {
		t.Fatal(err)
	}
	if unselected.SessionID != "session-old" {
		t.Fatalf("unselected attachment session = %q", unselected.SessionID)
	}
}
func TestTransferMessageAttachmentsAcceptsAlreadyTransferredRows(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-attachments")
	now := time.Now().UTC()
	for _, attachment := range []*models.TaskMessageAttachment{
		{ID: "transfer-source", OwnerID: "owner-1", WorkspaceID: "workspace-attachments", TaskID: "task-transfer", SessionID: "session-old", Name: "source", MimeType: "text/plain", Kind: "resource", DeliveryMode: "path", SizeBytes: 1, StorageKey: "transfer-source", State: models.AttachmentStateClaimed, ExpiresAt: now.Add(time.Hour), CreatedAt: now},
		{ID: "transfer-destination", OwnerID: "owner-1", WorkspaceID: "workspace-attachments", TaskID: "task-transfer", SessionID: "session-new", Name: "destination", MimeType: "text/plain", Kind: "resource", DeliveryMode: "path", SizeBytes: 1, StorageKey: "transfer-destination", State: models.AttachmentStateClaimed, ExpiresAt: now.Add(time.Hour), CreatedAt: now},
	} {
		if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
			t.Fatal(err)
		}
	}

	if err := repo.TransferMessageAttachments(
		ctx, "task-transfer", "session-old", "session-new",
		[]string{"transfer-source", "transfer-destination"},
	); err != nil {
		t.Fatalf("idempotent attachment transfer: %v", err)
	}
	for _, attachmentID := range []string{"transfer-source", "transfer-destination"} {
		attachment, err := repo.GetMessageAttachment(ctx, attachmentID)
		if err != nil {
			t.Fatal(err)
		}
		if attachment.SessionID != "session-new" {
			t.Fatalf("attachment %s session = %q, want session-new", attachmentID, attachment.SessionID)
		}
	}
}

func TestTransferMessageAttachmentsIgnoresTaskScopedRows(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-attachments")
	now := time.Now().UTC()
	attachment := &models.TaskMessageAttachment{
		ID: "transfer-task-scoped", OwnerID: "owner-1", WorkspaceID: "workspace-attachments",
		Name: "task-scoped", MimeType: "text/plain", Kind: "resource", DeliveryMode: "path",
		SizeBytes: 1, StorageKey: "transfer-task-scoped", TaskID: "task-transfer",
		State: models.AttachmentStateClaimed, ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	}
	if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
		t.Fatal(err)
	}

	if err := repo.TransferMessageAttachments(
		ctx, "task-transfer", "session-old", "session-new",
		[]string{attachment.ID},
	); err != nil {
		t.Fatalf("task-scoped attachment transfer: %v", err)
	}
	got, err := repo.GetMessageAttachment(ctx, attachment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.SessionID != "" {
		t.Fatalf("task-scoped attachment session = %q, want empty", got.SessionID)
	}
}

func TestTransferMessageAttachmentsRejectsPartialOwnership(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-attachments")
	now := time.Now().UTC()
	for _, attachment := range []*models.TaskMessageAttachment{
		{ID: "transfer-owned", OwnerID: "owner-1", WorkspaceID: "workspace-attachments", TaskID: "task-transfer", SessionID: "session-old", Name: "owned", MimeType: "text/plain", Kind: "resource", DeliveryMode: "path", SizeBytes: 1, StorageKey: "transfer-owned", State: models.AttachmentStateClaimed, ExpiresAt: now.Add(time.Hour), CreatedAt: now},
		{ID: "transfer-other-session", OwnerID: "owner-1", WorkspaceID: "workspace-attachments", TaskID: "task-transfer", SessionID: "session-other", Name: "other", MimeType: "text/plain", Kind: "resource", DeliveryMode: "path", SizeBytes: 1, StorageKey: "transfer-other-session", State: models.AttachmentStateClaimed, ExpiresAt: now.Add(time.Hour), CreatedAt: now},
	} {
		if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
			t.Fatal(err)
		}
	}

	err := repo.TransferMessageAttachments(
		ctx, "task-transfer", "session-old", "session-new",
		[]string{"transfer-owned", "transfer-other-session"},
	)

	if err == nil {
		t.Fatal("partial attachment transfer unexpectedly succeeded")
	}
	owned, getErr := repo.GetMessageAttachment(ctx, "transfer-owned")
	if getErr != nil {
		t.Fatal(getErr)
	}
	if owned.SessionID != "session-old" {
		t.Fatalf("partially moved attachment session = %q, want session-old", owned.SessionID)
	}
}
func TestTransferMessageAttachmentsAllowsOwnedActiveSessionTransfer(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-attachments")
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-transfer", Title: "Attachment transfer"}); err != nil {
		t.Fatal(err)
	}
	for _, sessionID := range []string{"session-old", "session-new"} {
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: sessionID, TaskID: "task-transfer", State: models.TaskSessionStateWaitingForInput,
		}); err != nil {
			t.Fatal(err)
		}
	}
	queueRepo, err := messagequeue.NewSQLiteRepository(repo.db, repo.db)
	if err != nil {
		t.Fatal(err)
	}
	compensations := queueRepo.(interface {
		UpsertSessionTransferCompensation(context.Context, messagequeue.SessionTransferCompensation) error
	})
	if err := compensations.UpsertSessionTransferCompensation(ctx, messagequeue.SessionTransferCompensation{
		OperationID: "attachment-transfer",
		TaskID:      "task-transfer", FromSessionID: "session-old", ToSessionID: "session-new",
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	attachment := &models.TaskMessageAttachment{
		ID: "transfer-owned-active", OwnerID: "owner-1", WorkspaceID: "workspace-attachments",
		TaskID: "task-transfer", SessionID: "session-old", Name: "owned", MimeType: "text/plain",
		Kind: "resource", DeliveryMode: "path", SizeBytes: 1, StorageKey: "transfer-owned-active",
		State: models.AttachmentStateClaimed, ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	}
	if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
		t.Fatal(err)
	}

	if err := repo.TransferMessageAttachments(
		ctx, "task-transfer", "session-old", "session-new", []string{attachment.ID},
	); err != nil {
		t.Fatalf("owned attachment transfer: %v", err)
	}
	got, err := repo.GetMessageAttachment(ctx, attachment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.SessionID != "session-new" {
		t.Fatalf("transferred attachment session = %q, want session-new", got.SessionID)
	}
}
func TestUpsertSessionTransferCompensationValidatesTaskSessions(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*Repository, context.Context) (string, string, string)
	}{
		{
			name: "missing task",
			setup: func(_ *Repository, _ context.Context) (string, string, string) {
				return "missing-task", "missing-old", "missing-new"
			},
		},
		{
			name: "missing session",
			setup: func(repo *Repository, ctx context.Context) (string, string, string) {
				if err := repo.CreateTask(ctx, &models.Task{ID: "task-missing-session", Title: "Transfer"}); err != nil {
					t.Fatal(err)
				}
				if err := repo.CreateTaskSession(ctx, &models.TaskSession{
					ID: "session-existing", TaskID: "task-missing-session",
					State: models.TaskSessionStateWaitingForInput,
				}); err != nil {
					t.Fatal(err)
				}
				return "task-missing-session", "session-existing", "session-missing"
			},
		},
		{
			name: "session belongs to another task",
			setup: func(repo *Repository, ctx context.Context) (string, string, string) {
				for _, task := range []*models.Task{
					{ID: "task-owner", Title: "Owner"},
					{ID: "task-other", Title: "Other"},
				} {
					if err := repo.CreateTask(ctx, task); err != nil {
						t.Fatal(err)
					}
				}
				if err := repo.CreateTaskSession(ctx, &models.TaskSession{
					ID: "session-owner", TaskID: "task-owner",
					State: models.TaskSessionStateWaitingForInput,
				}); err != nil {
					t.Fatal(err)
				}
				if err := repo.CreateTaskSession(ctx, &models.TaskSession{
					ID: "session-other", TaskID: "task-other",
					State: models.TaskSessionStateWaitingForInput,
				}); err != nil {
					t.Fatal(err)
				}
				return "task-owner", "session-owner", "session-other"
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			ctx := context.Background()
			taskID, fromSessionID, toSessionID := test.setup(repo, ctx)
			queueRepo, err := messagequeue.NewSQLiteRepository(repo.db, repo.db)
			if err != nil {
				t.Fatal(err)
			}
			persistence := queueRepo.(interface {
				UpsertSessionTransferCompensation(context.Context, messagequeue.SessionTransferCompensation) error
			})
			err = persistence.UpsertSessionTransferCompensation(ctx, messagequeue.SessionTransferCompensation{
				OperationID: "invalid-transfer-" + test.name,
				TaskID:      taskID, FromSessionID: fromSessionID, ToSessionID: toSessionID,
			})
			if err == nil {
				t.Fatal("invalid session transfer compensation unexpectedly succeeded")
			}
		})
	}
}

func TestClaimMessageAttachmentsRejectsActiveSessionTransfer(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-attachments")
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-transfer", Title: "Attachment transfer"}); err != nil {
		t.Fatal(err)
	}
	for _, sessionID := range []string{"session-old", "session-new"} {
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: sessionID, TaskID: "task-transfer", State: models.TaskSessionStateWaitingForInput,
		}); err != nil {
			t.Fatal(err)
		}
	}
	queueRepo, err := messagequeue.NewSQLiteRepository(repo.db, repo.db)
	if err != nil {
		t.Fatal(err)
	}
	compensations := queueRepo.(interface {
		UpsertSessionTransferCompensation(context.Context, messagequeue.SessionTransferCompensation) error
	})
	if err := compensations.UpsertSessionTransferCompensation(ctx, messagequeue.SessionTransferCompensation{
		OperationID: "claim-transfer",
		TaskID:      "task-transfer", FromSessionID: "session-old", ToSessionID: "session-new",
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	attachment := &models.TaskMessageAttachment{
		ID: "claim-during-transfer", OwnerID: "owner-1", WorkspaceID: "workspace-attachments",
		Name: "during.txt", MimeType: "text/plain", Kind: "resource", DeliveryMode: "path",
		SizeBytes: 1, StorageKey: "claim-during-transfer", State: models.AttachmentStateStaged,
		ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	}
	if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
		t.Fatal(err)
	}

	err = repo.ClaimMessageAttachments(
		ctx,
		[]string{attachment.ID},
		attachment.OwnerID,
		attachment.WorkspaceID,
		"task-transfer",
		"session-old",
	)
	if !errors.Is(err, messagequeue.ErrSessionTransferInProgress) {
		t.Fatalf("claim error = %v, want %v", err, messagequeue.ErrSessionTransferInProgress)
	}
	stored, err := repo.GetMessageAttachment(ctx, attachment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.SessionID != "" || stored.State != models.AttachmentStateStaged {
		t.Fatalf("attachment after rejected claim = %#v", stored)
	}
}

func TestPostgresAttachmentClaimRejectsActiveSessionTransfer(t *testing.T) {
	repoA, repoB, _ := newTaskPostgresRepoPair(t)
	ctx := context.Background()
	seedWorkspace(t, repoA, "workspace-transfer-claim")
	if err := repoA.CreateTask(ctx, &models.Task{ID: "task-transfer", Title: "Attachment transfer"}); err != nil {
		t.Fatal(err)
	}
	for _, sessionID := range []string{"session-old", "session-new"} {
		if err := repoA.CreateTaskSession(ctx, &models.TaskSession{
			ID: sessionID, TaskID: "task-transfer", State: models.TaskSessionStateWaitingForInput,
		}); err != nil {
			t.Fatal(err)
		}
	}
	queueRepo, err := messagequeue.NewSQLiteRepository(repoA.db, repoA.db)
	if err != nil {
		t.Fatal(err)
	}
	compensations := queueRepo.(interface {
		UpsertSessionTransferCompensation(context.Context, messagequeue.SessionTransferCompensation) error
	})
	if err := compensations.UpsertSessionTransferCompensation(ctx, messagequeue.SessionTransferCompensation{
		OperationID: "postgres-claim-transfer",
		TaskID:      "task-transfer", FromSessionID: "session-old", ToSessionID: "session-new",
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	attachment := &models.TaskMessageAttachment{
		ID: "postgres-claim-during-transfer", OwnerID: "owner-1", WorkspaceID: "workspace-transfer-claim",
		Name: "during.txt", MimeType: "text/plain", Kind: "resource", DeliveryMode: "path",
		SizeBytes: 1, StorageKey: "postgres-claim-during-transfer", State: models.AttachmentStateStaged,
		ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	}
	if err := repoA.CreateMessageAttachment(ctx, attachment); err != nil {
		t.Fatal(err)
	}

	err = repoB.ClaimMessageAttachments(
		ctx,
		[]string{attachment.ID},
		attachment.OwnerID,
		attachment.WorkspaceID,
		"task-transfer",
		"session-old",
	)
	if !errors.Is(err, messagequeue.ErrSessionTransferInProgress) {
		t.Fatalf("claim error = %v, want %v", err, messagequeue.ErrSessionTransferInProgress)
	}
	stored, err := repoA.GetMessageAttachment(ctx, attachment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.SessionID != "" || stored.State != models.AttachmentStateStaged {
		t.Fatalf("attachment after rejected claim = %#v", stored)
	}
}
