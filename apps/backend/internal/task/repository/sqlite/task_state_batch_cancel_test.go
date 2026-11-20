package sqlite

import (
	"context"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func addTaskCheckoutColumnsForBatchTests(t *testing.T, repo *Repository) {
	t.Helper()
	for _, stmt := range []string{
		`ALTER TABLE tasks ADD COLUMN checkout_agent_id TEXT`,
		`ALTER TABLE tasks ADD COLUMN checkout_at TIMESTAMP`,
		`ALTER TABLE tasks ADD COLUMN checkout_run_id TEXT`,
	} {
		if _, err := repo.db.Exec(stmt); err != nil {
			t.Fatalf("add checkout test column: %v", err)
		}
	}
}

func TestTaskStateBatchCancel(t *testing.T) {
	t.Run("commits all changed states and checkout cleanup with receipts", func(t *testing.T) {
		repo := newRepoForHealTests(t)
		addTaskCheckoutColumnsForBatchTests(t, repo)
		for _, taskID := range []string{"batch-root", "batch-child", "batch-skip"} {
			insertTask(t, repo.db, taskID)
		}
		if _, err := repo.db.Exec(`UPDATE tasks SET state = ? WHERE id = ?`, v1.TaskStateCancelled, "batch-skip"); err != nil {
			t.Fatalf("seed already-cancelled task: %v", err)
		}
		if _, err := repo.db.Exec(`UPDATE tasks SET checkout_agent_id = 'agent', checkout_at = CURRENT_TIMESTAMP, checkout_run_id = 'run'`); err != nil {
			t.Fatalf("seed checkouts: %v", err)
		}

		receipts, err := repo.CommitTaskStateBatch(context.Background(), models.TaskStateBatchWrite{
			TaskIDs:                []string{"batch-root", "batch-child", "batch-skip"},
			State:                  v1.TaskStateCancelled,
			ReleaseCheckoutTaskIDs: []string{"batch-root", "batch-child", "batch-skip"},
		})
		if err != nil {
			t.Fatalf("CommitTaskStateBatch: %v", err)
		}
		if len(receipts) != 2 {
			t.Fatalf("receipts = %+v, want only the two changed tasks", receipts)
		}
		for i, taskID := range []string{"batch-root", "batch-child"} {
			change := receipts[i]
			if !change.Matched || !change.Changed || change.OldState != v1.TaskStateTODO || change.Task == nil || change.Task.ID != taskID || change.Task.State != v1.TaskStateCancelled {
				t.Fatalf("receipt %d = %+v, want TODO -> CANCELLED for %s", i, change, taskID)
			}
		}
		for _, taskID := range []string{"batch-root", "batch-child", "batch-skip"} {
			var state string
			var checkoutAgent, checkoutRun string
			var checkoutAt any
			if err := repo.db.QueryRow(`SELECT state, COALESCE(checkout_agent_id, ''), COALESCE(checkout_run_id, ''), checkout_at FROM tasks WHERE id = ?`, taskID).
				Scan(&state, &checkoutAgent, &checkoutRun, &checkoutAt); err != nil {
				t.Fatalf("read %s: %v", taskID, err)
			}
			if state != string(v1.TaskStateCancelled) || checkoutAgent != "" || checkoutRun != "" || checkoutAt != nil {
				t.Fatalf("task %s = state %q checkout %q/%q/%v, want cancelled and released", taskID, state, checkoutAgent, checkoutRun, checkoutAt)
			}
		}
	})

	t.Run("rolls back every state when checkout cleanup fails", func(t *testing.T) {
		repo := newRepoForHealTests(t)
		addTaskCheckoutColumnsForBatchTests(t, repo)
		for _, taskID := range []string{"rollback-root", "rollback-child"} {
			insertTask(t, repo.db, taskID)
		}
		if _, err := repo.db.Exec(`CREATE TRIGGER fail_batch_checkout BEFORE UPDATE OF checkout_agent_id ON tasks
			BEGIN SELECT RAISE(ABORT, 'injected checkout failure'); END`); err != nil {
			t.Fatalf("create failure trigger: %v", err)
		}

		receipts, err := repo.CommitTaskStateBatch(context.Background(), models.TaskStateBatchWrite{
			TaskIDs:                []string{"rollback-root", "rollback-child"},
			State:                  v1.TaskStateCancelled,
			ReleaseCheckoutTaskIDs: []string{"rollback-root", "rollback-child"},
		})
		if err == nil {
			t.Fatal("CommitTaskStateBatch succeeded despite checkout failure")
		}
		if receipts != nil {
			t.Fatalf("receipts = %+v, want nil on rollback", receipts)
		}
		for _, taskID := range []string{"rollback-root", "rollback-child"} {
			var state string
			if err := repo.db.Get(&state, `SELECT state FROM tasks WHERE id = ?`, taskID); err != nil {
				t.Fatalf("read %s state: %v", taskID, err)
			}
			if state != "todo" {
				t.Fatalf("state %s = %q, want original todo after rollback", taskID, state)
			}
		}
	})

	t.Run("competing cancellation batches publish one receipt per changed task", func(t *testing.T) {
		repo := newRepoForHealTests(t)
		addTaskCheckoutColumnsForBatchTests(t, repo)
		for _, taskID := range []string{"race-root", "race-child"} {
			insertTask(t, repo.db, taskID)
		}
		write := models.TaskStateBatchWrite{
			TaskIDs:                []string{"race-root", "race-child"},
			State:                  v1.TaskStateCancelled,
			ReleaseCheckoutTaskIDs: []string{"race-root", "race-child"},
		}
		var wg sync.WaitGroup
		wg.Add(2)
		receipts := make(chan []models.TaskStateChange, 2)
		errs := make(chan error, 2)
		for range 2 {
			go func() {
				defer wg.Done()
				result, err := repo.CommitTaskStateBatch(context.Background(), write)
				receipts <- result
				errs <- err
			}()
		}
		wg.Wait()
		close(receipts)
		close(errs)
		total := 0
		for err := range errs {
			if err != nil {
				t.Fatalf("CommitTaskStateBatch: %v", err)
			}
		}
		for result := range receipts {
			total += len(result)
		}
		if total != 2 {
			t.Fatalf("combined receipts = %d, want one per changed task", total)
		}
	})
}
