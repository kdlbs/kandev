package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	taskservice "github.com/kandev/kandev/internal/task/service"
	_ "github.com/mattn/go-sqlite3"
)

type workflowAttachmentTransferStub struct {
	calls []struct {
		taskID, oldSessionID, newSessionID string
	}
	started chan struct{}
	release chan struct{}
}

func (s *workflowAttachmentTransferStub) TransferSessionMessageAttachments(
	_ context.Context,
	taskID, oldSessionID, newSessionID string,
	_ []string,
) error {
	s.calls = append(s.calls, struct {
		taskID, oldSessionID, newSessionID string
	}{taskID, oldSessionID, newSessionID})
	if s.started != nil {
		close(s.started)
		<-s.release
	}
	return nil
}

func TestTransferQueuedSessionStateRebindsAttachments(t *testing.T) {
	ctx := context.Background()
	transfer := &workflowAttachmentTransferStub{}
	svc := &Service{
		logger:                      testLogger(),
		messageQueue:                messagequeue.NewServiceMemory(testLogger()),
		sessionAttachmentTransferer: transfer,
	}
	queued, err := svc.messageQueue.QueueMessage(ctx, "session-old", "task-transfer", "handoff", "", messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{{AttachmentID: "attachment"}})
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.transferQueuedSessionState(ctx, "task-transfer", "session-old", "session-new"); err != nil {
		t.Fatal(err)
	}
	if len(transfer.calls) != 1 || transfer.calls[0].taskID != "task-transfer" || transfer.calls[0].oldSessionID != "session-old" || transfer.calls[0].newSessionID != "session-new" {
		t.Fatalf("attachment transfer calls = %+v", transfer.calls)
	}
	moved, ok := svc.messageQueue.TakeQueued(ctx, "session-new")
	if !ok || moved.ID != queued.ID {
		t.Fatalf("moved queue entry = %#v, ok=%t", moved, ok)
	}
}
func TestTransferQueuedSessionStateFailsClosedWithoutAttachmentTransferer(t *testing.T) {
	ctx := context.Background()
	queue := messagequeue.NewServiceMemory(testLogger())
	svc := &Service{logger: testLogger(), messageQueue: queue}
	if _, err := queue.QueueMessage(
		ctx, "session-old", "task-transfer", "handoff", "", messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{{AttachmentID: "attachment"}},
	); err != nil {
		t.Fatal(err)
	}

	err := svc.transferQueuedSessionState(ctx, "task-transfer", "session-old", "session-new")
	if !errors.Is(err, errSessionAttachmentTransferUnavailable) {
		t.Fatalf("transfer error = %v, want attachment transfer service unavailable", err)
	}
	if _, ok := queue.TakeQueued(ctx, "session-new"); ok {
		t.Fatal("queue entry moved despite unavailable attachment transfer service")
	}
	if _, ok := queue.TakeQueued(ctx, "session-old"); !ok {
		t.Fatal("queue entry missing after failed transfer")
	}
}
func TestTransferQueuedSessionStateFailsClosedWhenTaskAttachmentServiceUnavailable(t *testing.T) {
	ctx := context.Background()
	queue := messagequeue.NewServiceMemory(testLogger())
	svc := &Service{
		logger: testLogger(), messageQueue: queue,
		sessionAttachmentTransferer: &taskservice.Service{},
	}
	if _, err := queue.QueueMessage(
		ctx, "session-old", "task-transfer", "handoff", "", messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{{AttachmentID: "attachment"}},
	); err != nil {
		t.Fatal(err)
	}

	if err := svc.transferQueuedSessionState(ctx, "task-transfer", "session-old", "session-new"); err == nil {
		t.Fatal("transfer unexpectedly succeeded without task attachment service")
	}
	if _, ok := queue.TakeQueued(ctx, "session-new"); ok {
		t.Fatal("queue entry moved despite unavailable task attachment service")
	}
	if _, ok := queue.TakeQueued(ctx, "session-old"); !ok {
		t.Fatal("queue entry missing after failed transfer")
	}
}

func TestTransferQueuedSessionStateTreatsTypedNilAttachmentTransfererAsUnavailable(t *testing.T) {
	ctx := context.Background()
	queue := messagequeue.NewServiceMemory(testLogger())
	var transfer *taskservice.Service
	svc := &Service{
		logger: testLogger(), messageQueue: queue,
		sessionAttachmentTransferer: transfer,
	}
	if _, err := queue.QueueMessage(
		ctx, "session-old", "task-transfer", "handoff", "", messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{{AttachmentID: "attachment"}},
	); err != nil {
		t.Fatal(err)
	}

	err := svc.transferQueuedSessionState(ctx, "task-transfer", "session-old", "session-new")
	if !errors.Is(err, errSessionAttachmentTransferUnavailable) {
		t.Fatalf("transfer error = %v, want attachment transfer service unavailable", err)
	}
	if _, ok := queue.TakeQueued(ctx, "session-new"); ok {
		t.Fatal("queue entry moved despite typed nil attachment transfer service")
	}
}

func TestSessionTransferRecoveryFailsClosedWhenTaskAttachmentServiceUnavailable(t *testing.T) {
	ctx := context.Background()
	queue, db := newWorkflowTransferQueue(t, filepath.Join(t.TempDir(), "queue.db"))
	t.Cleanup(func() { _ = db.Close() })
	entry, err := queue.QueueMessage(
		ctx, "session-new", "task-transfer", "handoff", "", messagequeue.QueuedByUser, false, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.UpsertSessionTransferCompensation(ctx, messagequeue.SessionTransferCompensation{
		OperationID:   "unavailable-attachment-service",
		TaskID:        entry.TaskID,
		FromSessionID: "session-old",
		ToSessionID:   entry.SessionID,
		EntryIDs:      []string{entry.ID},
		AttachmentIDs: []string{"attachment"},
	}); err != nil {
		t.Fatal(err)
	}
	expireSessionTransferCompensationLease(t, db)
	svc := &Service{
		logger: testLogger(), messageQueue: queue,
		sessionAttachmentTransferer: &taskservice.Service{},
	}

	if err := svc.reconcileSessionTransferCompensationsOnStartup(ctx); err == nil {
		t.Fatal("recovery unexpectedly succeeded without task attachment service")
	}
	compensations, err := queue.ListSessionTransferCompensations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(compensations) != 1 {
		t.Fatalf("remaining compensations = %#v, want one", compensations)
	}
}
func TestSessionTransferCompensationRecoveryAllowsTextOnlyWithoutAttachmentTransferer(t *testing.T) {
	ctx := context.Background()
	queue, db := newWorkflowTransferQueue(t, filepath.Join(t.TempDir(), "queue.db"))
	t.Cleanup(func() { _ = db.Close() })
	entry, err := queue.QueueMessage(
		ctx, "session-new", "task-transfer", "handoff", "", messagequeue.QueuedByUser, false, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.UpsertSessionTransferCompensation(ctx, messagequeue.SessionTransferCompensation{
		OperationID:   "text-only-transfer",
		TaskID:        entry.TaskID,
		FromSessionID: "session-old",
		ToSessionID:   entry.SessionID,
		EntryIDs:      []string{entry.ID},
	}); err != nil {
		t.Fatal(err)
	}
	expireSessionTransferCompensationLease(t, db)
	svc := &Service{logger: testLogger(), messageQueue: queue}

	if err := svc.reconcileSessionTransferCompensationsOnStartup(ctx); err != nil {
		t.Fatal(err)
	}
	compensations, err := queue.ListSessionTransferCompensations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(compensations) != 0 {
		t.Fatalf("remaining compensations = %#v, want none", compensations)
	}
}

func TestTransferQueuedSessionStateSerializesAttachmentTransferWithQueueMutation(t *testing.T) {
	ctx := context.Background()
	transfer := &workflowAttachmentTransferStub{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	svc := &Service{
		logger:                      testLogger(),
		messageQueue:                messagequeue.NewServiceMemory(testLogger()),
		sessionAttachmentTransferer: transfer,
	}
	queued, err := svc.messageQueue.QueueMessage(ctx, "session-old", "task-transfer", "handoff", "", messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{{AttachmentID: "attachment"}})
	if err != nil {
		t.Fatal(err)
	}

	transferDone := make(chan error, 1)
	go func() {
		transferDone <- svc.transferQueuedSessionState(ctx, "task-transfer", "session-old", "session-new")
	}()
	select {
	case <-transfer.started:
	case <-time.After(time.Second):
		t.Fatal("attachment transfer did not start")
	}

	mutationDone := make(chan error, 1)
	go func() {
		mutationDone <- svc.messageQueue.UpdateMessageWithMetadata(
			ctx, "session-old", queued.ID, "changed", nil, nil, messagequeue.QueuedByUser,
		)
	}()
	select {
	case err := <-mutationDone:
		t.Fatalf("queue mutation completed during attachment transfer: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(transfer.release)

	if err := <-transferDone; err != nil {
		t.Fatal(err)
	}
	if err := <-mutationDone; err == nil {
		t.Fatal("queue mutation unexpectedly succeeded after transfer")
	}
}

func TestTransferQueuedSessionStateIncludesReservedAttachmentRows(t *testing.T) {
	ctx := context.Background()
	transfer := &workflowAttachmentTransferStub{}
	queue := messagequeue.NewServiceMemory(testLogger())
	svc := &Service{
		logger: testLogger(), messageQueue: queue, sessionAttachmentTransferer: transfer,
	}
	queued, err := queue.QueueMessageWithMetadata(
		ctx,
		"session-old",
		"task-transfer",
		"reserved handoff",
		"",
		messagequeue.QueuedByAgent,
		false,
		[]messagequeue.MessageAttachment{{AttachmentID: "attachment"}},
		map[string]interface{}{messagequeue.MetadataLifecycleDurable: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queue.ClaimSendNow(ctx, "session-old", []messagequeue.QueuedMessage{*queued}); err != nil {
		t.Fatal(err)
	}
	if queue.GetStatus(ctx, "session-old").Count != 0 {
		t.Fatal("reserved lifecycle row remained visible")
	}

	if err := svc.transferQueuedSessionState(ctx, "task-transfer", "session-old", "session-new"); err != nil {
		t.Fatal(err)
	}
	if len(transfer.calls) != 1 {
		t.Fatalf("attachment transfer calls = %d, want 1", len(transfer.calls))
	}
}

type statefulWorkflowAttachmentTransfer struct {
	currentSession string
	rollbackErr    error
	calls          int
}

func (s *statefulWorkflowAttachmentTransfer) TransferSessionMessageAttachments(
	_ context.Context,
	_ string,
	oldSessionID, newSessionID string,
	_ []string,
) error {
	s.calls++
	if s.currentSession != oldSessionID {
		return errors.New("attachment session mismatch")
	}
	if s.calls > 1 && s.rollbackErr != nil {
		return s.rollbackErr
	}
	s.currentSession = newSessionID
	return nil
}

func TestTransferQueuedSessionStateRecoversFailedAttachmentCompensationAfterRestart(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "queue.db")
	queue, db := newWorkflowTransferQueue(t, dbPath)
	_, err := queue.QueueMessage(ctx, "session-old", "task-transfer", "handoff", "", messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{{AttachmentID: "attachment"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TRIGGER fail_workflow_queue_transfer
		BEFORE UPDATE OF session_id ON queued_messages
		BEGIN
			SELECT RAISE(ABORT, 'forced queue transfer failure');
		END
	`); err != nil {
		t.Fatal(err)
	}
	transfer := &statefulWorkflowAttachmentTransfer{
		currentSession: "session-old",
		rollbackErr:    errors.New("forced attachment rollback failure"),
	}
	svc := &Service{logger: testLogger(), messageQueue: queue, sessionAttachmentTransferer: transfer}

	if err := svc.transferQueuedSessionState(ctx, "task-transfer", "session-old", "session-new"); err == nil {
		t.Fatal("transfer unexpectedly succeeded")
	}
	if transfer.currentSession != "session-new" {
		t.Fatalf("attachment session after failed compensation = %q, want session-new before recovery", transfer.currentSession)
	}
	expireSessionTransferCompensationLease(t, db)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	restartedQueue, restartedDB := newWorkflowTransferQueue(t, dbPath)
	t.Cleanup(func() { _ = restartedDB.Close() })
	transfer.rollbackErr = nil
	restarted := &Service{
		logger: testLogger(), messageQueue: restartedQueue, sessionAttachmentTransferer: transfer,
	}
	reconciler, ok := any(restarted).(interface {
		reconcileSessionTransferCompensationsOnStartup(context.Context) error
	})
	if !ok {
		t.Fatal("orchestrator has no durable session-transfer compensation reconciler")
	}
	if err := reconciler.reconcileSessionTransferCompensationsOnStartup(ctx); err != nil {
		t.Fatal(err)
	}
	if transfer.currentSession != "session-old" {
		t.Fatalf("attachment session after recovery = %q, want session-old", transfer.currentSession)
	}
}

type attachmentSessionSetTransfer struct {
	sessions map[string]string
}

func (s *attachmentSessionSetTransfer) TransferSessionMessageAttachments(
	_ context.Context,
	_ string,
	oldSessionID, newSessionID string,
	attachmentIDs []string,
) error {
	for _, attachmentID := range attachmentIDs {
		if s.sessions[attachmentID] == oldSessionID {
			s.sessions[attachmentID] = newSessionID
		}
	}
	return nil
}

func TestTransferQueuedSessionRollbackPreservesDestinationAttachmentClaims(t *testing.T) {
	ctx := context.Background()
	queue, db := newWorkflowTransferQueue(t, filepath.Join(t.TempDir(), "queue.db"))
	t.Cleanup(func() { _ = db.Close() })
	_, err := queue.QueueMessage(
		ctx, "session-old", "task-transfer", "source", "", messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{{AttachmentID: "source-attachment"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = queue.QueueMessage(
		ctx, "session-new", "task-transfer", "destination", "", messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{{AttachmentID: "destination-attachment"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TRIGGER fail_scoped_workflow_queue_transfer
		BEFORE UPDATE OF session_id ON queued_messages
		BEGIN
			SELECT RAISE(ABORT, 'forced queue transfer failure');
		END
	`); err != nil {
		t.Fatal(err)
	}
	transfer := &attachmentSessionSetTransfer{sessions: map[string]string{
		"source-attachment":      "session-old",
		"destination-attachment": "session-new",
	}}
	svc := &Service{logger: testLogger(), messageQueue: queue, sessionAttachmentTransferer: transfer}

	if err := svc.transferQueuedSessionState(ctx, "task-transfer", "session-old", "session-new"); err == nil {
		t.Fatal("transfer unexpectedly succeeded")
	}
	if got := transfer.sessions["destination-attachment"]; got != "session-new" {
		t.Fatalf("pre-existing destination attachment session = %q, want session-new", got)
	}
}
func TestSessionTransferCompensationRecoveryAcceptsDestinationAttachment(t *testing.T) {
	ctx := context.Background()
	queue, db := newWorkflowTransferQueue(t, filepath.Join(t.TempDir(), "queue.db"))
	t.Cleanup(func() { _ = db.Close() })
	entry, err := queue.QueueMessage(
		ctx, "session-new", "task-transfer", "handoff", "", messagequeue.QueuedByUser, false, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.UpsertSessionTransferCompensation(ctx, messagequeue.SessionTransferCompensation{
		OperationID:   "destination-transfer",
		TaskID:        entry.TaskID,
		FromSessionID: "session-old",
		ToSessionID:   entry.SessionID,
		EntryIDs:      []string{entry.ID},
		AttachmentIDs: []string{"destination-attachment"},
	}); err != nil {
		t.Fatal(err)
	}
	expireSessionTransferCompensationLease(t, db)
	transfer := &attachmentSessionSetTransfer{sessions: map[string]string{
		"destination-attachment": "session-new",
	}}
	restarted := &Service{
		logger: testLogger(), messageQueue: queue, sessionAttachmentTransferer: transfer,
	}
	if err := restarted.reconcileSessionTransferCompensationsOnStartup(ctx); err != nil {
		t.Fatal(err)
	}
	if got := transfer.sessions["destination-attachment"]; got != "session-new" {
		t.Fatalf("destination attachment session = %q, want session-new", got)
	}
	compensations, err := queue.ListSessionTransferCompensations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(compensations) != 0 {
		t.Fatalf("remaining compensations = %#v, want none", compensations)
	}
}

func TestCleanupOnlyTransferCompensationRecoversSourceAfterRestart(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "queue.db")
	queue, db := newWorkflowTransferQueue(t, dbPath)
	cleanup := messagequeue.AttachmentCleanup{
		SessionID: "session-old", EntryID: "cleanup-entry", OperationID: "cleanup-operation",
		TaskID: "task-transfer", Attachments: []messagequeue.MessageAttachment{{AttachmentID: "attachment"}},
	}
	if err := queue.UpsertAttachmentCleanup(ctx, cleanup); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TRIGGER fail_cleanup_only_queue_transfer
		BEFORE UPDATE OF current_session_id ON queue_attachment_cleanups
		BEGIN
			SELECT RAISE(ABORT, 'forced cleanup transfer failure');
		END
	`); err != nil {
		t.Fatal(err)
	}
	transfer := &statefulWorkflowAttachmentTransfer{
		currentSession: "session-old",
		rollbackErr:    errors.New("forced attachment rollback failure"),
	}
	svc := &Service{logger: testLogger(), messageQueue: queue, sessionAttachmentTransferer: transfer}
	if err := svc.transferQueuedSessionState(ctx, cleanup.TaskID, cleanup.SessionID, "session-new"); err == nil {
		t.Fatal("transfer unexpectedly succeeded")
	}
	if transfer.currentSession != "session-new" {
		t.Fatalf("attachment session before restart = %q, want session-new", transfer.currentSession)
	}
	expireSessionTransferCompensationLease(t, db)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	restartedQueue, restartedDB := newWorkflowTransferQueue(t, dbPath)
	t.Cleanup(func() { _ = restartedDB.Close() })
	transfer.rollbackErr = nil
	restarted := &Service{
		logger: testLogger(), messageQueue: restartedQueue, sessionAttachmentTransferer: transfer,
	}
	if err := restarted.reconcileSessionTransferCompensationsOnStartup(ctx); err != nil {
		t.Fatal(err)
	}
	if transfer.currentSession != "session-old" {
		t.Fatalf("attachment session after recovery = %q, want session-old", transfer.currentSession)
	}
}

func TestSessionTransferCompensationRecoveryRejectsActiveOwner(t *testing.T) {
	ctx := context.Background()
	queue, db := newWorkflowTransferQueue(t, filepath.Join(t.TempDir(), "queue.db"))
	t.Cleanup(func() { _ = db.Close() })
	entry, err := queue.QueueMessage(
		ctx, "session-old", "task-transfer", "handoff", "", messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{{AttachmentID: "attachment"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.UpsertSessionTransferCompensation(ctx, messagequeue.SessionTransferCompensation{
		OperationID:   "active-transfer",
		TaskID:        entry.TaskID,
		FromSessionID: entry.SessionID,
		ToSessionID:   "session-new",
		EntryIDs:      []string{entry.ID},
		AttachmentIDs: []string{"attachment"},
	}); err != nil {
		t.Fatal(err)
	}
	transfer := &workflowAttachmentTransferStub{}
	restarted := &Service{
		logger: testLogger(), messageQueue: queue, sessionAttachmentTransferer: transfer,
	}

	err = restarted.reconcileSessionTransferCompensationsOnStartup(ctx)

	if !errors.Is(err, messagequeue.ErrSessionTransferInProgress) {
		t.Fatalf("recovery error = %v, want active transfer rejection", err)
	}
	if len(transfer.calls) != 0 {
		t.Fatalf("attachment recovery calls = %#v, want none", transfer.calls)
	}
	compensations, listErr := queue.ListSessionTransferCompensations(ctx)
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(compensations) != 1 {
		t.Fatalf("remaining compensations = %#v, want active owner preserved", compensations)
	}
}

func TestDurableQueueStartupRecoveryStopsRetryWhenContextCancels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	queue, db := newWorkflowTransferQueue(t, filepath.Join(t.TempDir(), "queue.db"))
	t.Cleanup(func() { _ = db.Close() })
	entry, err := queue.QueueMessage(
		ctx, "session-old", "task-transfer", "handoff", "", messagequeue.QueuedByUser, false, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.UpsertSessionTransferCompensation(ctx, messagequeue.SessionTransferCompensation{
		OperationID: "active-transfer", TaskID: entry.TaskID, FromSessionID: entry.SessionID, ToSessionID: "session-new",
	}); err != nil {
		t.Fatal(err)
	}
	service := &Service{
		logger: testLogger(), messageQueue: queue, sessionAttachmentTransferer: &workflowAttachmentTransferStub{},
	}
	result := make(chan error, 1)
	go func() { result <- service.reconcileDurableQueueStateOnStartup(ctx) }()

	select {
	case err := <-result:
		t.Fatalf("startup recovery returned before cancellation: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("startup recovery error = %v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("startup recovery did not stop after cancellation")
	}
}

func TestCleanupOnlyTransferCompensationRecoversPreviousHopAfterRestart(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "queue.db")
	queue, db := newWorkflowTransferQueue(t, dbPath)
	cleanup := messagequeue.AttachmentCleanup{
		SessionID: "session-a", EntryID: "cleanup-entry", OperationID: "cleanup-operation",
		TaskID: "task-transfer", Attachments: []messagequeue.MessageAttachment{{AttachmentID: "attachment"}},
	}
	if err := queue.UpsertAttachmentCleanup(ctx, cleanup); err != nil {
		t.Fatal(err)
	}
	transfer := &statefulWorkflowAttachmentTransfer{currentSession: "session-a"}
	svc := &Service{logger: testLogger(), messageQueue: queue, sessionAttachmentTransferer: transfer}
	if err := svc.transferQueuedSessionState(ctx, cleanup.TaskID, "session-a", "session-b"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TRIGGER fail_multi_hop_cleanup_transfer
		BEFORE UPDATE OF current_session_id ON queue_attachment_cleanups
		BEGIN
			SELECT RAISE(ABORT, 'forced cleanup transfer failure');
		END
	`); err != nil {
		t.Fatal(err)
	}
	transfer.calls = 0
	transfer.rollbackErr = errors.New("forced attachment rollback failure")
	if err := svc.transferQueuedSessionState(ctx, cleanup.TaskID, "session-b", "session-c"); err == nil {
		t.Fatal("second transfer unexpectedly succeeded")
	}
	if transfer.currentSession != "session-c" {
		t.Fatalf("attachment session before restart = %q, want session-c", transfer.currentSession)
	}
	expireSessionTransferCompensationLease(t, db)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	restartedQueue, restartedDB := newWorkflowTransferQueue(t, dbPath)
	t.Cleanup(func() { _ = restartedDB.Close() })
	transfer.rollbackErr = nil
	restarted := &Service{
		logger: testLogger(), messageQueue: restartedQueue, sessionAttachmentTransferer: transfer,
	}
	if err := restarted.reconcileSessionTransferCompensationsOnStartup(ctx); err != nil {
		t.Fatal(err)
	}
	if transfer.currentSession != "session-b" {
		t.Fatalf("attachment session after recovery = %q, want session-b", transfer.currentSession)
	}
}

func TestDurableQueueStartupRecoveryReconcilesTransferBeforeDispatch(t *testing.T) {
	ctx := context.Background()
	queue, db := newWorkflowTransferQueue(t, filepath.Join(t.TempDir(), "queue.db"))
	t.Cleanup(func() { _ = db.Close() })
	entry, err := queue.QueueMessage(
		ctx, "session-old", "task-transfer", "handoff", "", messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{{AttachmentID: "attachment"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	reserved, ok := queue.ReserveQueued(ctx, entry.SessionID)
	if !ok || reserved.ID != entry.ID {
		t.Fatalf("reserved entry = %#v, ok=%t", reserved, ok)
	}
	if err := queue.UpsertSessionTransferCompensation(ctx, messagequeue.SessionTransferCompensation{
		OperationID:     "startup-transfer",
		TaskID:          entry.TaskID,
		FromSessionID:   entry.SessionID,
		ToSessionID:     "session-new",
		EntryIDs:        []string{entry.ID},
		AttachmentIDs:   []string{"attachment"},
		CleanupLocators: nil,
	}); err != nil {
		t.Fatal(err)
	}
	expireSessionTransferCompensationLease(t, db)
	transfer := &statefulWorkflowAttachmentTransfer{currentSession: "session-new"}
	restarted := &Service{
		logger: testLogger(), messageQueue: queue, sessionAttachmentTransferer: transfer,
	}

	if err := restarted.reconcileDurableQueueStateOnStartup(ctx); err != nil {
		t.Fatal(err)
	}

	if transfer.currentSession != entry.SessionID {
		t.Fatalf("attachment session after recovery = %q, want %q", transfer.currentSession, entry.SessionID)
	}
	status := queue.GetStatus(ctx, entry.SessionID)
	if len(status.Entries) != 1 || status.Entries[0].ID != entry.ID {
		t.Fatalf("recovered source queue = %#v", status.Entries)
	}
	compensations, err := queue.ListSessionTransferCompensations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(compensations) != 0 {
		t.Fatalf("remaining compensations = %#v", compensations)
	}
}

func expireSessionTransferCompensationLease(t *testing.T, db *sqlx.DB) {
	t.Helper()
	if _, err := db.Exec(`
		UPDATE queue_session_transfer_compensations
		SET recovery_lease_expires_at = ?
	`, time.Now().UTC().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
}

func newWorkflowTransferQueue(t *testing.T, dbPath string) (*messagequeue.Service, *sqlx.DB) {
	t.Helper()
	raw, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}

	raw.SetMaxOpenConns(1)
	db := sqlx.NewDb(raw, "sqlite3")
	repo, err := messagequeue.NewSQLiteRepository(db, db)
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	return messagequeue.NewService(repo, messagequeue.DefaultMaxPerSession, testLogger()), db
}
