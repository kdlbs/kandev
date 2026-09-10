package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
)

func TestOrdinaryQueueDispatchRestoresAfterProcessRestart(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/queue.db"
	queue, db := newWorkflowTransferQueue(t, dbPath)
	source, err := queue.QueueMessage(
		ctx, "session-1", "task-1", "ordinary prompt", "", messagequeue.QueuedByUser, false, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	reserved, ok := queue.ReserveQueued(ctx, "session-1")
	if !ok || reserved.ID != source.ID {
		t.Fatalf("reserved = %#v, ok=%t", reserved, ok)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	restartedQueue, restartedDB := newWorkflowTransferQueue(t, dbPath)
	t.Cleanup(func() { _ = restartedDB.Close() })
	restarted := &Service{logger: testLogger(), messageQueue: restartedQueue}
	reconciler, ok := any(restarted).(interface {
		reconcilePendingQueueDispatchesOnStartup(context.Context) error
	})
	if !ok {
		t.Fatal("orchestrator cannot recover pending ordinary queue dispatches")
	}
	if err := reconciler.reconcilePendingQueueDispatchesOnStartup(ctx); err != nil {
		t.Fatal(err)
	}
	status := restartedQueue.GetStatus(ctx, "session-1")
	if len(status.Entries) != 1 || status.Entries[0].ID != source.ID {
		t.Fatalf("queue after ordinary dispatch recovery = %#v, want source %s", status.Entries, source.ID)
	}
}

func TestAcceptedOrdinaryQueueDispatchIsAcknowledgedAfterProcessRestart(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/queue.db"
	queue, db := newWorkflowTransferQueue(t, dbPath)
	source, err := queue.QueueMessage(
		ctx, "session-1", "task-1", "ordinary prompt", "", messagequeue.QueuedByUser, false, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	reserved, ok := queue.ReserveQueued(ctx, "session-1")
	if !ok || reserved.ID != source.ID {
		t.Fatalf("reserved = %#v, ok=%t", reserved, ok)
	}
	if err := queue.MarkPendingQueueDispatchAccepted(ctx, reserved); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	restartedQueue, restartedDB := newWorkflowTransferQueue(t, dbPath)
	t.Cleanup(func() { _ = restartedDB.Close() })
	restarted := &Service{logger: testLogger(), messageQueue: restartedQueue}
	if err := restarted.reconcilePendingQueueDispatchesOnStartup(ctx); err != nil {
		t.Fatal(err)
	}
	status := restartedQueue.GetStatus(ctx, "session-1")
	if len(status.Entries) != 0 {
		t.Fatalf("queue after accepted ordinary dispatch recovery = %#v, want empty", status.Entries)
	}
	pending, err := restartedQueue.ListPendingQueueDispatches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending dispatches = %#v, want empty", pending)
	}
}

func TestOrdinaryQueueDispatchCallbackPersistsAcceptance(t *testing.T) {
	ctx := context.Background()
	queue, db := newWorkflowTransferQueue(t, t.TempDir()+"/queue.db")
	t.Cleanup(func() { _ = db.Close() })
	source, err := queue.QueueMessage(
		ctx, "session-1", "task-1", "ordinary prompt", "", messagequeue.QueuedByUser, false, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	reserved, ok := queue.ReserveQueued(ctx, "session-1")
	if !ok || reserved.ID != source.ID {
		t.Fatalf("reserved = %#v, ok=%t", reserved, ok)
	}
	service := &Service{logger: testLogger(), messageQueue: queue}
	afterDispatch := service.queuedMessageAfterDispatch(ctx, reserved, false)
	if afterDispatch == nil {
		t.Fatal("ordinary queue dispatch has no durable acceptance callback")
	}
	if err := afterDispatch(); err != nil {
		t.Fatal(err)
	}
	pending, err := queue.ListPendingQueueDispatches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || !pending[0].Accepted || pending[0].Message.ID != source.ID {
		t.Fatalf("pending dispatches = %#v, want accepted %s", pending, source.ID)
	}
}

func TestClearedOrdinaryQueueDispatchIsNotRestored(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/queue.db"
	queue, db := newWorkflowTransferQueue(t, dbPath)
	source, err := queue.QueueMessage(
		ctx, "session-1", "task-1", "ordinary prompt", "", messagequeue.QueuedByUser, false, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	reserved, ok := queue.ReserveQueued(ctx, "session-1")
	if !ok || reserved.ID != source.ID {
		t.Fatalf("reserved = %#v, ok=%t", reserved, ok)
	}
	if _, err := queue.CancelAll(ctx, "session-1"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	restartedQueue, restartedDB := newWorkflowTransferQueue(t, dbPath)
	t.Cleanup(func() { _ = restartedDB.Close() })
	restarted := &Service{logger: testLogger(), messageQueue: restartedQueue}
	if err := restarted.reconcilePendingQueueDispatchesOnStartup(ctx); err != nil {
		t.Fatal(err)
	}
	if status := restartedQueue.GetStatus(ctx, "session-1"); len(status.Entries) != 0 {
		t.Fatalf("queue after cleared dispatch recovery = %#v, want empty", status.Entries)
	}
}

func TestAcceptedOrdinaryDispatchFailureAcknowledgesRecoveryClaim(t *testing.T) {
	ctx := context.Background()
	queue, db := newWorkflowTransferQueue(t, t.TempDir()+"/queue.db")
	t.Cleanup(func() { _ = db.Close() })
	source, err := queue.QueueMessage(
		ctx, "session-1", "task-1", "ordinary prompt", "", messagequeue.QueuedByUser, false, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	reserved, ok := queue.ReserveQueued(ctx, "session-1")
	if !ok || reserved.ID != source.ID {
		t.Fatalf("reserved = %#v, ok=%t", reserved, ok)
	}
	service := &Service{logger: testLogger(), messageQueue: queue}
	service.finishQueuedMessageExecution(
		ctx, "session-1", "session-1", reserved, nil, false, false, false,
		&acceptedPromptDispatchError{err: errors.New("durable acceptance publication failed")},
	)
	pending, err := queue.ListPendingQueueDispatches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending dispatches = %#v, want empty", pending)
	}
}

func TestTransferredOrdinaryDispatchRestoresToDestinationAfterRestart(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/queue.db"
	queue, db := newWorkflowTransferQueue(t, dbPath)
	source, err := queue.QueueMessage(
		ctx, "session-old", "task-1", "ordinary prompt", "", messagequeue.QueuedByUser, false, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	reserved, ok := queue.ReserveQueued(ctx, source.SessionID)
	if !ok || reserved.ID != source.ID {
		t.Fatalf("reserved = %#v, ok=%t", reserved, ok)
	}
	if err := queue.TransferSession(ctx, source.SessionID, "session-new"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	restartedQueue, restartedDB := newWorkflowTransferQueue(t, dbPath)
	t.Cleanup(func() { _ = restartedDB.Close() })
	restarted := &Service{logger: testLogger(), messageQueue: restartedQueue}
	if err := restarted.reconcilePendingQueueDispatchesOnStartup(ctx); err != nil {
		t.Fatal(err)
	}
	if status := restartedQueue.GetStatus(ctx, source.SessionID); len(status.Entries) != 0 {
		t.Fatalf("source queue after transfer recovery = %#v, want empty", status.Entries)
	}
	status := restartedQueue.GetStatus(ctx, "session-new")
	if len(status.Entries) != 1 || status.Entries[0].ID != source.ID {
		t.Fatalf("destination queue after transfer recovery = %#v, want %s", status.Entries, source.ID)
	}
}
