package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

func TestRecoverTaskSessionQueueCommitsReceiptAndReplaysExactSnapshot(t *testing.T) {
	repo, queueRepo := newSessionQueueRecoveryFixture(t)
	ctx := context.Background()
	first := &messagequeue.QueuedMessage{
		SessionID: "source", TaskID: "task-recovery", Content: "first exact body", QueuedBy: messagequeue.QueuedByWorkflow,
		Metadata: map[string]interface{}{messagequeue.MetadataLifecycleDurable: true},
	}
	if err := queueRepo.Insert(ctx, first, 10); err != nil {
		t.Fatalf("insert durable source head: %v", err)
	}
	second := insertSessionQueueRecoveryMessage(t, queueRepo, "second exact body")
	if _, err := queueRepo.ReserveHead(ctx, "source"); err != nil {
		t.Fatalf("reserve durable source head: %v", err)
	}
	destinationHead := &messagequeue.QueuedMessage{
		SessionID: "destination", TaskID: "task-recovery", Content: "existing destination body", QueuedBy: messagequeue.QueuedByUser,
	}
	if err := queueRepo.Insert(ctx, destinationHead, 10); err != nil {
		t.Fatalf("insert destination head: %v", err)
	}

	result, err := repo.RecoverTaskSessionQueue(ctx, sessionQueueRecoveryScope())
	if err != nil {
		t.Fatalf("RecoverTaskSessionQueue: %v", err)
	}
	assertSessionQueueRecoveryResult(t, result, first, second)
	destination, err := queueRepo.ListBySession(ctx, "destination")
	if err != nil {
		t.Fatalf("list recovered destination: %v", err)
	}
	if len(destination) != 3 || destination[0].ID != destinationHead.ID ||
		destination[1].ID != first.ID || destination[2].ID != second.ID {
		t.Fatalf("destination FIFO = %#v, want existing head followed by recovered source order", destination)
	}
	if destination[1].IsReservedInFlight() || !result.Entries[0].IsReservedInFlight() {
		t.Fatalf("reservation restoration = destination %#v, receipt %#v", destination[1], result.Entries[0])
	}

	retry, err := repo.RecoverTaskSessionQueue(ctx, sessionQueueRecoveryScope())
	if err != nil {
		t.Fatalf("RecoverTaskSessionQueue retry: %v", err)
	}
	assertSessionQueueRecoveryResult(t, retry, first, second)
	if retry.Receipt.ID != result.Receipt.ID || retry.Receipt.SnapshotSHA256 != result.Receipt.SnapshotSHA256 {
		t.Fatalf("retry receipt = %#v, want stable identity/hash from %#v", retry.Receipt, result.Receipt)
	}
	late := &messagequeue.QueuedMessage{SessionID: "source", TaskID: "task-recovery", Content: "late source admission", QueuedBy: messagequeue.QueuedByUser}
	if err := queueRepo.Insert(ctx, late, 10); !errors.Is(err, messagequeue.ErrQueueRecoveryConflict) {
		t.Fatalf("late source admission error = %v, want recovery fence", err)
	}
}

func TestRecoverTaskSessionQueueRejectsStalePrimaryWithoutMutation(t *testing.T) {
	repo, queueRepo := newSessionQueueRecoveryFixture(t)
	ctx := context.Background()
	queued := insertSessionQueueRecoveryMessage(t, queueRepo, "must remain")
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "replacement", TaskID: "task-recovery", State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession replacement: %v", err)
	}
	if err := repo.SetSessionPrimary(ctx, "replacement"); err != nil {
		t.Fatalf("SetSessionPrimary replacement: %v", err)
	}

	_, err := repo.RecoverTaskSessionQueue(ctx, sessionQueueRecoveryScope())
	if !errors.Is(err, messagequeue.ErrQueueRecoveryUnauthorized) {
		t.Fatalf("RecoverTaskSessionQueue error = %v, want ErrQueueRecoveryUnauthorized", err)
	}
	assertQueueEntryRemainsAtSource(t, queueRepo, queued.ID)
}

func TestRecoverTaskSessionQueueRejectsResumedTargetWithoutMutation(t *testing.T) {
	repo, queueRepo := newSessionQueueRecoveryFixture(t)
	ctx := context.Background()
	queued := insertSessionQueueRecoveryMessage(t, queueRepo, "must remain")
	if err := repo.UpdateTaskSessionState(ctx, "source", models.TaskSessionStateRunning, ""); err != nil {
		t.Fatalf("resume source: %v", err)
	}

	_, err := repo.RecoverTaskSessionQueue(ctx, sessionQueueRecoveryScope())
	if !errors.Is(err, messagequeue.ErrQueueRecoveryTargetNotTerminal) {
		t.Fatalf("RecoverTaskSessionQueue error = %v, want ErrQueueRecoveryTargetNotTerminal", err)
	}
	assertQueueEntryRemainsAtSource(t, queueRepo, queued.ID)
}

func TestRecoverTaskSessionQueueRollsBackTransferWhenReceiptFails(t *testing.T) {
	repo, queueRepo := newSessionQueueRecoveryFixture(t)
	queued := insertSessionQueueRecoveryMessage(t, queueRepo, "must roll back")
	if _, err := repo.db.Exec(`
		CREATE TRIGGER fail_queue_recovery_receipt
		BEFORE INSERT ON queue_recovery_receipts
		BEGIN SELECT RAISE(ABORT, 'receipt failure'); END
	`); err != nil {
		t.Fatalf("create receipt failure trigger: %v", err)
	}

	if _, err := repo.RecoverTaskSessionQueue(context.Background(), sessionQueueRecoveryScope()); err == nil {
		t.Fatal("RecoverTaskSessionQueue error = nil, want receipt persistence failure")
	}
	assertQueueEntryRemainsAtSource(t, queueRepo, queued.ID)
	var receipts int
	if err := repo.db.Get(&receipts, `SELECT COUNT(*) FROM queue_recovery_receipts`); err != nil {
		t.Fatalf("count recovery receipts: %v", err)
	}
	if receipts != 0 {
		t.Fatalf("recovery receipt count = %d, want 0", receipts)
	}
}

func TestRecoverTaskSessionQueueRejectsPendingLifecycleMove(t *testing.T) {
	repo, queueRepo := newSessionQueueRecoveryFixture(t)
	ctx := context.Background()
	queued := insertSessionQueueRecoveryMessage(t, queueRepo, "must remain")
	if err := queueRepo.SetPendingMove(ctx, "source", &messagequeue.PendingMove{TaskID: "task-recovery"}); err != nil {
		t.Fatalf("SetPendingMove: %v", err)
	}

	if _, err := repo.RecoverTaskSessionQueue(ctx, sessionQueueRecoveryScope()); err == nil {
		t.Fatal("recovery moved queued messages while a pending lifecycle move remained")
	}
	assertQueueEntryRemainsAtSource(t, queueRepo, queued.ID)
	move, err := queueRepo.GetPendingMove(ctx, "source")
	if err != nil || move == nil {
		t.Fatalf("pending move after rejected recovery = %#v, err=%v", move, err)
	}
}

func TestRecoverTaskSessionQueueRejectsMismatchedEntryTask(t *testing.T) {
	repo, queueRepo := newSessionQueueRecoveryFixture(t)
	ctx := context.Background()
	queued := insertSessionQueueRecoveryMessage(t, queueRepo, "belongs elsewhere")
	if _, err := repo.db.ExecContext(ctx, `UPDATE queued_messages SET task_id = ? WHERE id = ?`, "other-task", queued.ID); err != nil {
		t.Fatalf("corrupt queue row task owner: %v", err)
	}

	if _, err := repo.RecoverTaskSessionQueue(ctx, sessionQueueRecoveryScope()); !errors.Is(err, messagequeue.ErrQueueRecoveryUnauthorized) {
		t.Fatalf("RecoverTaskSessionQueue error = %v, want cross-task denial", err)
	}
	assertQueueEntryRemainsAtSource(t, queueRepo, queued.ID)
}

func TestRecoverTaskSessionQueueRedactsReceiptAfterDisposition(t *testing.T) {
	repo, queueRepo := newSessionQueueRecoveryFixture(t)
	ctx := context.Background()
	insertSessionQueueRecoveryMessage(t, queueRepo, "temporary body")
	result, err := repo.RecoverTaskSessionQueue(ctx, sessionQueueRecoveryScope())
	if err != nil {
		t.Fatalf("RecoverTaskSessionQueue: %v", err)
	}
	if removed, err := queueRepo.DeleteAllBySession(ctx, "destination"); err != nil || removed != 1 {
		t.Fatalf("DeleteAllBySession removed=%d err=%v", removed, err)
	}
	if _, err := repo.RecoverTaskSessionQueue(ctx, sessionQueueRecoveryScope()); !errors.Is(err, messagequeue.ErrQueueRecoverySnapshotExpired) {
		t.Fatalf("replay after disposition error = %v, want expired snapshot", err)
	}
	var snapshot string
	if err := repo.db.GetContext(ctx, &snapshot, `SELECT snapshot_json FROM queue_recovery_receipts WHERE id = ?`, result.Receipt.ID); err != nil {
		t.Fatalf("read redacted receipt: %v", err)
	}
	if snapshot != "[]" {
		t.Fatalf("retained receipt body = %q, want redacted tombstone", snapshot)
	}
	var auditCount int
	if err := repo.db.GetContext(ctx, &auditCount, `SELECT COUNT(*) FROM queue_recovery_cleanup_events WHERE receipt_id = ?`, result.Receipt.ID); err != nil || auditCount != 1 {
		t.Fatalf("receipt cleanup audit count=%d err=%v", auditCount, err)
	}
}

func TestRecoverTaskSessionQueueRedactsReceiptAfterExactDisposition(t *testing.T) {
	repo, queueRepo := newSessionQueueRecoveryFixture(t)
	ctx := context.Background()
	insertSessionQueueRecoveryMessage(t, queueRepo, "temporary exact body")
	if _, err := repo.RecoverTaskSessionQueue(ctx, sessionQueueRecoveryScope()); err != nil {
		t.Fatalf("RecoverTaskSessionQueue: %v", err)
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	queue := messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, log)
	census, err := queue.Census(ctx, "destination")
	if err != nil || len(census.Entries) != 1 {
		t.Fatalf("destination census = %#v, err=%v", census, err)
	}
	_, err = queue.DisposeExact(ctx, "destination", []messagequeue.QueueEntryClaim{{
		ID: census.Entries[0].ID, Claim: census.Entries[0].Claim,
	}})
	if err != nil {
		t.Fatalf("DisposeExact: %v", err)
	}
	if _, err := repo.RecoverTaskSessionQueue(ctx, sessionQueueRecoveryScope()); !errors.Is(err, messagequeue.ErrQueueRecoverySnapshotExpired) {
		t.Fatalf("replay after exact disposition error = %v, want expired snapshot", err)
	}
}

func TestRecoverTaskSessionQueueRedactsSourceLineageAfterFinalDisposition(t *testing.T) {
	repo, queueRepo := newSessionQueueRecoveryFixture(t)
	ctx := context.Background()
	entry := insertSessionQueueRecoveryMessage(t, queueRepo, "body from source A")
	first, err := repo.RecoverTaskSessionQueue(ctx, sessionQueueRecoveryScope())
	if err != nil {
		t.Fatalf("recover A to B: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "final", TaskID: "task-recovery", State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("create final session: %v", err)
	}
	if err := repo.SetSessionPrimary(ctx, "final"); err != nil {
		t.Fatalf("promote final session: %v", err)
	}
	if err := repo.UpdateTaskSessionState(ctx, "destination", models.TaskSessionStateCompleted, ""); err != nil {
		t.Fatalf("complete intermediate session: %v", err)
	}
	unrelated := &messagequeue.QueuedMessage{
		SessionID: "final", TaskID: "task-recovery", Content: "unrelated pending dispatch", QueuedBy: messagequeue.QueuedByUser,
	}
	if err := queueRepo.Insert(ctx, unrelated, 10); err != nil {
		t.Fatalf("insert unrelated final entry: %v", err)
	}
	second, err := repo.RecoverTaskSessionQueue(ctx, messagequeue.QueueRecoveryScope{
		TaskID: "task-recovery", WorkspaceID: "workspace-recovery",
		SourceSessionID: "destination", DestinationSessionID: "final",
	})
	if err != nil {
		t.Fatalf("recover B to C: %v", err)
	}
	if claimed, err := queueRepo.TakeHead(ctx, "final"); err != nil || claimed == nil || claimed.ID != unrelated.ID {
		t.Fatalf("claim unrelated final entry = %#v, err=%v", claimed, err)
	}
	unrelatedPending := &messagequeue.QueuedMessage{
		SessionID: "final", TaskID: "task-recovery", Content: "unrelated queued body", QueuedBy: messagequeue.QueuedByUser,
	}
	if err := queueRepo.Insert(ctx, unrelatedPending, 10); err != nil {
		t.Fatalf("insert unrelated pending entry: %v", err)
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	queue := messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, log)
	census, err := queue.Census(ctx, "final")
	if err != nil || len(census.Entries) != 2 || census.Entries[0].ID != entry.ID {
		t.Fatalf("final census = %#v, err=%v", census, err)
	}
	if _, err := queue.DisposeExact(ctx, "final", []messagequeue.QueueEntryClaim{{
		ID: entry.ID, Claim: census.Entries[0].Claim,
	}}); err != nil {
		t.Fatalf("dispose original entry at C: %v", err)
	}
	for _, receiptID := range []string{first.Receipt.ID, second.Receipt.ID} {
		var snapshot string
		if err := repo.db.GetContext(ctx, &snapshot, `SELECT snapshot_json FROM queue_recovery_receipts WHERE id = ?`, receiptID); err != nil {
			t.Fatalf("read receipt %s: %v", receiptID, err)
		}
		if snapshot != "[]" {
			t.Fatalf("receipt %s retained original body after final disposition: %q", receiptID, snapshot)
		}
	}
	remaining, err := queueRepo.ListBySession(ctx, "final")
	if err != nil || len(remaining) != 1 || remaining[0].ID != unrelatedPending.ID {
		t.Fatalf("unrelated queued work changed = %#v, err=%v", remaining, err)
	}
}

func TestRecoverTaskSessionQueueRedactsReceiptAfterDeliveryAcknowledgement(t *testing.T) {
	repo, queueRepo := newSessionQueueRecoveryFixture(t)
	ctx := context.Background()
	entry := &messagequeue.QueuedMessage{
		SessionID: "source", TaskID: "task-recovery", Content: "accepted body",
		QueuedBy: messagequeue.QueuedByWorkflow,
		Metadata: map[string]interface{}{messagequeue.MetadataLifecycleDurable: true},
	}
	if err := queueRepo.Insert(ctx, entry, 10); err != nil {
		t.Fatalf("insert durable message: %v", err)
	}
	if _, err := repo.RecoverTaskSessionQueue(ctx, sessionQueueRecoveryScope()); err != nil {
		t.Fatalf("RecoverTaskSessionQueue: %v", err)
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	queue := messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, log)
	reserved, ok := queue.ReserveQueued(ctx, "destination")
	if !ok || reserved == nil {
		t.Fatalf("ReserveQueued = %#v, ok=%t", reserved, ok)
	}
	if err := queueRepo.AcknowledgeReserved(ctx, reserved); err != nil {
		t.Fatalf("AcknowledgeReserved: %v", err)
	}
	if _, err := repo.RecoverTaskSessionQueue(ctx, sessionQueueRecoveryScope()); !errors.Is(err, messagequeue.ErrQueueRecoverySnapshotExpired) {
		t.Fatalf("replay after acknowledgement error = %v, want expired snapshot", err)
	}
}

func TestRecoverTaskSessionQueueRedactsReceiptAfterSendNowAcknowledgement(t *testing.T) {
	repo, queueRepo := newSessionQueueRecoveryFixture(t)
	ctx := context.Background()
	insertSessionQueueRecoveryMessage(t, queueRepo, "send now body")
	if _, err := repo.RecoverTaskSessionQueue(ctx, sessionQueueRecoveryScope()); err != nil {
		t.Fatalf("RecoverTaskSessionQueue: %v", err)
	}
	destination, err := queueRepo.ListBySession(ctx, "destination")
	if err != nil || len(destination) != 1 {
		t.Fatalf("destination queue = %#v, err=%v", destination, err)
	}
	claim, err := queueRepo.ClaimSendNow(ctx, "destination", destination)
	if err != nil {
		t.Fatalf("ClaimSendNow: %v", err)
	}
	if err := queueRepo.AcknowledgeSendNowClaim(ctx, claim); err != nil {
		t.Fatalf("AcknowledgeSendNowClaim: %v", err)
	}
	if _, err := repo.RecoverTaskSessionQueue(ctx, sessionQueueRecoveryScope()); !errors.Is(err, messagequeue.ErrQueueRecoverySnapshotExpired) {
		t.Fatalf("replay after Send Now acknowledgement error = %v, want expired snapshot", err)
	}
}

func TestRecoverTaskSessionQueueRedactsReceiptAfterDispatchAcknowledgement(t *testing.T) {
	repo, queueRepo := newSessionQueueRecoveryFixture(t)
	ctx := context.Background()
	insertSessionQueueRecoveryMessage(t, queueRepo, "dispatched body")
	if _, err := repo.RecoverTaskSessionQueue(ctx, sessionQueueRecoveryScope()); err != nil {
		t.Fatalf("RecoverTaskSessionQueue: %v", err)
	}
	dispatch, err := queueRepo.TakeHead(ctx, "destination")
	if err != nil || dispatch == nil {
		t.Fatalf("TakeHead = %#v, err=%v", dispatch, err)
	}
	claims := queueRepo.(interface {
		MarkPendingQueueDispatchAccepted(context.Context, *messagequeue.QueuedMessage) error
		DeletePendingQueueDispatch(context.Context, *messagequeue.QueuedMessage) error
	})
	if err := claims.MarkPendingQueueDispatchAccepted(ctx, dispatch); err != nil {
		t.Fatalf("MarkPendingQueueDispatchAccepted: %v", err)
	}
	if err := claims.DeletePendingQueueDispatch(ctx, dispatch); err != nil {
		t.Fatalf("DeletePendingQueueDispatch: %v", err)
	}
	if _, err := repo.RecoverTaskSessionQueue(ctx, sessionQueueRecoveryScope()); !errors.Is(err, messagequeue.ErrQueueRecoverySnapshotExpired) {
		t.Fatalf("replay after dispatch acknowledgement error = %v, want expired snapshot", err)
	}
}

func TestRecoverTaskSessionQueueRedactsReceiptOnTaskArchive(t *testing.T) {
	repo, queueRepo := newSessionQueueRecoveryFixture(t)
	ctx := context.Background()
	insertSessionQueueRecoveryMessage(t, queueRepo, "archive body")
	result, err := repo.RecoverTaskSessionQueue(ctx, sessionQueueRecoveryScope())
	if err != nil {
		t.Fatalf("RecoverTaskSessionQueue: %v", err)
	}
	if err := repo.ArchiveTask(ctx, "task-recovery"); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}
	var snapshot string
	if err := repo.db.GetContext(ctx, &snapshot, `SELECT snapshot_json FROM queue_recovery_receipts WHERE id = ?`, result.Receipt.ID); err != nil {
		t.Fatalf("read archived recovery receipt: %v", err)
	}
	if snapshot != "[]" {
		t.Fatalf("task archive retained recovery body: %q", snapshot)
	}
	var auditCount int
	if err := repo.db.GetContext(ctx, &auditCount, `SELECT COUNT(*) FROM queue_recovery_cleanup_events WHERE receipt_id = ? AND reason = 'task_purged'`, result.Receipt.ID); err != nil || auditCount != 1 {
		t.Fatalf("task archive cleanup audit count=%d err=%v", auditCount, err)
	}
}

func newSessionQueueRecoveryFixture(t *testing.T) (*Repository, messagequeue.Repository) {
	t.Helper()
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-recovery", WorkspaceID: "workspace-recovery", Title: "Queue recovery",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	for _, session := range []*models.TaskSession{
		{ID: "destination", TaskID: "task-recovery", IsPrimary: true, State: models.TaskSessionStateWaitingForInput},
		{ID: "source", TaskID: "task-recovery", State: models.TaskSessionStateCompleted},
	} {
		if err := repo.CreateTaskSession(ctx, session); err != nil {
			t.Fatalf("CreateTaskSession(%s): %v", session.ID, err)
		}
	}
	queueRepo, err := messagequeue.NewSQLiteRepository(repo.db, repo.ro)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	return repo, queueRepo
}

func sessionQueueRecoveryScope() messagequeue.QueueRecoveryScope {
	return messagequeue.QueueRecoveryScope{
		TaskID: "task-recovery", WorkspaceID: "workspace-recovery",
		SourceSessionID: "source", DestinationSessionID: "destination",
	}
}

func insertSessionQueueRecoveryMessage(
	t *testing.T,
	queueRepo messagequeue.Repository,
	content string,
) *messagequeue.QueuedMessage {
	t.Helper()
	entry := &messagequeue.QueuedMessage{
		SessionID: "source", TaskID: "task-recovery", Content: content, QueuedBy: messagequeue.QueuedByUser,
	}
	if err := queueRepo.Insert(context.Background(), entry, 10); err != nil {
		t.Fatalf("insert queue message: %v", err)
	}
	return entry
}

func assertSessionQueueRecoveryResult(
	t *testing.T,
	result *messagequeue.QueueRecoveryResult,
	want ...*messagequeue.QueuedMessage,
) {
	t.Helper()
	if result == nil || result.Receipt.ID == "" || result.Receipt.SnapshotSHA256 == "" {
		t.Fatalf("recovery result = %#v, want durable receipt", result)
	}
	if result.Receipt.EntryCount != len(want) || len(result.Entries) != len(want) {
		t.Fatalf("recovery result counts = receipt %d entries %d, want %d", result.Receipt.EntryCount, len(result.Entries), len(want))
	}
	for index := range want {
		if result.Entries[index].ID != want[index].ID || result.Entries[index].Content != want[index].Content ||
			result.Entries[index].SessionID != "source" || result.Entries[index].Position != want[index].Position {
			t.Fatalf("recovery entry %d = %#v, want identity/body/FIFO from %#v", index, result.Entries[index], want[index])
		}
	}
}

func assertQueueEntryRemainsAtSource(t *testing.T, queueRepo messagequeue.Repository, wantID string) {
	t.Helper()
	source, err := queueRepo.ListBySession(context.Background(), "source")
	if err != nil {
		t.Fatalf("list source queue: %v", err)
	}
	if len(source) != 1 || source[0].ID != wantID {
		t.Fatalf("source queue = %#v, want unchanged entry %s", source, wantID)
	}
	destination, err := queueRepo.ListBySession(context.Background(), "destination")
	if err != nil {
		t.Fatalf("list destination queue: %v", err)
	}
	if len(destination) != 0 {
		t.Fatalf("destination queue = %#v, want empty", destination)
	}
}
