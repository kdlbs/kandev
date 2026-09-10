package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync/atomic"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
)

func TestSendNowOrdinaryClaimRestoresAfterProcessRestart(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/queue.db"
	queue, db := newWorkflowTransferQueue(t, dbPath)
	source, err := queue.QueueMessage(
		ctx, "session-1", "task-1", "ordinary prompt", "", messagequeue.QueuedByUser, false, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queue.ClaimSendNow(ctx, "session-1", []messagequeue.QueuedMessage{*source}); err != nil {
		t.Fatal(err)
	}
	if queue.GetStatus(ctx, "session-1").Count != 0 {
		t.Fatal("claimed ordinary prompt remained visible before restart")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	restartedQueue, restartedDB := newWorkflowTransferQueue(t, dbPath)
	t.Cleanup(func() { _ = restartedDB.Close() })
	restarted := &Service{logger: testLogger(), messageQueue: restartedQueue}
	if err := restarted.reconcilePendingSendNowClaimsOnStartup(ctx); err != nil {
		t.Fatal(err)
	}
	status := restartedQueue.GetStatus(ctx, "session-1")
	if len(status.Entries) != 1 || status.Entries[0].ID != source.ID {
		t.Fatalf("queue after restart recovery = %#v, want source %s", status.Entries, source.ID)
	}
}

func TestSendNowWorkerCancellationAfterClaimRestoresOrdinarySource(t *testing.T) {
	ctx := context.Background()
	queue, db := newWorkflowTransferQueue(t, t.TempDir()+"/queue.db")
	t.Cleanup(func() { _ = db.Close() })
	source, err := queue.QueueMessage(
		ctx, "session-1", "task-1", "ordinary prompt", "", messagequeue.QueuedByUser, false, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := queue.ClaimSendNow(ctx, "session-1", []messagequeue.QueuedMessage{*source})
	if err != nil {
		t.Fatal(err)
	}
	workerCtx, cancel := context.WithCancel(ctx)
	cancel()
	svc := &Service{logger: testLogger(), messageQueue: queue}

	svc.executeSendNowClaimWithContext(workerCtx, claim, nil)

	status := queue.GetStatus(ctx, "session-1")
	if len(status.Entries) != 1 || status.Entries[0].ID != source.ID {
		t.Fatalf("queue after cancelled claimed worker = %#v, want source %s", status.Entries, source.ID)
	}
}

func TestSendNowAcceptedClaimIsAcknowledgedAfterProcessRestart(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/queue.db"
	queue, db := newWorkflowTransferQueue(t, dbPath)
	source, err := queue.QueueMessage(
		ctx, "session-1", "task-1", "accepted prompt", "", messagequeue.QueuedByUser, false, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := queue.ClaimSendNow(ctx, "session-1", []messagequeue.QueuedMessage{*source})
	if err != nil {
		t.Fatal(err)
	}
	marker, ok := any(queue).(interface {
		MarkPendingSendNowClaimAccepted(context.Context, *messagequeue.SendNowClaim) error
	})
	if !ok {
		t.Fatal("message queue cannot durably mark accepted Send Now claims")
	}
	if err := marker.MarkPendingSendNowClaimAccepted(ctx, claim); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	restartedQueue, restartedDB := newWorkflowTransferQueue(t, dbPath)
	t.Cleanup(func() { _ = restartedDB.Close() })
	restarted := &Service{logger: testLogger(), messageQueue: restartedQueue}
	if err := restarted.reconcilePendingSendNowClaimsOnStartup(ctx); err != nil {
		t.Fatal(err)
	}
	if status := restartedQueue.GetStatus(ctx, "session-1"); status.Count != 0 {
		t.Fatalf("accepted Send Now source was restored after restart: %#v", status.Entries)
	}
}

type transientAcceptedMarkerRepository struct {
	messagequeue.Repository
	pending interface {
		ListPendingSendNowClaims(context.Context) ([]messagequeue.PendingSendNowClaim, error)
		MarkPendingSendNowClaimAccepted(context.Context, *messagequeue.SendNowClaim) error
		DeletePendingSendNowClaim(context.Context, *messagequeue.SendNowClaim) error
	}
	failures atomic.Int32
}

func (r *transientAcceptedMarkerRepository) ListPendingSendNowClaims(
	ctx context.Context,
) ([]messagequeue.PendingSendNowClaim, error) {
	return r.pending.ListPendingSendNowClaims(ctx)
}

func (r *transientAcceptedMarkerRepository) MarkPendingSendNowClaimAccepted(
	ctx context.Context,
	claim *messagequeue.SendNowClaim,
) error {
	if r.failures.Add(-1) >= 0 {
		return errors.New("accepted marker temporarily unavailable")
	}
	return r.pending.MarkPendingSendNowClaimAccepted(ctx, claim)
}

func (r *transientAcceptedMarkerRepository) DeletePendingSendNowClaim(
	ctx context.Context,
	claim *messagequeue.SendNowClaim,
) error {
	return r.pending.DeletePendingSendNowClaim(ctx, claim)
}

func TestAcceptedSendNowMarkerFailureDoesNotRestoreDeliveredPrompt(t *testing.T) {
	ctx := context.Background()
	raw, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "queue.db"))
	if err != nil {
		t.Fatal(err)
	}
	db := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	baseRepo, err := messagequeue.NewSQLiteRepository(db, db)
	if err != nil {
		t.Fatal(err)
	}
	pending := baseRepo.(interface {
		ListPendingSendNowClaims(context.Context) ([]messagequeue.PendingSendNowClaim, error)
		MarkPendingSendNowClaimAccepted(context.Context, *messagequeue.SendNowClaim) error
		DeletePendingSendNowClaim(context.Context, *messagequeue.SendNowClaim) error
	})
	failingRepo := &transientAcceptedMarkerRepository{Repository: baseRepo, pending: pending}
	failingRepo.failures.Store(6)
	queue := messagequeue.NewService(failingRepo, messagequeue.DefaultMaxPerSession, testLogger())

	taskRepo := setupTestRepo(t)
	seedSession(t, taskRepo, "task-accepted-marker", "session-accepted-marker", "step-1")
	seedExecutorRunning(t, taskRepo, "session-accepted-marker", "task-accepted-marker", "exec-1")
	session, err := taskRepo.GetTaskSession(ctx, "session-accepted-marker")
	if err != nil {
		t.Fatal(err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	if err := taskRepo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: taskRepo}
	svc := createTestServiceWithAgent(taskRepo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, taskRepo, testLogger(), executor.ExecutorConfig{})
	svc.messageQueue = queue

	source, err := queue.QueueMessage(
		ctx, session.ID, session.TaskID, "accepted prompt", "", messagequeue.QueuedByUser, false, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := queue.ClaimSendNow(ctx, session.ID, []messagequeue.QueuedMessage{*source})
	if err != nil {
		t.Fatal(err)
	}
	svc.markQueuedDispatchInFlight(session.ID, claim.Dispatch.ID)
	svc.executeSendNowClaimWithContext(ctx, claim, nil)

	if status := queue.GetStatus(ctx, session.ID); status.Count != 0 {
		t.Fatalf("accepted prompt sources were restored: %#v", status.Entries)
	}
	claims, err := queue.ListPendingSendNowClaims(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(claims) != 0 {
		t.Fatalf("accepted claim was not acknowledged after marker failures: %#v", claims)
	}
}
