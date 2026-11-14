package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.11
func TestRetiredEnvironmentsAreExcludedFromCanonicalBindings(t *testing.T) {
	for _, backend := range []struct {
		name string
		open func(*testing.T) *Repository
	}{
		{name: "sqlite", open: newRepoForEntityTests},
		{name: "postgres", open: func(t *testing.T) *Repository {
			repo, _, _ := newTaskPostgresRepoPair(t)
			return repo
		}},
	} {
		t.Run(backend.name, func(t *testing.T) {
			for _, withActive := range []bool{false, true} {
				name := "retired only"
				if withActive {
					name = "retired and active"
				}
				t.Run(name, func(t *testing.T) {
					testRetiredCanonicalBinding(t, backend.open(t), withActive)
				})
			}
		})
	}
}

func testRetiredCanonicalBinding(t *testing.T, repo *Repository, withActive bool) {
	t.Helper()
	ctx := context.Background()
	const taskID = "task-retired-canonical"
	seedMaterializationClaimTask(t, repo, taskID)
	retired := &models.TaskEnvironment{
		ID: "a-retired-canonical", TaskID: taskID,
		ExecutorType: string(models.ExecutorTypeWorktree), Status: models.TaskEnvironmentStatusFailed,
		WorkspacePath: t.TempDir(),
	}
	if err := repo.CreateTaskEnvironment(ctx, retired); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE task_environments SET retired_at = ? WHERE id = ?`), time.Now().UTC(), retired.ID); err != nil {
		t.Fatal(err)
	}
	const activeID = "z-active-canonical"
	if withActive {
		if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
			ID: activeID, TaskID: taskID,
			ExecutorType: string(models.ExecutorTypeWorktree), Status: models.TaskEnvironmentStatusReady,
			WorkspacePath: t.TempDir(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	workspace, err := repo.loadCanonicalTaskWorkspaceForSessionTx(ctx, tx, taskID)
	if withActive {
		if err != nil || workspace == nil || workspace.ID != activeID {
			t.Errorf("shared-group canonical binding = %+v, %v; want %s", workspace, err, activeID)
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("shared-group canonical binding = %+v, %v; want no active owner", workspace, err)
	}
	canonical, err := repo.loadInheritedTransitionCanonicalTx(ctx, tx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if withActive {
		if canonical == nil || canonical.ID != activeID {
			t.Errorf("inherited-transition canonical binding = %+v; want %s", canonical, activeID)
		}
	} else if canonical != nil {
		t.Errorf("inherited-transition canonical binding = %+v; want no active owner", canonical)
	}
}
