package service

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func TestConversationForkCopiesSelectedAttachmentsIntoIndependentDraftStorage(t *testing.T) {
	svc, _, attachments, source, ctx := newConversationForkAttachmentFixture(t, "source-file.txt", "private source bytes")
	candidates, err := svc.ListConversationForkCandidates(ctx, models.ConversationForkSourceRequest{
		SessionID: "session-fork-service", CutoffMessageID: "message-fork-service",
	})
	if err != nil {
		t.Fatalf("list fork candidates: %v", err)
	}
	available := false
	for _, candidate := range candidates.AttachmentCandidates {
		if candidate.SourceID == source.ID {
			available = candidate.Available
		}
	}
	if !available {
		t.Fatalf("source attachment candidate is unavailable: %+v", candidates.AttachmentCandidates)
	}
	request := models.ConversationForkCreateRequest{
		Source: models.ConversationForkSourceRequest{
			SessionID: "session-fork-service", CutoffMessageID: "message-fork-service",
		},
		DraftRequestID: "fork-with-file",
		AttachmentIDs:  []string{source.ID},
	}
	draft, err := svc.CreateConversationForkDraft(ctx, request)
	if err != nil {
		t.Fatalf("create fork with selected attachment: %v", err)
	}
	if len(draft.Descriptor.AttachmentDescriptors) != 1 {
		t.Fatalf("fork attachment descriptors = %+v, want one copy", draft.Descriptor.AttachmentDescriptors)
	}
	copyDescriptor := draft.Descriptor.AttachmentDescriptors[0]
	if copyDescriptor.ID == "" || copyDescriptor.ID == source.ID || copyDescriptor.SourceID != source.ID || copyDescriptor.Size != source.SizeBytes || !draft.Descriptor.Estimate.AttachmentsUnmeasured {
		t.Fatalf("copy descriptor or estimate = %+v, estimate %+v", copyDescriptor, draft.Descriptor.Estimate)
	}
	if !strings.Contains(draft.CompiledText, "Attachment 1: source-file.txt") || strings.Contains(draft.CompiledText, source.StorageKey) {
		t.Fatalf("compiled attachment inventory leaks or omits source position: %s", draft.CompiledText)
	}
	copyAttachment, file, err := attachments.Open(ctx, "user-a", copyDescriptor.ID)
	if err != nil {
		t.Fatalf("open independent copy: %v", err)
	}
	buffer, err := io.ReadAll(file)
	if err != nil {
		_ = file.Close()
		t.Fatalf("read copied bytes: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close copied bytes: %v", err)
	}
	if string(buffer) != "private source bytes" || copyAttachment.State != models.AttachmentStateStaged {
		t.Fatalf("copied attachment = %+v with bytes %q", copyAttachment, buffer)
	}
	if _, err := attachments.Get(ctx, "user-a", source.ID); err != nil {
		t.Fatalf("source attachment was changed: %v", err)
	}

	retry, err := svc.CreateConversationForkDraft(ctx, request)
	if err != nil || retry.Descriptor.ID != draft.Descriptor.ID || retry.Descriptor.AttachmentDescriptors[0].ID != copyDescriptor.ID {
		t.Fatalf("idempotent fork retry = %+v, %v", retry.Descriptor, err)
	}
	if err := attachments.DeleteByTask(ctx, "task-fork-service"); err != nil {
		t.Fatalf("delete source task attachments: %v", err)
	}
	if _, err := attachments.Get(ctx, "user-a", source.ID); err == nil {
		t.Fatal("source task deletion kept its claimed attachment")
	}
	_, file, err = attachments.Open(ctx, "user-a", copyDescriptor.ID)
	if err != nil {
		t.Fatalf("open copy after source deletion: %v", err)
	}
	buffer, err = io.ReadAll(file)
	if err != nil {
		_ = file.Close()
		t.Fatalf("read copy after source deletion: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close copied bytes after source deletion: %v", err)
	}
	if string(buffer) != "private source bytes" {
		t.Fatalf("copy after source deletion = %q", buffer)
	}

	if err := svc.DiscardConversationForkDraft(ctx, draft.Descriptor.ID); err != nil {
		t.Fatalf("discard fork copies: %v", err)
	}
	if _, err := attachments.Get(ctx, "user-a", copyDescriptor.ID); err == nil {
		t.Fatal("discard kept the draft-owned attachment copy")
	}
}

func TestConversationForkExpiryCleanupDeletesDraftOwnedAttachmentCopies(t *testing.T) {
	svc, repo, attachments, source, ctx := newConversationForkAttachmentFixture(t, "source-file.txt", "private source bytes")
	draft, err := svc.CreateConversationForkDraft(ctx, models.ConversationForkCreateRequest{
		Source: models.ConversationForkSourceRequest{
			SessionID: "session-fork-service", CutoffMessageID: "message-fork-service",
		},
		DraftRequestID: "fork-expiry-copy",
		AttachmentIDs:  []string{source.ID},
	})
	if err != nil {
		t.Fatalf("create fork with selected attachment: %v", err)
	}
	copyID := draft.Descriptor.AttachmentDescriptors[0].ID
	if _, err := repo.DB().ExecContext(ctx, `UPDATE task_conversation_forks SET expires_at = ? WHERE id = ?`, time.Now().UTC().Add(-time.Minute), draft.Descriptor.ID); err != nil {
		t.Fatalf("expire fork draft: %v", err)
	}
	svc.cleanupExpiredConversationForkDraftRows(ctx, time.Now().UTC())
	if _, err := attachments.Get(ctx, "user-a", copyID); !errors.Is(err, ErrAttachmentNotFound) {
		t.Fatalf("expired draft retained attachment copy: %v", err)
	}
	if _, err := svc.GetConversationForkDraft(ctx, draft.Descriptor.ID); !errors.Is(err, models.ErrConversationForkNotFound) {
		t.Fatalf("expired draft remains readable after cleanup: %v", err)
	}
}

func TestConversationForkAttachmentCopyFailureKeepsExistingDraft(t *testing.T) {
	svc, repo, attachments, source, ctx := newConversationForkAttachmentFixture(t, "source-file.txt", "private source bytes")
	firstRequest := models.ConversationForkCreateRequest{
		Source:         models.ConversationForkSourceRequest{SessionID: "session-fork-service", CutoffMessageID: "message-fork-service"},
		DraftRequestID: "fork-without-file",
	}
	first, err := svc.CreateConversationForkDraft(ctx, firstRequest)
	if err != nil {
		t.Fatalf("create previous ready draft: %v", err)
	}
	if _, err := repo.DB().ExecContext(ctx, `UPDATE task_message_attachments SET name = ? WHERE id = ?`, "../invalid.txt", source.ID); err != nil {
		t.Fatalf("make source name invalid for Stage validation: %v", err)
	}
	failed, err := svc.CreateConversationForkDraft(ctx, models.ConversationForkCreateRequest{
		Source: firstRequest.Source, DraftRequestID: "replacement-fails", AttachmentIDs: []string{source.ID},
	})
	if err == nil || failed.Descriptor.ID != "" {
		t.Fatalf("invalid copy result = %+v, %v; want failure without a ready replacement", failed, err)
	}
	stillReady, err := svc.GetConversationForkDraft(ctx, first.Descriptor.ID)
	if err != nil || stillReady.Descriptor.ID != first.Descriptor.ID {
		t.Fatalf("previous draft after replacement failure = %+v, %v", stillReady.Descriptor, err)
	}
	if _, err := attachments.Get(ctx, "user-a", source.ID); err != nil {
		t.Fatalf("failed replacement removed source attachment: %v", err)
	}
	var stagedCopies int
	if err := repo.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM task_message_attachments WHERE owner_id = 'user-a' AND workspace_id = 'workspace-fork-service' AND state = 'staged'`).Scan(&stagedCopies); err != nil {
		t.Fatalf("count partial copies: %v", err)
	}
	if stagedCopies != 0 {
		t.Fatalf("failed replacement left %d staged copies", stagedCopies)
	}
}

func TestConversationForkSelectedAttachmentMustBelongToTheSelectedRange(t *testing.T) {
	svc, _, _, source, ctx := newConversationForkAttachmentFixture(t, "source-file.txt", "private source bytes")
	if _, err := svc.CreateConversationForkDraft(ctx, models.ConversationForkCreateRequest{
		Source:         models.ConversationForkSourceRequest{SessionID: "session-fork-service", CutoffMessageID: "message-fork-service"},
		DraftRequestID: "fork-foreign-file", AttachmentIDs: []string{source.ID, "outside-source-range"},
	}); !errors.Is(err, models.ErrConversationForkAttachmentMissing) {
		t.Fatalf("out-of-range attachment error = %v, want missing attachment", err)
	}
}

func TestConversationForkSelectedAttachmentAggregateLimitLeavesNoCopies(t *testing.T) {
	svc, repo, _, source, ctx := newConversationForkAttachmentFixture(t, "source-file.txt", "private source bytes")
	second, err := svc.attachmentSvc.Stage(ctx, "user-a", "workspace-fork-service", "second.txt", "text/plain", "resource", "prompt", strings.NewReader("second"))
	if err != nil {
		t.Fatalf("stage second source attachment: %v", err)
	}
	if _, err := repo.DB().ExecContext(ctx, `
		UPDATE task_message_attachments
		SET task_id = ?, session_id = ?, message_id = ?, state = ?, size_bytes = ?
		WHERE id = ?
	`, "task-fork-service", "session-fork-service", "message-fork-service", models.AttachmentStateClaimed, models.MaxMessageAttachmentBytes*3/5+1, second.ID); err != nil {
		t.Fatalf("claim second source attachment: %v", err)
	}
	if _, err := repo.DB().ExecContext(ctx, `UPDATE task_message_attachments SET size_bytes = ? WHERE id = ?`, models.MaxMessageAttachmentBytes*3/5+1, source.ID); err != nil {
		t.Fatalf("raise source attachment size: %v", err)
	}
	if _, err := svc.CreateConversationForkDraft(ctx, models.ConversationForkCreateRequest{
		Source:         models.ConversationForkSourceRequest{SessionID: "session-fork-service", CutoffMessageID: "message-fork-service"},
		DraftRequestID: "fork-too-many-bytes", AttachmentIDs: []string{source.ID, second.ID},
	}); !errors.Is(err, models.ErrConversationForkLimitExceeded) {
		t.Fatalf("aggregate attachment error = %v, want size limit", err)
	}
	var stagedCopies int
	if err := repo.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM task_message_attachments WHERE owner_id = 'user-a' AND state = 'staged'`).Scan(&stagedCopies); err != nil {
		t.Fatalf("count after aggregate rejection: %v", err)
	}
	if stagedCopies != 0 {
		t.Fatalf("aggregate rejection left %d staged copies", stagedCopies)
	}
}

func newConversationForkAttachmentFixture(t *testing.T, name, body string) (*Service, *sqliterepo.Repository, *AttachmentService, *models.TaskMessageAttachment, context.Context) {
	t.Helper()
	svc, _, repo := createTestService(t)
	seedConversationForkServiceSource(t, repo)
	ctx := ctxAs("user-a")
	attachments, err := NewAttachmentService(repo, t.TempDir(), nil, accessTestLogger(t))
	if err != nil {
		t.Fatalf("create attachment service: %v", err)
	}
	attachments.SetTaskAuthorizer(svc.AuthorizeTaskAccess)
	svc.SetAttachmentService(attachments)
	source, err := attachments.Stage(ctx, "user-a", "workspace-fork-service", name, "text/plain", "resource", "prompt", strings.NewReader(body))
	if err != nil {
		t.Fatalf("stage source attachment: %v", err)
	}
	if _, err := repo.DB().ExecContext(ctx, `
		UPDATE task_message_attachments
		SET task_id = ?, session_id = ?, message_id = ?, state = ?, expires_at = ?
		WHERE id = ?
	`, "task-fork-service", "session-fork-service", "message-fork-service", models.AttachmentStateClaimed, time.Now().UTC().Add(time.Hour), source.ID); err != nil {
		t.Fatalf("claim source attachment on source message: %v", err)
	}
	return svc, repo, attachments, source, ctx
}
