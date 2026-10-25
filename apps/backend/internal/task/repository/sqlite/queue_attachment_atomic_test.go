package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

func TestQueueAdmissionClaimsAttachmentsAtomically(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-queue-claim")
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-queue-claim", WorkspaceID: "workspace-queue-claim", Title: "Queue claim"}); err != nil {
		t.Fatal(err)
	}
	session := &models.TaskSession{ID: "session-queue-claim", TaskID: "task-queue-claim"}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	attachment := &models.TaskMessageAttachment{
		ID: "attachment-queue-claim", OwnerID: "owner-queue-claim", WorkspaceID: "workspace-queue-claim",
		Name: "notes.txt", MimeType: "text/plain", Kind: "resource", DeliveryMode: "path",
		SizeBytes: 5, StorageKey: "attachment-queue-claim", State: models.AttachmentStateStaged,
		ExpiresAt: time.Now().UTC().Add(time.Hour), CreatedAt: time.Now().UTC(),
	}
	if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
		t.Fatal(err)
	}

	db := sqlx.NewDb(repo.DB(), "sqlite3")
	queueRepo, err := messagequeue.NewSQLiteRepository(db, db)
	if err != nil {
		t.Fatal(err)
	}
	queue := messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, logger.Default())
	identity := messagequeue.QueueSessionIdentity{
		TaskID: session.TaskID, SessionID: session.ID, SessionIncarnationID: session.QueueIncarnationID,
	}
	queued, err := queue.QueueMessageWithMetadataForSessionWithClaim(
		ctx, identity, "review", "", messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{{AttachmentID: attachment.ID, Name: attachment.Name, DeliveryMode: attachment.DeliveryMode}},
		nil,
		messagequeue.QueueAttachmentClaim{OwnerID: attachment.OwnerID, WorkspaceID: attachment.WorkspaceID, IDs: []string{attachment.ID}},
	)
	if err != nil {
		t.Fatalf("queue with attachment claim: %v", err)
	}
	if queued == nil {
		t.Fatal("queued message is nil")
	}
	stored, err := repo.GetMessageAttachment(ctx, attachment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != models.AttachmentStateClaimed || stored.TaskID != session.TaskID || stored.SessionID != session.ID {
		t.Fatalf("attachment claim = %+v", stored)
	}
}

func TestQueueAdmissionRollsBackAttachmentClaimWhenInsertFails(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-queue-rollback")
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-queue-rollback", WorkspaceID: "workspace-queue-rollback", Title: "Queue rollback"}); err != nil {
		t.Fatal(err)
	}
	session := &models.TaskSession{ID: "session-queue-rollback", TaskID: "task-queue-rollback"}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	attachment := &models.TaskMessageAttachment{
		ID: "attachment-queue-rollback", OwnerID: "owner-queue-rollback", WorkspaceID: "workspace-queue-rollback",
		Name: "notes.txt", MimeType: "text/plain", Kind: "resource", DeliveryMode: "path",
		SizeBytes: 5, StorageKey: "attachment-queue-rollback", State: models.AttachmentStateStaged,
		ExpiresAt: time.Now().UTC().Add(time.Hour), CreatedAt: time.Now().UTC(),
	}
	if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
		t.Fatal(err)
	}

	db := sqlx.NewDb(repo.DB(), "sqlite3")
	queueRepo, err := messagequeue.NewSQLiteRepository(db, db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DB().Exec(`
		CREATE TRIGGER fail_queue_insert
		BEFORE INSERT ON queued_messages
		BEGIN
			SELECT RAISE(ABORT, 'forced queue insert failure');
		END
	`); err != nil {
		t.Fatal(err)
	}
	queue := messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, logger.Default())
	identity := messagequeue.QueueSessionIdentity{
		TaskID: session.TaskID, SessionID: session.ID, SessionIncarnationID: session.QueueIncarnationID,
	}
	_, err = queue.QueueMessageWithMetadataForSessionWithClaim(
		ctx, identity, "review", "", messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{{AttachmentID: attachment.ID, Name: attachment.Name, DeliveryMode: attachment.DeliveryMode}},
		nil,
		messagequeue.QueueAttachmentClaim{OwnerID: attachment.OwnerID, WorkspaceID: attachment.WorkspaceID, IDs: []string{attachment.ID}},
	)
	if err == nil {
		t.Fatal("queue admission with forced insert failure succeeded")
	}
	status, snapshotErr := queue.Snapshot(ctx, identity)
	if snapshotErr != nil {
		t.Fatal(snapshotErr)
	}
	if status.Count != 0 {
		t.Fatalf("queue count = %d, want 0", status.Count)
	}
	stored, err := repo.GetMessageAttachment(ctx, attachment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != models.AttachmentStateStaged || stored.TaskID != "" || stored.SessionID != "" {
		t.Fatalf("attachment changed after rollback = %+v", stored)
	}
}

func TestQueueAdmissionAtCapacityDoesNotClaimAttachments(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-queue-full-claim")
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-queue-full-claim", WorkspaceID: "workspace-queue-full-claim", Title: "Queue full claim",
	}); err != nil {
		t.Fatal(err)
	}
	session := &models.TaskSession{ID: "session-queue-full-claim", TaskID: "task-queue-full-claim"}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	attachment := &models.TaskMessageAttachment{
		ID: "attachment-queue-full-claim", OwnerID: "owner-queue-full-claim", WorkspaceID: "workspace-queue-full-claim",
		Name: "notes.txt", MimeType: "text/plain", Kind: "resource", DeliveryMode: "path",
		SizeBytes: 5, StorageKey: "attachment-queue-full-claim", State: models.AttachmentStateStaged,
		ExpiresAt: time.Now().UTC().Add(time.Hour), CreatedAt: time.Now().UTC(),
	}
	if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
		t.Fatal(err)
	}

	db := sqlx.NewDb(repo.DB(), "sqlite3")
	queueRepo, err := messagequeue.NewSQLiteRepository(db, db)
	if err != nil {
		t.Fatal(err)
	}
	queue := messagequeue.NewService(queueRepo, 1, logger.Default())
	identity := messagequeue.QueueSessionIdentity{
		TaskID: session.TaskID, SessionID: session.ID, SessionIncarnationID: session.QueueIncarnationID,
	}
	first, err := queue.QueueMessageWithMetadataForSession(
		ctx, identity, "first", "", messagequeue.QueuedByUser, false, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	queued, err := queue.QueueMessageWithMetadataForSessionWithClaim(
		ctx, identity, "second", "", messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{{
			AttachmentID: attachment.ID, Name: attachment.Name, DeliveryMode: attachment.DeliveryMode,
		}},
		nil,
		messagequeue.QueueAttachmentClaim{
			OwnerID: attachment.OwnerID, WorkspaceID: attachment.WorkspaceID, IDs: []string{attachment.ID},
		},
	)
	if !errors.Is(err, messagequeue.ErrQueueFull) || queued != nil {
		t.Fatalf("full queue attachment admission = %+v, err=%v, want queue full", queued, err)
	}
	status, err := queue.Snapshot(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}
	if status.Count != 1 || status.Entries[0].ID != first.ID || status.Entries[0].Content != "first" {
		t.Fatalf("queue changed after rejected attachment admission: %+v", status)
	}
	stored, err := repo.GetMessageAttachment(ctx, attachment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != models.AttachmentStateStaged || stored.TaskID != "" || stored.SessionID != "" {
		t.Fatalf("attachment changed after full queue rejection = %+v", stored)
	}
}

func TestQueueTransferMovesAttachmentClaimToDestinationSession(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-queue-transfer-claim")
	task := &models.Task{
		ID: "task-queue-transfer-claim", WorkspaceID: "workspace-queue-transfer-claim", Title: "Queue transfer claim",
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	source := &models.TaskSession{ID: "session-queue-transfer-source", TaskID: task.ID}
	destination := &models.TaskSession{ID: "session-queue-transfer-destination", TaskID: task.ID}
	if err := repo.CreateTaskSession(ctx, source); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, destination); err != nil {
		t.Fatal(err)
	}
	attachment := &models.TaskMessageAttachment{
		ID: "attachment-queue-transfer", OwnerID: "owner-queue-transfer", WorkspaceID: task.WorkspaceID,
		TaskID: task.ID, SessionID: source.ID, Name: "notes.txt", MimeType: "text/plain",
		Kind: "resource", DeliveryMode: "path", SizeBytes: 5, StorageKey: "attachment-queue-transfer",
		State: models.AttachmentStateClaimed, ExpiresAt: time.Now().UTC().Add(time.Hour), CreatedAt: time.Now().UTC(),
	}
	if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
		t.Fatal(err)
	}

	db := sqlx.NewDb(repo.DB(), "sqlite3")
	queueRepo, err := messagequeue.NewSQLiteRepository(db, db)
	if err != nil {
		t.Fatal(err)
	}
	sourceIdentity := messagequeue.QueueSessionIdentity{
		TaskID: task.ID, SessionID: source.ID, SessionIncarnationID: source.QueueIncarnationID,
	}
	destinationIdentity := messagequeue.QueueSessionIdentity{
		TaskID: task.ID, SessionID: destination.ID, SessionIncarnationID: destination.QueueIncarnationID,
	}
	if err := queueRepo.InsertForSession(ctx, sourceIdentity, &messagequeue.QueuedMessage{
		ID: "queue-transfer-attachment", TaskID: task.ID, SessionID: source.ID, Content: "review",
		QueuedBy: messagequeue.QueuedByUser, QueuedAt: time.Now().UTC(),
		Attachments: []messagequeue.MessageAttachment{{
			AttachmentID: attachment.ID, Name: attachment.Name, DeliveryMode: attachment.DeliveryMode,
		}},
	}, messagequeue.DefaultMaxPerSession); err != nil {
		t.Fatalf("queue source attachment: %v", err)
	}
	retained, err := repo.DeleteClaimedMessageAttachments(
		ctx, []string{attachment.ID}, attachment.OwnerID, task.ID, source.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(retained) != 0 {
		t.Fatalf("released attachment still referenced by queue: %+v", retained)
	}
	if _, err := repo.GetMessageAttachment(ctx, attachment.ID); err != nil {
		t.Fatalf("queued attachment was removed: %v", err)
	}
	if err := queueRepo.TransferSessionIdentities(ctx, sourceIdentity, destinationIdentity); err != nil {
		t.Fatalf("transfer queue: %v", err)
	}
	stored, err := repo.GetMessageAttachment(ctx, attachment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.SessionID != destination.ID || stored.TaskID != task.ID || stored.State != models.AttachmentStateClaimed {
		t.Fatalf("transferred attachment claim = %+v", stored)
	}
	removed, err := queueRepo.DeleteByIDForSession(ctx, destinationIdentity, "queue-transfer-attachment")
	if err != nil {
		t.Fatal(err)
	}
	if len(removed.Removed) != 1 || removed.Removed[0].ID != "queue-transfer-attachment" {
		t.Fatalf("removed queue rows = %+v", removed)
	}
	released, err := repo.DeleteClaimedMessageAttachments(
		ctx, []string{attachment.ID}, attachment.OwnerID, task.ID, destination.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(released) != 1 || released[0].StorageKey != attachment.StorageKey {
		t.Fatalf("released attachments = %+v", released)
	}
	if _, err := repo.GetMessageAttachment(ctx, attachment.ID); !errors.Is(err, models.ErrAttachmentNotFound) {
		t.Fatalf("attachment remains after destination cleanup: %v", err)
	}
}
