package sqlite

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

// TestArchiveTaskNotifiesQueuePurgeAfterCommit proves the post-commit purge
// notifier fires after ArchiveTask empties queued_messages. Live badge zeroing
// depends on this hook publishing message.queue.status_changed.
func TestArchiveTaskNotifiesQueuePurgeAfterCommit(t *testing.T) {
	repo := newRepoForArchiveTests(t, "task-queue-purge-notify")
	ctx := context.Background()
	seedLiveSessionForQueue(t, repo, "session-1", "task-queue-purge-notify")

	mqRepo, err := messagequeue.NewSQLiteRepository(repo.db, repo.db)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	queue := messagequeue.NewService(mqRepo, messagequeue.DefaultMaxPerSession, log)
	identity := queueIdentityForSession(t, repo, "session-1")
	if _, err := mqRepo.SetAutoMergeOverride(ctx, identity, false); err != nil {
		t.Fatalf("SetAutoMergeOverride: %v", err)
	}
	if err := queue.SetPendingMove(ctx, identity.SessionID, &messagequeue.PendingMove{
		MoveID:               "archive-pending-move",
		SessionIncarnationID: identity.SessionIncarnationID,
		TaskID:               identity.TaskID,
	}); err != nil {
		t.Fatalf("SetPendingMove: %v", err)
	}
	staleIdentity := identity
	staleIdentity.SessionIncarnationID = "stale-incarnation"
	if _, err := mqRepo.SetAutoMergeOverride(ctx, staleIdentity, true); !errors.Is(err, messagequeue.ErrSessionIdentityMismatch) {
		t.Fatalf("stale SetAutoMergeOverride error = %v, want identity mismatch", err)
	}
	if _, err := queue.QueueMessage(ctx, "session-1", "task-queue-purge-notify", "follow up", "", "user", false, nil); err != nil {
		t.Fatalf("QueueMessage: %v", err)
	}
	if got, err := queue.CountPendingByTask(ctx, "task-queue-purge-notify"); err != nil || got != 1 {
		t.Fatalf("pending before archive = %d err=%v, want 1", got, err)
	}

	var notified atomic.Int32
	var notifiedTask string
	repo.SetTaskQueuePurgeNotifier(func(_ context.Context, taskID string) {
		notified.Add(1)
		notifiedTask = taskID
	})

	if err := repo.ArchiveTask(ctx, "task-queue-purge-notify"); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}

	if notified.Load() != 1 {
		t.Fatalf("purge notifier calls = %d, want 1 after ArchiveTask", notified.Load())
	}
	if notifiedTask != "task-queue-purge-notify" {
		t.Fatalf("notified task_id = %q, want task-queue-purge-notify", notifiedTask)
	}
	if got, err := queue.CountPendingByTask(ctx, "task-queue-purge-notify"); err != nil || got != 0 {
		t.Fatalf("pending after archive = %d err=%v, want 0", got, err)
	}
	if override, err := mqRepo.GetAutoMergeOverride(ctx, identity); err != nil || override == nil || override.Enabled {
		t.Fatalf("archive override = %+v err=%v, want preserved OFF", override, err)
	}
	if move, ok := queue.GetPendingMove(ctx, identity.SessionID); ok || move != nil {
		t.Fatalf("archive pending move = %+v exists=%t, want absent", move, ok)
	}

	task, err := repo.GetTask(ctx, "task-queue-purge-notify")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.ArchivedAt == nil {
		t.Fatal("ArchivedAt = nil after archive")
	}
}

func TestArchiveTaskPurgesDurableQueueRecovery(t *testing.T) {
	repo := newRepoForArchiveTests(t, "task-archive-recovery")
	ctx := context.Background()
	const sessionID = "session-archive-recovery"
	seedLiveSessionForQueue(t, repo, sessionID, "task-archive-recovery")
	mqRepo, err := messagequeue.NewSQLiteRepository(repo.db, repo.db)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	queue := messagequeue.NewService(mqRepo, messagequeue.DefaultMaxPerSession, log)
	entry, err := queue.QueueMessage(
		ctx, sessionID, "task-archive-recovery", "follow up", "", "user", false, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if reserved, ok := queue.ReserveQueued(ctx, sessionID); !ok || reserved.ID != entry.ID {
		t.Fatalf("reserved = %#v, ok=%t", reserved, ok)
	}
	if err := queue.UpsertAttachmentCleanup(ctx, messagequeue.AttachmentCleanup{
		SessionID: sessionID, EntryID: entry.ID, OperationID: "archive-cleanup",
		TaskID: entry.TaskID, Attachments: []messagequeue.MessageAttachment{{AttachmentID: "attachment"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ArchiveTask(ctx, entry.TaskID); err != nil {
		t.Fatal(err)
	}
	dispatches, err := queue.ListPendingQueueDispatches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cleanups, err := queue.ListAttachmentCleanups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(dispatches) != 0 || len(cleanups) != 1 {
		t.Fatalf("recovery rows after archive: dispatches=%#v cleanups=%#v", dispatches, cleanups)
	}
}

func TestDeleteTaskNotifiesQueuePurgeAfterCommit(t *testing.T) {
	repo := newRepoForArchiveTests(t, "task-delete-purge-notify")
	ctx := context.Background()
	seedLiveSessionForQueue(t, repo, "session-1", "task-delete-purge-notify")

	mqRepo, err := messagequeue.NewSQLiteRepository(repo.db, repo.db)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	queue := messagequeue.NewService(mqRepo, messagequeue.DefaultMaxPerSession, log)
	identity := queueIdentityForSession(t, repo, "session-1")
	if _, err := mqRepo.SetAutoMergeOverride(ctx, identity, false); err != nil {
		t.Fatalf("SetAutoMergeOverride: %v", err)
	}
	if _, err := queue.QueueMessage(ctx, "session-1", "task-delete-purge-notify", "follow up", "", "user", false, nil); err != nil {
		t.Fatalf("QueueMessage: %v", err)
	}

	var notified atomic.Int32
	repo.SetTaskQueuePurgeNotifier(func(_ context.Context, taskID string) {
		if taskID == "task-delete-purge-notify" {
			notified.Add(1)
		}
	})

	if err := repo.DeleteTask(ctx, "task-delete-purge-notify"); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if notified.Load() != 1 {
		t.Fatalf("purge notifier calls = %d, want 1 after DeleteTask", notified.Load())
	}
	if _, err := repo.GetTask(ctx, "task-delete-purge-notify"); err == nil {
		t.Fatal("expected task gone after DeleteTask")
	}
	override, err := mqRepo.GetAutoMergeOverride(ctx, identity)
	if override != nil || !errors.Is(err, messagequeue.ErrSessionIdentityMismatch) {
		t.Fatalf("deleted task override = %+v err=%v, want fail-closed identity mismatch", override, err)
	}
}

func TestDeleteTaskSessionPurgesQueuedMessages(t *testing.T) {
	// Session delete used to leave queued_messages rows behind; CountPendingByTask
	// then kept the sidebar badge inflated. Cascade purge must clear them.
	repo := newRepoForArchiveTests(t, "task-session-queue-purge")
	ctx := context.Background()

	seedLiveSessionForQueue(t, repo, "session-1", "task-session-queue-purge")
	seedLiveSessionForQueue(t, repo, "session-drop", "task-session-queue-purge")

	mqRepo, err := messagequeue.NewSQLiteRepository(repo.db, repo.db)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	queue := messagequeue.NewService(mqRepo, messagequeue.DefaultMaxPerSession, log)
	dropIdentity := queueIdentityForSession(t, repo, "session-drop")
	if _, err := mqRepo.SetAutoMergeOverride(ctx, dropIdentity, false); err != nil {
		t.Fatalf("SetAutoMergeOverride: %v", err)
	}
	if err := queue.SetPendingMove(ctx, dropIdentity.SessionID, &messagequeue.PendingMove{
		MoveID:               "session-delete-pending-move",
		SessionIncarnationID: dropIdentity.SessionIncarnationID,
		TaskID:               dropIdentity.TaskID,
	}); err != nil {
		t.Fatalf("SetPendingMove: %v", err)
	}
	if _, err := queue.QueueMessage(ctx, "session-drop", "task-session-queue-purge", "orphan me", "", "user", false, nil); err != nil {
		t.Fatalf("QueueMessage drop: %v", err)
	}
	if _, err := queue.QueueMessage(ctx, "session-1", "task-session-queue-purge", "keep me", "", "user", false, nil); err != nil {
		t.Fatalf("QueueMessage keep: %v", err)
	}

	if err := deleteTaskSessionForTest(t, repo, ctx, "session-drop"); err != nil {
		t.Fatalf("DeleteTaskSession: %v", err)
	}
	if got := queue.GetStatus(ctx, "session-drop").Count; got != 0 {
		t.Fatalf("session-drop queue count = %d, want 0", got)
	}
	if got, err := queue.CountPendingByTask(ctx, "task-session-queue-purge"); err != nil || got != 1 {
		t.Fatalf("pending after session delete = %d err=%v, want 1 (kept session)", got, err)
	}
	if override, err := mqRepo.GetAutoMergeOverride(ctx, dropIdentity); !errors.Is(err, messagequeue.ErrSessionIdentityMismatch) || override != nil {
		t.Fatalf("deleted session override = %+v err=%v, want identity mismatch", override, err)
	}
	if move, ok := queue.GetPendingMove(ctx, dropIdentity.SessionID); ok || move != nil {
		t.Fatalf("deleted session pending move = %+v exists=%t, want absent", move, ok)
	}
}

func TestDeleteTaskSessionPurgesDurableQueueClaimsInTransaction(t *testing.T) {
	repo := newRepoForArchiveTests(t, "task-session-recovery-purge")
	ctx := context.Background()
	const sessionID = "session-recovery-purge"
	seedLiveSessionForQueue(t, repo, sessionID, "task-session-recovery-purge")

	mqRepo, err := messagequeue.NewSQLiteRepository(repo.db, repo.db)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	queue := messagequeue.NewService(mqRepo, messagequeue.DefaultMaxPerSession, log)
	dispatchEntry, err := queue.QueueMessage(ctx, sessionID, "task-session-recovery-purge", "dispatch", "", "user", false, nil)
	if err != nil {
		t.Fatalf("queue dispatch entry: %v", err)
	}
	if reserved, ok := queue.ReserveQueued(ctx, sessionID); !ok || reserved.ID != dispatchEntry.ID {
		t.Fatalf("reserved dispatch entry = %#v, ok=%t", reserved, ok)
	}
	sendNowEntry, err := queue.QueueMessage(ctx, sessionID, "task-session-recovery-purge", "send now", "", "user", false, nil)
	if err != nil {
		t.Fatalf("queue Send Now entry: %v", err)
	}
	if _, err := mqRepo.ClaimSendNow(ctx, sessionID, []messagequeue.QueuedMessage{*sendNowEntry}); err != nil {
		t.Fatalf("claim Send Now entry: %v", err)
	}

	if err := deleteTaskSessionByID(ctx, repo, sessionID); err != nil {
		t.Fatalf("DeleteTaskSession: %v", err)
	}
	dispatches, err := mqRepo.(interface {
		ListPendingQueueDispatches(context.Context) ([]messagequeue.PendingQueueDispatch, error)
	}).ListPendingQueueDispatches(ctx)
	if err != nil {
		t.Fatalf("list dispatch claims: %v", err)
	}
	sendNowClaims, err := mqRepo.(interface {
		ListPendingSendNowClaims(context.Context) ([]messagequeue.PendingSendNowClaim, error)
	}).ListPendingSendNowClaims(ctx)
	if err != nil {
		t.Fatalf("list Send Now claims: %v", err)
	}
	if len(dispatches) != 0 || len(sendNowClaims) != 0 {
		t.Fatalf("durable claims after session delete: dispatches=%#v send_now=%#v", dispatches, sendNowClaims)
	}
}

func TestDeleteTaskSessionRejectsActiveQueueTransfer(t *testing.T) {
	repo := newRepoForArchiveTests(t, "task-session-transfer")
	ctx := context.Background()
	seedLiveSessionForQueue(t, repo, "session-old", "task-session-transfer")
	seedLiveSessionForQueue(t, repo, "session-new", "task-session-transfer")
	queueRepo, err := messagequeue.NewSQLiteRepository(repo.db, repo.db)
	if err != nil {
		t.Fatal(err)
	}
	compensations := queueRepo.(interface {
		UpsertSessionTransferCompensation(context.Context, messagequeue.SessionTransferCompensation) error
	})
	if err := compensations.UpsertSessionTransferCompensation(ctx, messagequeue.SessionTransferCompensation{
		OperationID: "session-delete-transfer",
		TaskID:      "task-session-transfer", FromSessionID: "session-old", ToSessionID: "session-new",
	}); err != nil {
		t.Fatal(err)
	}

	err = deleteTaskSessionByID(ctx, repo, "session-old")

	if !errors.Is(err, messagequeue.ErrSessionTransferInProgress) {
		t.Fatalf("DeleteTaskSession error = %v, want %v", err, messagequeue.ErrSessionTransferInProgress)
	}
	var count int
	if err := repo.db.GetContext(ctx, &count, `SELECT COUNT(*) FROM task_sessions WHERE id = 'session-old'`); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("session-old row count = %d, want 1", count)
	}
}

func TestDeleteTaskSessionNotifiesQueueSessionPurge(t *testing.T) {
	repo := newRepoForArchiveTests(t, "task-session-queue-purge-notify")
	ctx := context.Background()
	seedLiveSessionForQueue(t, repo, "session-drop", "task-session-queue-purge-notify")

	var notifiedTask, notifiedSession string
	repo.SetTaskSessionQueuePurgeNotifier(func(_ context.Context, taskID, sessionID string) {
		notifiedTask = taskID
		notifiedSession = sessionID
	})

	if err := deleteTaskSessionByID(ctx, repo, "session-drop"); err != nil {
		t.Fatalf("DeleteTaskSession: %v", err)
	}
	if notifiedTask != "task-session-queue-purge-notify" || notifiedSession != "session-drop" {
		t.Fatalf("session purge notification = (%q, %q), want (%q, %q)",
			notifiedTask, notifiedSession, "task-session-queue-purge-notify", "session-drop")
	}
}

func TestDeleteTaskSessionPurgesPendingMove(t *testing.T) {
	repo := newRepoForArchiveTests(t, "task-session-pending-move")
	ctx := context.Background()
	seedLiveSessionForQueue(t, repo, "session-drop", "task-session-pending-move")

	mqRepo, err := messagequeue.NewSQLiteRepository(repo.db, repo.db)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	if err := mqRepo.SetPendingMove(ctx, "session-drop", &messagequeue.PendingMove{
		TaskID: "task-session-pending-move",
	}); err != nil {
		t.Fatalf("SetPendingMove: %v", err)
	}

	if err := deleteTaskSessionByID(ctx, repo, "session-drop"); err != nil {
		t.Fatalf("DeleteTaskSession: %v", err)
	}
	move, err := mqRepo.GetPendingMove(ctx, "session-drop")
	if err != nil {
		t.Fatalf("GetPendingMove: %v", err)
	}
	if move != nil {
		t.Fatalf("pending move after session delete = %#v, want nil", move)
	}

	err = mqRepo.Insert(ctx, &messagequeue.QueuedMessage{
		SessionID: "session-drop",
		TaskID:    "task-session-pending-move",
		Content:   "must reject deleted session",
		QueuedBy:  "user",
	}, 0)
	if !errors.Is(err, messagequeue.ErrTaskInactive) {
		t.Fatalf("queue admission after session delete error = %v, want ErrTaskInactive", err)
	}
	if err := mqRepo.SetPendingMove(ctx, "session-drop", &messagequeue.PendingMove{
		TaskID: "task-session-pending-move",
	}); !errors.Is(err, messagequeue.ErrTaskInactive) {
		t.Fatalf("pending move after session delete error = %v, want ErrTaskInactive", err)
	}
	err = mqRepo.ReplaceSession(ctx, "session-drop", []messagequeue.QueuedMessage{{
		ID:        "restored-after-delete",
		SessionID: "session-drop",
		TaskID:    "task-session-pending-move",
		Content:   "must not restore",
		QueuedBy:  "user",
	}}, nil)
	if !errors.Is(err, messagequeue.ErrTaskInactive) {
		t.Fatalf("session restore after delete error = %v, want ErrTaskInactive", err)
	}
}

func seedLiveSessionForQueue(t *testing.T, repo *Repository, sessionID, taskID string) {
	t.Helper()
	now := time.Now().UTC()
	if err := repo.CreateTaskSession(context.Background(), &models.TaskSession{
		ID: sessionID, TaskID: taskID,
		State: models.TaskSessionStateCompleted, StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateTaskSession(%s): %v", sessionID, err)
	}
}

func queueIdentityForSession(
	t *testing.T,
	repo *Repository,
	sessionID string,
) messagequeue.QueueSessionIdentity {
	t.Helper()
	session, err := repo.GetTaskSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession(%s): %v", sessionID, err)
	}
	return messagequeue.QueueSessionIdentity{
		TaskID:               session.TaskID,
		SessionID:            session.ID,
		SessionIncarnationID: session.QueueIncarnationID,
	}
}

func deleteTaskSessionByID(ctx context.Context, repo *Repository, sessionID string) error {
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return err
	}
	return repo.DeleteTaskSession(ctx, session)
}
