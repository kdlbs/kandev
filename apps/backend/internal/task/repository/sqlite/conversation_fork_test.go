package sqlite

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestConversationForkSnapshot(t *testing.T) {
	repo := newRepoForSessionTests(t)
	var count int
	if err := repo.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM task_conversation_forks`).Scan(&count); err != nil {
		t.Fatalf("read conversation fork snapshot table: %v", err)
	}
	if count != 0 {
		t.Fatalf("new repository contains %d conversation fork snapshots, want 0", count)
	}
}

func TestConversationForkSnapshotRangeUsesStableInclusiveOrdering(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-fork-source", "session-fork-source", "turn-fork-source")
	stamp := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workspaces(id, name, owner_id, created_at, updated_at) VALUES (?, 'Source files', 'owner-a', ?, ?)
	`), "workspace-fork-source", stamp, stamp); err != nil {
		t.Fatalf("create source workspace: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE tasks SET workspace_id = ? WHERE id = ?`), "workspace-fork-source", "task-fork-source"); err != nil {
		t.Fatalf("attach source task to workspace: %v", err)
	}
	insertAgentMsg(t, repo, "message-c", "session-fork-source", "turn-fork-source", "user", "third", stamp)
	insertAgentMsg(t, repo, "message-a", "session-fork-source", "turn-fork-source", "user", "first", stamp)
	insertAgentMsg(t, repo, "message-b", "session-fork-source", "turn-fork-source", "user", "second", stamp)
	insertAgentMsg(t, repo, "message-later", "session-fork-source", "turn-fork-source", "user", "later", stamp.Add(time.Minute))
	if err := repo.CreateMessageAttachment(ctx, &models.TaskMessageAttachment{
		ID: "attachment-fork-source", OwnerID: "owner-a", WorkspaceID: "workspace-fork-source",
		TaskID: "task-fork-source", SessionID: "session-fork-source", MessageID: "message-b",
		Name: "spec.md", MimeType: "text/markdown", Kind: "resource", DeliveryMode: "prompt",
		SizeBytes: 42, StorageKey: "attachment-fork-source", State: models.AttachmentStateClaimed,
		ExpiresAt: stamp.Add(time.Hour),
	}); err != nil {
		t.Fatalf("create source attachment: %v", err)
	}

	source, err := repo.ReadConversationForkSource(ctx, models.ConversationForkSourceRequest{SessionID: "session-fork-source", CutoffMessageID: "message-b"})
	if err != nil {
		t.Fatalf("read conversation fork source: %v", err)
	}
	if source.SessionID != "session-fork-source" || source.TaskID != "task-fork-source" || source.Revision != 5 {
		t.Fatalf("source metadata = %+v", source)
	}
	if len(source.Messages) != 2 || source.Messages[0].ID != "message-a" || source.Messages[1].ID != "message-b" {
		t.Fatalf("inclusive cutoff rows = %#v, want message-a then message-b", source.Messages)
	}
	if len(source.Attachments) != 1 || source.Attachments[0].ID != "attachment-fork-source" {
		t.Fatalf("source attachment candidates = %+v, want the attachment within the selected range", source.Attachments)
	}

	ranged, err := repo.ReadConversationForkSource(ctx, models.ConversationForkSourceRequest{SessionID: "session-fork-source", StartMessageID: "message-b", CutoffMessageID: "message-c"})
	if err != nil {
		t.Fatalf("read narrowed conversation fork source: %v", err)
	}
	if len(ranged.Messages) != 2 || ranged.Messages[0].ID != "message-b" || ranged.Messages[1].ID != "message-c" {
		t.Fatalf("narrowed rows = %#v, want message-b then message-c", ranged.Messages)
	}
	if _, err := repo.ReadConversationForkSource(ctx, models.ConversationForkSourceRequest{SessionID: "session-fork-source", CutoffMessageID: "missing"}); !errors.Is(err, models.ErrConversationForkCutoffUnavailable) {
		t.Fatalf("missing cutoff error = %v, want cutoff unavailable", err)
	}
	if _, err := repo.readConversationForkSourceWithLimits(ctx, models.ConversationForkSourceRequest{SessionID: "session-fork-source", CutoffMessageID: "message-later"}, 2, 1<<20); !errors.Is(err, models.ErrConversationForkLimitExceeded) {
		t.Fatalf("row limit error = %v, want storage limit", err)
	}
	if _, err := repo.readConversationForkSourceWithLimits(ctx, models.ConversationForkSourceRequest{SessionID: "session-fork-source", CutoffMessageID: "message-later"}, 10, 4); !errors.Is(err, models.ErrConversationForkLimitExceeded) {
		t.Fatalf("byte limit error = %v, want storage limit", err)
	}
}

func TestConversationForkSourceRehydratesSelectedToolPayloadInSnapshot(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-fork-payload", "session-fork-payload", "turn-fork-payload")
	stamp := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	if err := repo.CreateMessage(ctx, &models.Message{
		ID: "tool-fork-payload", TaskID: "task-fork-payload", TaskSessionID: "session-fork-payload", TurnID: "turn-fork-payload",
		AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeToolExecute, Content: "Ran tests",
		Metadata: map[string]interface{}{"normalized": map[string]interface{}{"kind": "shell_exec", "shell_exec": map[string]interface{}{
			"command": "go test ./...", "output": map[string]interface{}{"stdout": strings.Repeat("test-output-", 600), "stderr": "", "exit_code": 0},
		}}}, CreatedAt: stamp, UpdatedAt: stamp,
	}); err != nil {
		t.Fatalf("create large tool message: %v", err)
	}
	insertAgentMsg(t, repo, "user-fork-payload-cutoff", "session-fork-payload", "turn-fork-payload", "user", "Continue.", stamp.Add(time.Minute))
	source, err := repo.ReadConversationForkSource(ctx, models.ConversationForkSourceRequest{
		SessionID: "session-fork-payload", CutoffMessageID: "user-fork-payload-cutoff", IncludeToolEvidence: true,
	})
	if err != nil {
		t.Fatalf("read source with tool payload: %v", err)
	}
	if len(source.Messages) != 2 || source.Messages[0].PayloadDigest != "" {
		t.Fatalf("tool payload message was not rehydrated in selected source: %+v", source.Messages)
	}
	output, ok := models.ExtractShellExecOutput(source.Messages[0].Metadata)
	if !ok || len(output.Stdout) < 6000 {
		t.Fatalf("recovered shell output = (%v, %d bytes), want payload contents", ok, len(output.Stdout))
	}
}

func TestConversationForkSourceWithoutToolEvidenceDoesNotCountToolPayloadBytes(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-fork-large-tool", "session-fork-large-tool", "turn-fork-large-tool")
	stamp := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	if err := repo.CreateMessage(ctx, &models.Message{
		ID: "tool-fork-large-payload", TaskID: "task-fork-large-tool", TaskSessionID: "session-fork-large-tool",
		TurnID: "turn-fork-large-tool", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeToolExecute,
		Content: "Ran command", CreatedAt: stamp, UpdatedAt: stamp,
	}); err != nil {
		t.Fatalf("create tool message: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		UPDATE task_session_messages SET payload_size = ? WHERE id = ?
	`), conversationForkMaxSourceBytes+1, "tool-fork-large-payload"); err != nil {
		t.Fatalf("set retained payload size: %v", err)
	}
	if err := repo.CreateMessage(ctx, &models.Message{
		ID: "user-fork-large-cutoff", TaskID: "task-fork-large-tool", TaskSessionID: "session-fork-large-tool",
		TurnID: "turn-fork-large-tool", AuthorType: models.MessageAuthorUser, Type: models.MessageTypeMessage,
		Content: "Continue without tool output", CreatedAt: stamp.Add(time.Minute), UpdatedAt: stamp.Add(time.Minute),
	}); err != nil {
		t.Fatalf("create cutoff message: %v", err)
	}

	source, err := repo.ReadConversationForkSource(ctx, models.ConversationForkSourceRequest{
		SessionID: "session-fork-large-tool", CutoffMessageID: "user-fork-large-cutoff", IncludeToolEvidence: false,
	})
	if err != nil {
		t.Fatalf("read source without tool evidence: %v", err)
	}
	if len(source.Messages) != 2 {
		t.Fatalf("source has %d messages, want 2", len(source.Messages))
	}
}

func TestConversationForkAttachmentCandidatesAreBoundedAndPaged(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-fork-pages", "session-fork-pages", "turn-fork-pages")
	now := time.Now().UTC()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-fork-pages", Name: "Fork pages", OwnerID: "owner-a"}); err != nil {
		t.Fatalf("create candidate workspace: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE tasks SET workspace_id = ? WHERE id = ?`), "workspace-fork-pages", "task-fork-pages"); err != nil {
		t.Fatalf("bind candidate task workspace: %v", err)
	}
	insertAgentMsg(t, repo, "message-fork-pages", "session-fork-pages", "turn-fork-pages", "user", "Select files.", now)
	for index := 0; index < conversationForkCandidatePageSize+1; index++ {
		id := fmt.Sprintf("fork-page-attachment-%03d", index)
		if err := repo.CreateMessageAttachment(ctx, &models.TaskMessageAttachment{
			ID: id, OwnerID: "owner-a", WorkspaceID: "workspace-fork-pages", TaskID: "task-fork-pages",
			SessionID: "session-fork-pages", MessageID: "message-fork-pages", Name: fmt.Sprintf("file-%03d.txt", index),
			MimeType: "text/plain", Kind: "resource", DeliveryMode: "prompt", SizeBytes: 1,
			StorageKey: id, State: models.AttachmentStateClaimed, ExpiresAt: now.Add(time.Hour),
		}); err != nil {
			t.Fatalf("create candidate %d: %v", index, err)
		}
	}
	first, err := repo.ReadConversationForkSource(ctx, models.ConversationForkSourceRequest{SessionID: "session-fork-pages", CutoffMessageID: "message-fork-pages"})
	if err != nil {
		t.Fatalf("read first candidate page: %v", err)
	}
	if len(first.Attachments) != conversationForkCandidatePageSize || !first.AttachmentsHasMore || first.AttachmentCursor != first.Attachments[len(first.Attachments)-1].ID {
		t.Fatalf("first attachment page count=%d more=%v cursor=%q", len(first.Attachments), first.AttachmentsHasMore, first.AttachmentCursor)
	}
	second, err := repo.ReadConversationForkSource(ctx, models.ConversationForkSourceRequest{
		SessionID: "session-fork-pages", CutoffMessageID: "message-fork-pages", AttachmentCursor: first.AttachmentCursor,
	})
	if err != nil {
		t.Fatalf("read second candidate page: %v", err)
	}
	if len(second.Attachments) != 1 || second.AttachmentsHasMore || second.Attachments[0].ID != "fork-page-attachment-100" {
		t.Fatalf("second attachment page = %+v, more=%v", second.Attachments, second.AttachmentsHasMore)
	}
}

func TestConversationForkDraftStorageQuotaIdempotencyAndRetention(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workspaces(id, name, owner_id, created_at, updated_at) VALUES (?, 'Fork test', 'owner-a', ?, ?)
	`), "workspace-fork-drafts", now, now); err != nil {
		t.Fatalf("create draft workspace: %v", err)
	}

	first, err := repo.CreateConversationForkDraft(ctx, newConversationForkDraft("request-0", "hash-0", now.Add(time.Hour)))
	if err != nil {
		t.Fatalf("create first fork draft: %v", err)
	}
	retry, err := repo.CreateConversationForkDraft(ctx, newConversationForkDraft("request-0", "hash-0", now.Add(time.Hour)))
	if err != nil || retry.Descriptor.ID != first.Descriptor.ID {
		t.Fatalf("idempotent create returned (%q, %v), want original %q", retry.Descriptor.ID, err, first.Descriptor.ID)
	}
	if _, err := repo.CreateConversationForkDraft(ctx, newConversationForkDraft("request-0", "different", now.Add(time.Hour))); !errors.Is(err, models.ErrConversationForkConflict) {
		t.Fatalf("conflicting request error = %v, want conflict", err)
	}
	loaded, err := repo.GetConversationForkDraft(ctx, "owner-a", first.Descriptor.ID, now)
	if err != nil || loaded.CompiledText != "frozen context" || loaded.Descriptor.ContentHash != "hash-0" {
		t.Fatalf("loaded draft = %+v, %v", loaded, err)
	}
	if _, err := repo.GetConversationForkDraft(ctx, "owner-b", first.Descriptor.ID, now); !errors.Is(err, models.ErrConversationForkNotFound) {
		t.Fatalf("foreign-owner read error = %v, want not found", err)
	}
	if err := repo.DiscardConversationForkDraft(ctx, "owner-a", first.Descriptor.ID); err != nil {
		t.Fatalf("discard draft: %v", err)
	}
	discarded, err := repo.GetConversationForkDraft(ctx, "owner-a", first.Descriptor.ID, now)
	if err != nil || discarded.Descriptor.State != "discarded" || discarded.CompiledText != "" || discarded.Descriptor.TextBytes != 0 {
		t.Fatalf("discarded draft retained transcript data: %+v, %v", discarded.Descriptor, err)
	}
	if err := repo.DiscardConversationForkDraft(ctx, "owner-a", first.Descriptor.ID); err != nil {
		t.Fatalf("idempotent discard: %v", err)
	}

	var firstActive models.ConversationForkDraft
	for i := 1; i <= 20; i++ {
		created, err := repo.CreateConversationForkDraft(ctx, newConversationForkDraft(fmt.Sprintf("request-%d", i), fmt.Sprintf("hash-%d", i), now.Add(time.Hour)))
		if err != nil {
			t.Fatalf("create draft %d: %v", i, err)
		}
		if i == 1 {
			firstActive = created
		}
	}
	var activeCount int
	if err := repo.db.QueryRowContext(ctx, repo.db.Rebind(`SELECT COUNT(*) FROM task_conversation_forks WHERE owner_id = ? AND state = 'draft' AND expires_at > ?`), "owner-a", now).Scan(&activeCount); err != nil {
		t.Fatalf("count active drafts: %v", err)
	}
	if activeCount != 20 {
		t.Fatalf("active drafts = %d, want 20", activeCount)
	}
	if _, err := repo.CreateConversationForkDraft(ctx, newConversationForkDraft("request-over-quota", "quota", now.Add(time.Hour))); !errors.Is(err, models.ErrConversationForkQuotaExceeded) {
		t.Fatalf("quota error = %v, want quota exceeded", err)
	}
	// A retry still resolves to its durable result after the active quota fills.
	if retry, err := repo.CreateConversationForkDraft(ctx, newConversationForkDraft("request-0", "hash-0", now.Add(time.Hour))); err != nil || retry.Descriptor.ID != first.Descriptor.ID {
		t.Fatalf("quota-full idempotent retry = %q, %v", retry.Descriptor.ID, err)
	}
	if err := repo.DiscardConversationForkDraft(ctx, "owner-a", firstActive.Descriptor.ID); err != nil {
		t.Fatalf("discard one active draft before expiry test: %v", err)
	}

	expiredDraft := newConversationForkDraft("request-expired", "expired", now.Add(time.Minute))
	expiredDraft.Descriptor.AttachmentDescriptors = []models.ConversationForkAttachment{{ID: "expired-copy", Name: "expired.txt", Size: 10, Available: true}}
	expired, err := repo.CreateConversationForkDraft(ctx, expiredDraft)
	if err != nil {
		t.Fatalf("create expiring draft: %v", err)
	}
	if _, err := repo.GetConversationForkDraft(ctx, "owner-a", expired.Descriptor.ID, now.Add(2*time.Minute)); !errors.Is(err, models.ErrConversationForkExpired) {
		t.Fatalf("expired draft error = %v, want expired", err)
	}
	cleaned, err := repo.DeleteExpiredConversationForkDrafts(ctx, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("delete expired drafts: %v", err)
	}
	foundExpiredCopy := false
	for _, item := range cleaned {
		for _, attachment := range item.Attachments {
			if item.OwnerID == "owner-a" && attachment.ID == "expired-copy" {
				foundExpiredCopy = true
			}
		}
	}
	if !foundExpiredCopy {
		t.Fatalf("expired attachment cleanup result omitted expired copy: %+v", cleaned)
	}
	if _, err := repo.GetConversationForkDraft(ctx, "owner-a", expired.Descriptor.ID, now.Add(2*time.Minute)); !errors.Is(err, models.ErrConversationForkNotFound) {
		t.Fatalf("expired draft after retention cleanup = %v, want not found", err)
	}
}

func TestConversationForkDestinationCompletionMigrationAddsFalseDefault(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if _, err := repo.db.ExecContext(ctx, `DROP TABLE task_conversation_forks`); err != nil {
		t.Fatalf("drop current fork table: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `CREATE TABLE task_conversation_forks (id TEXT PRIMARY KEY, state TEXT NOT NULL)`); err != nil {
		t.Fatalf("create legacy fork table: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `INSERT INTO task_conversation_forks(id, state) VALUES ('legacy-fork', 'attached')`); err != nil {
		t.Fatalf("insert legacy fork row: %v", err)
	}
	if err := repo.migrateConversationForkDestinationComplete(); err != nil {
		t.Fatalf("migrate destination completion column: %v", err)
	}
	var destinationComplete bool
	if err := repo.db.QueryRowContext(ctx, `SELECT destination_complete FROM task_conversation_forks WHERE id = 'legacy-fork'`).Scan(&destinationComplete); err != nil {
		t.Fatalf("read migrated completion value: %v", err)
	}
	if destinationComplete {
		t.Fatal("legacy fork receipt should remain incomplete until synchronous destination work finishes")
	}
}

func newConversationForkDraft(requestID, contentHash string, expiresAt time.Time) *models.ConversationForkDraft {
	return &models.ConversationForkDraft{
		OwnerID:            "owner-a",
		WorkspaceID:        "workspace-fork-drafts",
		CompiledText:       "frozen context",
		DraftRequestID:     requestID,
		RequestFingerprint: contentHash,
		Descriptor: models.ConversationForkDescriptor{
			SourceTaskID:    "source-task",
			SourceSessionID: "source-session",
			SourceMessageID: "source-message",
			CompilerVersion: "conversation-fork-v1",
			ContentHash:     contentHash,
			Omissions:       map[string]int{},
			Estimate:        models.ConversationForkEstimate{EstimatedTokens: 3, Method: "o200k_base:conversation-fork-v1"},
			ExpiresAt:       expiresAt,
			State:           "draft",
		},
	}
}
