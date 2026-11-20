package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestTaskStateCommit(t *testing.T) {
	t.Run("returns the committed snapshot and authoritative prior state", func(t *testing.T) {
		repo := newRepoForHealTests(t)
		insertTask(t, repo.db, "commit-success")
		stepID := ""
		expectedState := v1.TaskStateTODO
		change, err := repo.CommitTaskState(context.Background(), "commit-success", models.TaskStateWrite{
			State:                  v1.TaskStateInProgress,
			ExpectedWorkflowStepID: &stepID,
			ExpectedState:          &expectedState,
		})
		if err != nil {
			t.Fatalf("CommitTaskState: %v", err)
		}
		if !change.Matched || !change.Changed {
			t.Fatalf("receipt = %+v, want a matched transition", change)
		}
		if change.OldState != v1.TaskStateTODO || change.Task == nil || change.Task.State != v1.TaskStateInProgress {
			t.Fatalf("receipt = %+v, want TODO -> IN_PROGRESS", change)
		}
	})

	t.Run("rejects mismatched state and step guards", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			write models.TaskStateWrite
		}{
			{
				name:  "state",
				write: models.TaskStateWrite{State: v1.TaskStateCancelled, ExpectedState: ptrState(v1.TaskStateInProgress)},
			},
			{
				name:  "step",
				write: models.TaskStateWrite{State: v1.TaskStateCancelled, ExpectedWorkflowStepID: ptrString("stale-step")},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				repo := newRepoForHealTests(t)
				insertTask(t, repo.db, "commit-reject")
				change, err := repo.CommitTaskState(context.Background(), "commit-reject", tc.write)
				if err != nil {
					t.Fatalf("CommitTaskState: %v", err)
				}
				if change.Matched || change.Changed || change.Task != nil {
					t.Fatalf("receipt = %+v, want rejected guard", change)
				}
				stored, err := repo.GetTask(context.Background(), "commit-reject")
				if err != nil {
					t.Fatalf("GetTask: %v", err)
				}
				if stored.State != v1.TaskState("todo") {
					t.Fatalf("stored state = %q, want unchanged legacy todo", stored.State)
				}
			})
		}
	})

	t.Run("matched no-op does not change the row timestamp", func(t *testing.T) {
		repo := newRepoForHealTests(t)
		insertTask(t, repo.db, "commit-noop")
		before := time.Date(2020, 2, 3, 4, 5, 6, 0, time.UTC)
		if _, err := repo.db.Exec(`UPDATE tasks SET updated_at = ? WHERE id = ?`, before, "commit-noop"); err != nil {
			t.Fatalf("set timestamp: %v", err)
		}
		change, err := repo.CommitTaskState(context.Background(), "commit-noop", models.TaskStateWrite{State: v1.TaskStateTODO})
		if err != nil {
			t.Fatalf("CommitTaskState: %v", err)
		}
		if !change.Matched || change.Changed || change.OldState != v1.TaskStateTODO {
			t.Fatalf("receipt = %+v, want matched no-op", change)
		}
		var after time.Time
		if err := repo.db.Get(&after, `SELECT updated_at FROM tasks WHERE id = ?`, "commit-noop"); err != nil {
			t.Fatalf("read timestamp: %v", err)
		}
		if !after.Equal(before) {
			t.Fatalf("updated_at = %s, want unchanged %s", after, before)
		}
	})

	t.Run("accepts an already restored target without accepting other state", func(t *testing.T) {
		repo := newRepoForHealTests(t)
		insertTask(t, repo.db, "commit-retry")
		if _, err := repo.db.Exec(`UPDATE tasks SET state = ? WHERE id = ?`, v1.TaskStateInProgress, "commit-retry"); err != nil {
			t.Fatalf("set restored task state: %v", err)
		}
		expectedState := v1.TaskStateCancelled
		write := models.TaskStateWrite{
			State: v1.TaskStateInProgress, ExpectedState: &expectedState, AcceptIfAlreadyAtTarget: true,
		}
		change, err := repo.CommitTaskState(context.Background(), "commit-retry", write)
		if err != nil {
			t.Fatalf("CommitTaskState already-restored retry: %v", err)
		}
		if !change.Matched || change.Changed || change.Task == nil || change.Task.State != v1.TaskStateInProgress {
			t.Fatalf("already-restored receipt = %+v, want matched no-op at IN_PROGRESS", change)
		}

		if _, err := repo.db.Exec(`UPDATE tasks SET state = ? WHERE id = ?`, v1.TaskStateFailed, "commit-retry"); err != nil {
			t.Fatalf("set unrelated task state: %v", err)
		}
		change, err = repo.CommitTaskState(context.Background(), "commit-retry", write)
		if err != nil {
			t.Fatalf("CommitTaskState unrelated state: %v", err)
		}
		if change.Matched || change.Changed || change.Task != nil {
			t.Fatalf("unrelated state receipt = %+v, want rejected guard", change)
		}
		stored, err := repo.GetTask(context.Background(), "commit-retry")
		if err != nil {
			t.Fatalf("GetTask: %v", err)
		}
		if stored.State != v1.TaskStateFailed {
			t.Fatalf("stored state = %s, want unrelated FAILED state unchanged", stored.State)
		}
	})

	t.Run("missing task returns the repository not-found error", func(t *testing.T) {
		repo := newRepoForHealTests(t)
		_, err := repo.CommitTaskState(context.Background(), "missing-commit", models.TaskStateWrite{State: v1.TaskStateCancelled})
		if !errors.Is(err, ErrTaskNotFound) {
			t.Fatalf("CommitTaskState error = %v, want ErrTaskNotFound", err)
		}
	})

	t.Run("completion guard failure rolls back", func(t *testing.T) {
		repo := newRepoForBuiltinWorkflowTests(t)
		ctx := context.Background()
		if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "commit-required", Name: "Office"}); err != nil {
			t.Fatalf("CreateWorkspace: %v", err)
		}
		workflowID, err := repo.EnsureRoutineWorkflow(ctx, "commit-required")
		if err != nil {
			t.Fatalf("EnsureRoutineWorkflow: %v", err)
		}
		var startStep string
		if err := repo.db.QueryRowContext(ctx, `SELECT id FROM workflow_steps WHERE workflow_id = ? AND is_start_step = 1`, workflowID).Scan(&startStep); err != nil {
			t.Fatalf("read start step: %v", err)
		}
		task := &models.Task{
			ID: "commit-required-task", WorkspaceID: "commit-required", WorkflowID: workflowID,
			WorkflowStepID: startStep, State: v1.TaskStateInProgress,
			Metadata: map[string]interface{}{models.MetaKeyRequiredSubtaskCount: 1},
		}
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
		_, err = repo.CommitTaskState(ctx, task.ID, models.TaskStateWrite{State: v1.TaskStateCompleted})
		if err == nil {
			t.Fatal("CommitTaskState succeeded despite the required-subtask completion guard")
		}
		stored, err := repo.GetTask(ctx, task.ID)
		if err != nil {
			t.Fatalf("GetTask: %v", err)
		}
		if stored.State != v1.TaskStateInProgress {
			t.Fatalf("state = %q, want IN_PROGRESS after rollback", stored.State)
		}
	})

	t.Run("stale competing state writer cannot overwrite the first commit", func(t *testing.T) {
		repo := newRepoForHealTests(t)
		insertTask(t, repo.db, "commit-competing")
		expected := v1.TaskStateTODO
		first, err := repo.CommitTaskState(context.Background(), "commit-competing", models.TaskStateWrite{
			State: v1.TaskStateCancelled, ExpectedState: &expected,
		})
		if err != nil || !first.Changed {
			t.Fatalf("first commit = %+v, %v", first, err)
		}
		second, err := repo.CommitTaskState(context.Background(), "commit-competing", models.TaskStateWrite{
			State: v1.TaskStateCompleted, ExpectedState: &expected,
		})
		if err != nil {
			t.Fatalf("stale commit: %v", err)
		}
		if second.Matched || second.Changed {
			t.Fatalf("stale receipt = %+v, want guard rejection", second)
		}
		stored, err := repo.GetTask(context.Background(), "commit-competing")
		if err != nil {
			t.Fatalf("GetTask: %v", err)
		}
		if stored.State != v1.TaskStateCancelled {
			t.Fatalf("state = %q, want CANCELLED", stored.State)
		}
	})
}

func ptrState(state v1.TaskState) *v1.TaskState { return &state }

func ptrString(value string) *string { return &value }
