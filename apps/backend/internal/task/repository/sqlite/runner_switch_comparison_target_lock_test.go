package sqlite

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	kandevdb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/testutil"
)

// TestSwitchTaskRunner_ConcurrentBaseBranchUpdateNeverBlendsWithSwitch covers
// the in-place task-repository link update class-2 writer added by this
// change: UpdateTaskRepositoryBaseBranchAndClearComparisonTarget mutates the
// same link a runner switch's compatibility re-check reads, so the two must
// serialize on the task row lock and produce only the two outcomes
// AC-TASKS-RUNNER-SWITCH-002.3a permits.
func TestSwitchTaskRunner_ConcurrentBaseBranchUpdateNeverBlendsWithSwitch(t *testing.T) {
	repo := newRunnerSwitchTestRepo(t)
	ctx := context.Background()
	seedRunnerSwitchWorkspace(t, repo, "ws-1")
	seedRunnerSwitchTask(t, repo, "task-1", "ws-1", seedRunnerSwitchTaskOpts{
		Metadata: `{"executor_profile_id":"profile-old"}`,
	})
	seedRunnerSwitchRepository(t, repo, "repo-1", "ws-1")
	taskRepo := seedRunnerSwitchTaskRepository(t, repo, "task-1", "repo-1")

	var wg sync.WaitGroup
	var switchErr, branchErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, switchErr = repo.SwitchTaskRunner(ctx, baseRunnerSwitchRequest("task-1", "profile-new", taskRepo))
	}()
	go func() {
		defer wg.Done()
		_, _, branchErr = repo.UpdateTaskRepositoryBaseBranchAndClearComparisonTarget(ctx, taskRepo.ID, "develop", true)
	}()
	wg.Wait()

	if branchErr != nil {
		t.Fatalf("UpdateTaskRepositoryBaseBranchAndClearComparisonTarget error = %v, want nil", branchErr)
	}

	switchRejectedAsStale := false
	if switchErr != nil {
		if !errors.Is(switchErr, repoerrors.ErrRunnerEvaluationUnavailable) {
			t.Fatalf("switch error = %v, want nil or ErrRunnerEvaluationUnavailable", switchErr)
		}
		switchRejectedAsStale = true
	}

	task, err := repo.GetTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	stored, _ := task.Metadata[models.MetaKeyExecutorProfileID].(string)

	if switchRejectedAsStale {
		if stored != "profile-old" {
			t.Fatalf("switch was rejected but stored profile = %q, want unchanged profile-old", stored)
		}
		return
	}
	if stored != "profile-new" {
		t.Fatalf("switch committed but stored profile = %q, want profile-new", stored)
	}
}

func TestPostgresRepositoryWritersUseConsistentTaskThenLinkLockOrder(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-pg-link-order", Name: "PG Link Order Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-pg-link-order", WorkspaceID: "ws-pg-link-order", Name: "pg-link-order-repo"}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	task := &models.Task{ID: "task-pg-link-order", WorkspaceID: "ws-pg-link-order", Title: "PG Link Order Task", Priority: "medium"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	link := &models.TaskRepository{ID: "task-repo-pg-link-order", TaskID: task.ID, RepositoryID: "repo-pg-link-order", BaseBranch: "main"}
	if err := repo.CreateTaskRepository(ctx, link); err != nil {
		t.Fatalf("CreateTaskRepository: %v", err)
	}

	holderDB := openSecondPostgresConnection(t, dsn, db)
	updateDB := openSecondPostgresConnection(t, dsn, db)
	comparisonDB := openSecondPostgresConnection(t, dsn, db)
	updateRepo, err := NewWithDB(updateDB, updateDB, nil)
	if err != nil {
		t.Fatalf("init update repository: %v", err)
	}
	comparisonRepo, err := NewWithDB(comparisonDB, comparisonDB, nil)
	if err != nil {
		t.Fatalf("init comparison repository: %v", err)
	}

	holderTx, err := holderDB.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin holder transaction: %v", err)
	}
	if err := kandevdb.LockTaskRowInTx(ctx, holderTx, holderDB.DriverName(), task.ID); err != nil {
		_ = holderTx.Rollback()
		t.Fatalf("lock task row: %v", err)
	}
	released := false
	t.Cleanup(func() {
		if !released {
			_ = holderTx.Rollback()
		}
	})

	updatedLink := *link
	updatedLink.BaseBranch = "develop"
	var updatePID int
	if err := updateDB.Get(&updatePID, "SELECT pg_backend_pid()"); err != nil {
		t.Fatalf("read update backend pid: %v", err)
	}
	var comparisonPID int
	if err := comparisonDB.Get(&comparisonPID, "SELECT pg_backend_pid()"); err != nil {
		t.Fatalf("read comparison backend pid: %v", err)
	}

	updateErr := make(chan error, 1)
	updateFinished := make(chan struct{})
	go func() {
		defer close(updateFinished)
		updateErr <- updateRepo.UpdateTaskRepository(ctx, &updatedLink)
	}()
	if err := waitForPostgresLock(ctx, db, updatePID, updateFinished); err != nil {
		t.Fatal(err)
	}

	comparisonErr := make(chan error, 1)
	comparisonFinished := make(chan struct{})
	go func() {
		defer close(comparisonFinished)
		_, _, err := comparisonRepo.UpdateTaskRepositoryBaseBranchAndClearComparisonTarget(ctx, link.ID, "release", true)
		comparisonErr <- err
	}()
	if err := waitForPostgresLock(ctx, db, comparisonPID, comparisonFinished); err != nil {
		t.Fatal(err)
	}

	if err := holderTx.Commit(); err != nil {
		t.Fatalf("release task lock: %v", err)
	}
	released = true

	select {
	case err := <-updateErr:
		if err != nil {
			t.Fatalf("UpdateTaskRepository: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for UpdateTaskRepository")
	}
	select {
	case err := <-comparisonErr:
		if err != nil {
			t.Fatalf("UpdateTaskRepositoryBaseBranchAndClearComparisonTarget: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for UpdateTaskRepositoryBaseBranchAndClearComparisonTarget")
	}
}
