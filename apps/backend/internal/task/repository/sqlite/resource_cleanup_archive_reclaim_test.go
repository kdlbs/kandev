package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	"github.com/kandev/kandev/internal/task/repository/managedconversation"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func newArchiveReclaimAdmissionRepo(t *testing.T) *Repository {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "archive-reclaim.db")
	writerDB, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite writer: %v", err)
	}
	t.Cleanup(func() { _ = writerDB.Close() })

	writer := sqlx.NewDb(writerDB, "sqlite3")
	repo, err := NewWithDB(writer, writer, nil)
	if err != nil {
		t.Fatalf("initialize sqlite repository: %v", err)
	}

	readerDB, err := dbutil.OpenSQLiteReader(dbPath)
	if err != nil {
		t.Fatalf("open sqlite reader: %v", err)
	}
	t.Cleanup(func() { _ = readerDB.Close() })
	repo.ro = sqlx.NewDb(readerDB, "sqlite3")
	return repo
}

func createArchivedReclaimAdmissionTask(t *testing.T, repo *Repository, taskID string) time.Time {
	t.Helper()
	ctx := t.Context()
	workspaceID := "workspace-" + taskID
	seedWorkspace(t, repo, workspaceID)
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: taskID}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.ArchiveTask(ctx, taskID); err != nil {
		t.Fatalf("archive task: %v", err)
	}
	task, err := repo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("reload archived task: %v", err)
	}
	if task.ArchivedAt == nil {
		t.Fatal("archived task has no archive generation")
	}
	return *task.ArchivedAt
}

func archiveReclaimAdmissionOperationID(taskID string, archivedAt time.Time) string {
	return "archive_reclaim:" + taskID + ":worktree-1:" + archivedAt.UTC().Format(time.RFC3339Nano)
}

func seedPersistedArchiveReclaimAdmission(t *testing.T, repo *Repository, taskID, operationID string) *models.TaskResourceCleanupJob {
	t.Helper()
	completedAt := time.Date(2026, time.October, 27, 10, 0, 0, 0, time.UTC)
	job := &models.TaskResourceCleanupJob{
		ID: "persisted-" + taskID, OperationID: operationID, TaskID: taskID,
		Trigger: models.TaskResourceCleanupTriggerArchiveReclaim,
		State:   models.TaskResourceCleanupStateFailed, ResourceSnapshot: `{"worktree_id":"worktree-1"}`,
		Attempts: 3, LastError: "preserve prior result", CompletedAt: &completedAt,
	}
	if err := repo.CreateTaskResourceCleanupJob(t.Context(), job); err != nil {
		t.Fatalf("seed persisted archive reclaim: %v", err)
	}
	return job
}

func createArchivedRecoveryClaimTask(t *testing.T, repo *Repository, taskID string) (time.Time, string, string) {
	t.Helper()
	ctx := t.Context()
	environmentID := "environment-" + taskID
	sessionID := "session-" + taskID
	workspaceID := "workspace-" + taskID
	seedWorkspace(t, repo, workspaceID)
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: taskID}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environmentID, TaskID: taskID, ExecutorType: string(models.ExecutorTypeWorktree),
		Status: models.TaskEnvironmentStatusCreating, OwnershipGeneration: 1,
	}); err != nil {
		t.Fatalf("create task environment: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, TaskEnvironmentID: environmentID,
		State: models.TaskSessionStateCreated,
	}); err != nil {
		t.Fatalf("create task session: %v", err)
	}
	if err := repo.ArchiveTask(ctx, taskID); err != nil {
		t.Fatalf("archive task: %v", err)
	}
	task, err := repo.GetTask(ctx, taskID)
	if err != nil || task.ArchivedAt == nil {
		t.Fatalf("load archive generation: task=%+v error=%v", task, err)
	}
	return *task.ArchivedAt, environmentID, sessionID
}

func createManagedDeletionBarrier(t *testing.T, repo *Repository, taskID string) {
	t.Helper()
	ctx := t.Context()
	barrier := &models.TaskResourceCleanupJob{
		ID: "managed-delete-barrier", OperationID: "managed-delete-operation", TaskID: taskID,
		Trigger: models.TaskResourceCleanupTriggerDelete, State: models.TaskResourceCleanupStatePrepared,
		ResourceSnapshot: `{}`,
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, barrier); err != nil {
		t.Fatalf("create managed deletion barrier row: %v", err)
	}
	managedSnapshot, err := managedconversation.WithDeletionEnvelope(`{}`, managedconversation.DeleteClaim{
		JobID: barrier.ID, Owner: "managed-delete-owner", Phase: managedconversation.DeletePrepared,
	})
	if err != nil {
		t.Fatalf("create managed deletion snapshot: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
		`UPDATE task_resource_cleanup_jobs SET resource_snapshot = ? WHERE id = ?`,
	), managedSnapshot, barrier.ID); err != nil {
		t.Fatalf("persist managed deletion snapshot: %v", err)
	}
}

func assertNoArchiveReclaimOperation(t *testing.T, repo *Repository, operationID string) {
	t.Helper()
	var count int
	if err := repo.db.GetContext(t.Context(), &count, repo.db.Rebind(
		`SELECT COUNT(*) FROM task_resource_cleanup_jobs WHERE operation_id = ?`,
	), operationID); err != nil {
		t.Fatalf("count archive reclaim operations: %v", err)
	}
	if count != 0 {
		t.Fatalf("archive reclaim operation count = %d, want 0", count)
	}
}

// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.3, AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.5, AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.7
func TestCreateArchiveReclaimTaskResourceCleanupJobSkipsWriterForPersistedOperation(t *testing.T) {
	repo := newArchiveReclaimAdmissionRepo(t)
	ctx := t.Context()
	archivedAt := createArchivedReclaimAdmissionTask(t, repo, "task-reclaim-existing")
	operationID := archiveReclaimAdmissionOperationID("task-reclaim-existing", archivedAt)
	persisted := seedPersistedArchiveReclaimAdmission(t, repo, "task-reclaim-existing", operationID)

	holder, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("hold sqlite writer: %v", err)
	}
	holderReleased := false
	t.Cleanup(func() {
		if !holderReleased {
			_ = holder.Rollback()
		}
	})

	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	type result struct {
		created bool
		err     error
	}
	done := make(chan result, 1)
	go func() {
		created, err := repo.CreateArchiveReclaimTaskResourceCleanupJob(callCtx, &models.TaskResourceCleanupJob{
			ID: "duplicate-reclaim", OperationID: operationID, TaskID: "task-reclaim-existing",
			Trigger: models.TaskResourceCleanupTriggerArchiveReclaim, ResourceSnapshot: `{}`,
		}, archivedAt)
		done <- result{created: created, err: err}
	}()

	select {
	case got := <-done:
		if got.err != nil || got.created {
			t.Fatalf("persisted operation = created %v, error %v; want false, nil", got.created, got.err)
		}
	case <-callCtx.Done():
		t.Fatalf("persisted operation waited for the held writer: %v", callCtx.Err())
	}

	got, err := repo.GetTaskResourceCleanupJobByOperationID(ctx, operationID)
	if err != nil {
		t.Fatalf("reload persisted operation while writer is held: %v", err)
	}
	if got.ID != persisted.ID || got.State != persisted.State || got.Attempts != persisted.Attempts ||
		got.ResourceSnapshot != persisted.ResourceSnapshot || got.LastError != persisted.LastError ||
		got.CompletedAt == nil || !got.CompletedAt.Equal(*persisted.CompletedAt) {
		t.Fatalf("persisted operation changed: got %+v, want %+v", got, persisted)
	}
	if err := holder.Rollback(); err != nil {
		t.Fatalf("release sqlite writer: %v", err)
	}
	holderReleased = true
}

// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.5
func TestCreateArchiveReclaimTaskResourceCleanupJobAdmitsCurrentGeneration(t *testing.T) {
	repo := newArchiveReclaimAdmissionRepo(t)
	archivedAt := createArchivedReclaimAdmissionTask(t, repo, "task-reclaim-current")
	operationID := archiveReclaimAdmissionOperationID("task-reclaim-current", archivedAt)
	job := &models.TaskResourceCleanupJob{
		ID: "new-reclaim", OperationID: operationID, TaskID: "task-reclaim-current",
		Trigger: models.TaskResourceCleanupTriggerArchiveReclaim, State: models.TaskResourceCleanupStatePending,
		ResourceSnapshot: `{"worktree_id":"worktree-1"}`,
	}

	created, err := repo.CreateArchiveReclaimTaskResourceCleanupJob(t.Context(), job, archivedAt)
	if err != nil || !created {
		t.Fatalf("current archive generation = created %v, error %v; want true, nil", created, err)
	}
	got, err := repo.GetTaskResourceCleanupJobByOperationID(t.Context(), operationID)
	if err != nil || got.ID != job.ID || got.State != models.TaskResourceCleanupStatePending {
		t.Fatalf("new operation = %+v, error %v; want persisted pending job", got, err)
	}
}

// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.5
func TestCreateArchiveReclaimTaskResourceCleanupJobSkipsStaleGeneration(t *testing.T) {
	repo := newArchiveReclaimAdmissionRepo(t)
	archivedAt := createArchivedReclaimAdmissionTask(t, repo, "task-reclaim-stale")
	operationID := archiveReclaimAdmissionOperationID("task-reclaim-stale", archivedAt.Add(-time.Second))
	job := &models.TaskResourceCleanupJob{
		OperationID: operationID, TaskID: "task-reclaim-stale",
		Trigger: models.TaskResourceCleanupTriggerArchiveReclaim, ResourceSnapshot: `{}`,
	}

	created, err := repo.CreateArchiveReclaimTaskResourceCleanupJob(t.Context(), job, archivedAt.Add(-time.Second))
	if err != nil || created {
		t.Fatalf("stale archive generation = created %v, error %v; want false, nil", created, err)
	}
	assertNoArchiveReclaimOperation(t, repo, operationID)
}

// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.5, AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.7
func TestCreateArchiveReclaimTaskResourceCleanupJobFailsClosedOnReaderError(t *testing.T) {
	repo := newArchiveReclaimAdmissionRepo(t)
	archivedAt := createArchivedReclaimAdmissionTask(t, repo, "task-reclaim-reader-error")
	if err := repo.ro.Close(); err != nil {
		t.Fatalf("close sqlite reader: %v", err)
	}
	operationID := archiveReclaimAdmissionOperationID("task-reclaim-reader-error", archivedAt)
	job := &models.TaskResourceCleanupJob{
		OperationID: operationID, TaskID: "task-reclaim-reader-error",
		Trigger: models.TaskResourceCleanupTriggerArchiveReclaim, ResourceSnapshot: `{}`,
	}

	created, err := repo.CreateArchiveReclaimTaskResourceCleanupJob(t.Context(), job, archivedAt)
	if err == nil || created {
		t.Fatalf("closed reader = created %v, error %v; want false and an error", created, err)
	}
	assertNoArchiveReclaimOperation(t, repo, operationID)
}

// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.5
func TestCreateArchiveReclaimTaskResourceCleanupJobRejectsCancelledContext(t *testing.T) {
	repo := newArchiveReclaimAdmissionRepo(t)
	archivedAt := createArchivedReclaimAdmissionTask(t, repo, "task-reclaim-cancelled")
	operationID := archiveReclaimAdmissionOperationID("task-reclaim-cancelled", archivedAt)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	job := &models.TaskResourceCleanupJob{
		OperationID: operationID, TaskID: "task-reclaim-cancelled",
		Trigger: models.TaskResourceCleanupTriggerArchiveReclaim, ResourceSnapshot: `{}`,
	}

	created, err := repo.CreateArchiveReclaimTaskResourceCleanupJob(ctx, job, archivedAt)
	if !errors.Is(err, context.Canceled) || created {
		t.Fatalf("cancelled context = created %v, error %v; want false and context.Canceled", created, err)
	}
	assertNoArchiveReclaimOperation(t, repo, operationID)
}

// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.7
func TestCreateArchiveReclaimTaskResourceCleanupJobRejectsManagedEnvelopeBeforeLookup(t *testing.T) {
	repo := newArchiveReclaimAdmissionRepo(t)
	archivedAt := createArchivedReclaimAdmissionTask(t, repo, "task-reclaim-envelope")
	if err := repo.ro.Close(); err != nil {
		t.Fatalf("close sqlite reader: %v", err)
	}
	snapshot, err := managedconversation.WithDeletionEnvelope(`{}`, managedconversation.DeleteClaim{
		JobID: "managed-delete-job", Owner: "managed-delete-owner", Phase: managedconversation.DeletePrepared,
	})
	if err != nil {
		t.Fatalf("create managed deletion envelope: %v", err)
	}
	job := &models.TaskResourceCleanupJob{
		OperationID: "archive_reclaim:task-reclaim-envelope:worktree-1:generation",
		TaskID:      "task-reclaim-envelope", Trigger: models.TaskResourceCleanupTriggerArchiveReclaim,
		ResourceSnapshot: snapshot,
	}

	created, err := repo.CreateArchiveReclaimTaskResourceCleanupJob(t.Context(), job, archivedAt)
	if !errors.Is(err, managedconversation.ErrUnavailable) || created {
		t.Fatalf("managed envelope = created %v, error %v; want false and ErrUnavailable", created, err)
	}
}

// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.3, AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.5, AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.7
func TestCreateArchiveReclaimTaskResourceCleanupJobSkipsExistingRecoveryClaim(t *testing.T) {
	repo := newArchiveReclaimAdmissionRepo(t)
	ctx := t.Context()
	const taskID = "task-reclaim-recovery-claim"
	archivedAt, environmentID, sessionID := createArchivedRecoveryClaimTask(t, repo, taskID)
	operationID := archiveReclaimAdmissionOperationID(taskID, archivedAt)
	seedPersistedArchiveReclaimAdmission(t, repo, taskID, operationID)
	claim, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx,
		recoveryClaimRequest(environmentID, taskID, sessionID, "recovery-claim-reclaim", 1))
	if err != nil {
		t.Fatalf("acquire recovery claim: %v", err)
	}
	t.Cleanup(func() { _ = repo.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim) })

	created, err := repo.CreateArchiveReclaimTaskResourceCleanupJob(ctx, &models.TaskResourceCleanupJob{
		OperationID: operationID, TaskID: taskID,
		Trigger: models.TaskResourceCleanupTriggerArchiveReclaim, ResourceSnapshot: `{}`,
	}, archivedAt)
	if err != nil || created {
		t.Fatalf("persisted operation under recovery claim = created %v, error %v; want false, nil", created, err)
	}
}

// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.3, AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.5, AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.7
func TestCreateArchiveReclaimTaskResourceCleanupJobRejectsNewIntentDuringRecoveryClaim(t *testing.T) {
	repo := newArchiveReclaimAdmissionRepo(t)
	ctx := t.Context()
	const taskID = "task-reclaim-recovery-claim-miss"
	archivedAt, environmentID, sessionID := createArchivedRecoveryClaimTask(t, repo, taskID)
	operationID := archiveReclaimAdmissionOperationID(taskID, archivedAt)
	claim, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx,
		recoveryClaimRequest(environmentID, taskID, sessionID, "recovery-claim-"+taskID, 1))
	if err != nil {
		t.Fatalf("acquire recovery claim: %v", err)
	}
	t.Cleanup(func() { _ = repo.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim) })

	created, err := repo.CreateArchiveReclaimTaskResourceCleanupJob(ctx, &models.TaskResourceCleanupJob{
		OperationID: operationID, TaskID: taskID,
		Trigger: models.TaskResourceCleanupTriggerArchiveReclaim, ResourceSnapshot: `{}`,
	}, archivedAt)
	if !errors.Is(err, recoveryclaim.ErrBusy) || created {
		t.Fatalf("new operation under recovery claim = created %v, error %v; want false and ErrBusy", created, err)
	}
	assertNoArchiveReclaimOperation(t, repo, operationID)
}

// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.3, AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.5, AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.7
func TestCreateArchiveReclaimTaskResourceCleanupJobSkipsManagedDeletionBarrier(t *testing.T) {
	repo := newArchiveReclaimAdmissionRepo(t)
	ctx := t.Context()
	const taskID = "task-reclaim-managed-barrier"
	archivedAt := createArchivedReclaimAdmissionTask(t, repo, taskID)
	operationID := archiveReclaimAdmissionOperationID(taskID, archivedAt)
	seedPersistedArchiveReclaimAdmission(t, repo, taskID, operationID)
	createManagedDeletionBarrier(t, repo, taskID)

	created, err := repo.CreateArchiveReclaimTaskResourceCleanupJob(ctx, &models.TaskResourceCleanupJob{
		OperationID: operationID, TaskID: taskID,
		Trigger: models.TaskResourceCleanupTriggerArchiveReclaim, ResourceSnapshot: `{}`,
	}, archivedAt)
	if err != nil || created {
		t.Fatalf("persisted operation under managed deletion barrier = created %v, error %v; want false, nil", created, err)
	}
}

// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.3, AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.5, AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.7
func TestCreateArchiveReclaimTaskResourceCleanupJobRejectsNewIntentDuringManagedDeletion(t *testing.T) {
	repo := newArchiveReclaimAdmissionRepo(t)
	ctx := t.Context()
	const taskID = "task-reclaim-managed-barrier-miss"
	archivedAt := createArchivedReclaimAdmissionTask(t, repo, taskID)
	createManagedDeletionBarrier(t, repo, taskID)

	operationID := archiveReclaimAdmissionOperationID(taskID, archivedAt)
	created, err := repo.CreateArchiveReclaimTaskResourceCleanupJob(ctx, &models.TaskResourceCleanupJob{
		OperationID: operationID, TaskID: taskID,
		Trigger: models.TaskResourceCleanupTriggerArchiveReclaim, ResourceSnapshot: `{}`,
	}, archivedAt)
	if !errors.Is(err, repoerrors.ErrTaskCleanupInProgress) || created {
		t.Fatalf("new operation under managed deletion barrier = created %v, error %v; want false and ErrTaskCleanupInProgress", created, err)
	}
	assertNoArchiveReclaimOperation(t, repo, operationID)
}
