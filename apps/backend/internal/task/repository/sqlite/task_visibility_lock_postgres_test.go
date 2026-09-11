package sqlite

// TestPostgresArchiveLocksStepAgainstConcurrentReorder and
// TestPostgresDeleteLocksStepAgainstConcurrentReorder prove ArchiveTask and
// DeleteTask lock the step a task belongs to (lockTaskStepForWrite) before
// changing its visibility, so a concurrent ReorderStepTasks of that same
// step cannot straddle the change: REQ-TASKS-KANBAN-TASK-REORDERING-001.15
// (a hidden task's position shall not be rewritten) and .26 (a task leaving
// a band's membership during the race window is a conflict, never a silent
// success). Before this lock existed, ArchiveTask and DeleteTask took no
// step-scoped lock at all, so either could commit in the window between
// ReorderStepTasks' membership read and its blind per-task position write —
// DeleteTask's case is the sharper one: the renumbering UPDATE has no
// RowsAffected check, so it would silently write zero rows for a task
// already gone, and ReorderStepTasks would still report success naming a
// task that no longer existed.
//
// Both tests use reorderPreWriteHook (mirrors the promotion-lock test) to
// pause a reorder transaction after its membership read but before its
// write, run the concurrent archive/delete during that pause, and then
// observe the target row's visibility WHILE the reorder is still paused —
// a genuinely deterministic check, since a transaction blocked on Postgres'
// row lock cannot have committed regardless of timing.
//
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/testutil"
)

func seedVisibilityLockFixture(t *testing.T, wsID, workflowID, stepID string) *Repository {
	t.Helper()
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 3)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	// ArchiveTask/DeleteTask purge the task's queue, which requires the
	// messagequeue package's own tables (not part of this package's schema).
	if _, err := messagequeue.NewSQLiteRepository(db, db); err != nil {
		t.Fatalf("init messagequeue schema: %v", err)
	}
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: wsID, Name: "Visibility lock workspace"}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: wsID, Name: "Visibility lock workflow"}); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO workflow_steps (id, workflow_id, name, position)
		VALUES (?, ?, ?, ?)
	`), stepID, workflowID, stepID, 0); err != nil {
		t.Fatalf("seed workflow step %s: %v", stepID, err)
	}
	return repo
}

// armReorderPause returns a channel closed once a ReorderStepTasks call has
// read stepID's membership and is paused before its write, plus a releaseFn
// that resumes it. releaseFn is safe to call more than once (the second call
// is a no-op) and is also registered as t.Cleanup, so a t.Fatalf between
// pausing and releasing still unblocks the paused reorder goroutine instead
// of leaking it and hanging the schema-drop cleanup.
func armReorderPause(t *testing.T, repo *Repository) (paused chan struct{}, releaseFn func()) {
	t.Helper()
	paused = make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseFn = func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseFn)

	var fired bool
	repo.reorderPreWriteHook = func() {
		if fired {
			return
		}
		fired = true
		close(paused)
		<-release
	}
	return paused, releaseFn
}

func TestPostgresArchiveLocksStepAgainstConcurrentReorder(t *testing.T) {
	const step = "archive-lock-step"
	repo := seedVisibilityLockFixture(t, "archive-lock-ws", "archive-lock-workflow", step)
	ctx := context.Background()

	archiving := &models.Task{
		ID: "archive-lock-target", WorkspaceID: "archive-lock-ws", WorkflowID: "archive-lock-workflow",
		WorkflowStepID: step, Title: "Archiving", WIPAdmitted: true,
	}
	stays := &models.Task{
		ID: "archive-lock-stays", WorkspaceID: "archive-lock-ws", WorkflowID: "archive-lock-workflow",
		WorkflowStepID: step, Title: "Stays", WIPAdmitted: true,
	}
	for _, task := range []*models.Task{archiving, stays} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("seed task %s: %v", task.ID, err)
		}
	}

	paused, release := armReorderPause(t, repo)
	t.Cleanup(func() { repo.reorderPreWriteHook = nil })

	var wg sync.WaitGroup
	var reorderErr, archiveErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _, reorderErr = repo.ReorderStepTasks(ctx, step, ReorderBandAdmitted, []string{archiving.ID, stays.ID})
	}()

	<-paused // reorder holds step's row lock, has already read archiving as still live

	wg.Add(1)
	go func() {
		defer wg.Done()
		archiveErr = repo.ArchiveTask(ctx, archiving.ID)
	}()

	// Give the archive goroutine time to either run to completion (unfixed
	// code, no step lock) or register its wait against reorder's held step
	// lock (fixed code) before we peek at its committed state.
	time.Sleep(200 * time.Millisecond)

	var archivedAlready sql.NullTime
	if err := repo.db.QueryRowContext(ctx, repo.db.Rebind(
		`SELECT archived_at FROM tasks WHERE id = ?`,
	), archiving.ID).Scan(&archivedAlready); err != nil {
		t.Fatalf("peek archived_at mid-pause: %v", err)
	}
	if archivedAlready.Valid {
		t.Fatalf("ArchiveTask committed while the reorder still held step %q's lock: it must wait behind "+
			"the reorder instead of racing its read-then-write window", step)
	}

	release()
	wg.Wait()

	if archiveErr != nil {
		t.Fatalf("archive must always succeed: %v", archiveErr)
	}
	if reorderErr != nil && !errors.Is(reorderErr, repoerrors.ErrStepChanged) {
		t.Fatalf("reorder error = %v, want nil or ErrStepChanged", reorderErr)
	}

	stored, err := repo.GetTask(ctx, archiving.ID)
	if err != nil {
		t.Fatalf("reload archived task: %v", err)
	}
	if stored.ArchivedAt == nil {
		t.Fatalf("task %q was not archived", archiving.ID)
	}
}

func TestPostgresDeleteLocksStepAgainstConcurrentReorder(t *testing.T) {
	const step = "delete-lock-step"
	repo := seedVisibilityLockFixture(t, "delete-lock-ws", "delete-lock-workflow", step)
	ctx := context.Background()

	deleting := &models.Task{
		ID: "delete-lock-target", WorkspaceID: "delete-lock-ws", WorkflowID: "delete-lock-workflow",
		WorkflowStepID: step, Title: "Deleting", WIPAdmitted: true,
	}
	stays := &models.Task{
		ID: "delete-lock-stays", WorkspaceID: "delete-lock-ws", WorkflowID: "delete-lock-workflow",
		WorkflowStepID: step, Title: "Stays", WIPAdmitted: true,
	}
	for _, task := range []*models.Task{deleting, stays} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("seed task %s: %v", task.ID, err)
		}
	}

	paused, release := armReorderPause(t, repo)
	t.Cleanup(func() { repo.reorderPreWriteHook = nil })

	var wg sync.WaitGroup
	var reorderErr, deleteErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _, reorderErr = repo.ReorderStepTasks(ctx, step, ReorderBandAdmitted, []string{deleting.ID, stays.ID})
	}()

	<-paused // reorder holds step's row lock, has already read deleting as still live

	wg.Add(1)
	go func() {
		defer wg.Done()
		deleteErr = repo.DeleteTask(ctx, deleting.ID)
	}()

	// Give the delete goroutine time to either run to completion (unfixed
	// code, no step lock) or register its wait against reorder's held step
	// lock (fixed code) before we peek at its committed state.
	time.Sleep(200 * time.Millisecond)

	var stillExists bool
	if err := repo.db.QueryRowContext(ctx, repo.db.Rebind(
		`SELECT EXISTS (SELECT 1 FROM tasks WHERE id = ?)`,
	), deleting.ID).Scan(&stillExists); err != nil {
		t.Fatalf("peek row existence mid-pause: %v", err)
	}
	if !stillExists {
		t.Fatalf("DeleteTask committed while the reorder still held step %q's lock: it must wait behind "+
			"the reorder instead of racing its read-then-write window", step)
	}

	release()
	wg.Wait()

	if deleteErr != nil {
		t.Fatalf("delete must always succeed: %v", deleteErr)
	}
	if reorderErr != nil && !errors.Is(reorderErr, repoerrors.ErrStepChanged) {
		t.Fatalf("reorder error = %v, want nil or ErrStepChanged", reorderErr)
	}

	if _, err := repo.GetTask(ctx, deleting.ID); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("GetTask(deleted task) error = %v, want ErrTaskNotFound", err)
	}
}
