package sqlite

// TestPostgresPromotionLocksSourceStepAgainstConcurrentReorder proves
// PromoteQueuedTaskIfWorkflowStepHasCapacity locks the step a task is
// LEAVING, not only the one it is entering. Before this lock existed, the
// promotion's claim held no lock a concurrent ReorderStepTasks of the source
// step would wait on, so the two transactions could interleave on Postgres:
// the reorder could read the source step's membership before the promotion
// committed (a legitimate read - the task had not left yet), then, paused
// until the promotion committed, write the promoted task's position (a bare
// UPDATE keyed only on task id, with no step predicate) after the promotion
// had already moved that task into the destination step and assigned it a
// destination-scoped position - silently clobbering that position with a
// stale source-step-scoped one.
//
// Natural goroutine scheduling almost never reproduces this: reorder
// reaches its write with fewer preliminary round trips than the promotion
// reaches its own, so it usually writes first and the promotion's later,
// predicate-guarded write simply overwrites it correctly. Constructing the
// adversarial ordering deterministically needs reorderPreWriteHook (mirrors
// usageEventPreRollupHook's role in the usage-event deadlock test) to pause
// a real reorder transaction after its membership read but before its
// write, so the promotion can run to completion in between.
//
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresPromotionLocksSourceStepAgainstConcurrentReorder(t *testing.T) {
	const (
		feeder = "promo-lock-feeder"
		target = "promo-lock-target"
	)
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "promo-lock-ws", Name: "Promotion lock workspace"}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "promo-lock-workflow", WorkspaceID: "promo-lock-ws", Name: "Promotion lock workflow"}); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
	for _, step := range []string{feeder, target} {
		if _, err := db.Exec(db.Rebind(`
			INSERT INTO workflow_steps (id, workflow_id, name, position)
			VALUES (?, ?, ?, ?)
		`), step, "promo-lock-workflow", step, 0); err != nil {
			t.Fatalf("seed workflow step %s: %v", step, err)
		}
	}

	prior := &models.Task{
		ID: "promo-lock-prior", WorkspaceID: "promo-lock-ws", WorkflowID: "promo-lock-workflow",
		WorkflowStepID: target, Title: "Prior", WIPAdmitted: true,
	}
	if err := repo.CreateTask(ctx, prior); err != nil {
		t.Fatalf("seed prior target task: %v", err)
	}

	promoted := &models.Task{
		ID: "promo-lock-promoted", WorkspaceID: "promo-lock-ws", WorkflowID: "promo-lock-workflow",
		WorkflowStepID: feeder, Title: "Promoted", WIPAdmitted: true, QueuedForStepID: target,
	}
	stays := &models.Task{
		ID: "promo-lock-stays", WorkspaceID: "promo-lock-ws", WorkflowID: "promo-lock-workflow",
		WorkflowStepID: feeder, Title: "Stays", WIPAdmitted: true,
	}
	for _, task := range []*models.Task{promoted, stays} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("seed feeder task %s: %v", task.ID, err)
		}
	}

	// Mirrors service.promoteFeederQueuedTask: the caller stamps the
	// candidate's destination membership onto the struct before invoking the
	// repository claim, since the repository only assigns the arrival
	// position, not the step itself.
	promoted.WorkflowStepID = target
	promoted.WIPAdmitted = true
	promoted.QueuedForStepID = ""

	reorderPaused := make(chan struct{})
	releaseReorder := make(chan struct{})
	var hookFired bool
	repo.reorderPreWriteHook = func() {
		if hookFired {
			return
		}
		hookFired = true
		close(reorderPaused)
		<-releaseReorder
	}
	t.Cleanup(func() { repo.reorderPreWriteHook = nil })

	var wg sync.WaitGroup
	var reorderErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _, reorderErr = repo.ReorderStepTasks(ctx, feeder, ReorderBandAdmitted, []string{promoted.ID, stays.ID})
	}()

	<-reorderPaused // reorder holds feeder's row lock and has already read promoted as still resident there

	var claimed bool
	var promoteErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		claimed, promoteErr = repo.PromoteQueuedTaskIfWorkflowStepHasCapacity(ctx, promoted, feeder, target, 0)
	}()

	// Give the promotion goroutine time to run to completion (unfixed code)
	// or register its lock wait against reorder's held feeder lock (fixed
	// code) before releasing reorder - mirrors the usage-event deadlock
	// test's registration-order wait.
	time.Sleep(200 * time.Millisecond)
	close(releaseReorder)

	wg.Wait()

	if promoteErr != nil {
		t.Fatalf("promotion must always succeed (unlimited target): %v", promoteErr)
	}
	if !claimed {
		t.Fatalf("promotion did not claim the feeder task")
	}
	if reorderErr != nil && !errors.Is(reorderErr, repoerrors.ErrStepChanged) {
		t.Fatalf("reorder error = %v, want nil or ErrStepChanged", reorderErr)
	}

	storedPromoted, err := repo.GetTask(ctx, promoted.ID)
	if err != nil {
		t.Fatalf("reload promoted task: %v", err)
	}
	if storedPromoted.WorkflowStepID != target {
		t.Fatalf("promoted task ended up in step %q, want %q", storedPromoted.WorkflowStepID, target)
	}
	storedPrior, err := repo.GetTask(ctx, prior.ID)
	if err != nil {
		t.Fatalf("reload prior target task: %v", err)
	}
	if storedPromoted.Position == storedPrior.Position {
		t.Fatalf("promoted task position %d collides with prior target task position %d: a concurrent reorder of the "+
			"feeder step overwrote the arrival position the promotion assigned", storedPromoted.Position, storedPrior.Position)
	}
}
