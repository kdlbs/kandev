package sqlite

import (
	"context"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestPostgresTaskStateCommitAndBatchCancel(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 4)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize PostgreSQL task repository: %v", err)
	}
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "state-commit-ws", Name: "State commit"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	for _, stmt := range []string{
		`ALTER TABLE tasks ADD COLUMN checkout_agent_id TEXT`,
		`ALTER TABLE tasks ADD COLUMN checkout_at TIMESTAMP`,
		`ALTER TABLE tasks ADD COLUMN checkout_run_id TEXT`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("add Office checkout column: %v", err)
		}
	}

	createTask := func(taskID string, state v1.TaskState) {
		t.Helper()
		if err := repo.CreateTask(ctx, &models.Task{
			ID: taskID, WorkspaceID: "state-commit-ws", Title: taskID, State: state, WIPAdmitted: true,
		}); err != nil {
			t.Fatalf("create task %s: %v", taskID, err)
		}
	}
	createTask("commit-postgres-single", v1.TaskStateTODO)
	stepID := ""
	expectedState := v1.TaskStateTODO
	change, err := repo.CommitTaskState(ctx, "commit-postgres-single", models.TaskStateWrite{
		State:                  v1.TaskStateInProgress,
		ExpectedWorkflowStepID: &stepID,
		ExpectedState:          &expectedState,
	})
	if err != nil || !change.Matched || !change.Changed || change.OldState != v1.TaskStateTODO {
		t.Fatalf("PostgreSQL state receipt = %+v, %v", change, err)
	}

	for _, taskID := range []string{"batch-postgres-root", "batch-postgres-child", "batch-postgres-skip"} {
		createTask(taskID, v1.TaskStateTODO)
	}
	if _, err := db.Exec(db.Rebind(`UPDATE tasks SET state = ? WHERE id = ?`), v1.TaskStateCancelled, "batch-postgres-skip"); err != nil {
		t.Fatalf("seed already-cancelled task: %v", err)
	}
	write := models.TaskStateBatchWrite{
		TaskIDs:                []string{"batch-postgres-root", "batch-postgres-child", "batch-postgres-skip"},
		State:                  v1.TaskStateCancelled,
		ReleaseCheckoutTaskIDs: []string{"batch-postgres-root", "batch-postgres-child", "batch-postgres-skip"},
	}
	if _, err := db.Exec(db.Rebind(`UPDATE tasks SET checkout_agent_id = 'agent', checkout_run_id = 'run', checkout_at = CURRENT_TIMESTAMP WHERE id IN (?, ?, ?)`),
		"batch-postgres-root", "batch-postgres-child", "batch-postgres-skip"); err != nil {
		t.Fatalf("seed checkout rows: %v", err)
	}
	receipts, err := repo.CommitTaskStateBatch(ctx, write)
	if err != nil {
		t.Fatalf("PostgreSQL CommitTaskStateBatch: %v", err)
	}
	if len(receipts) != 2 {
		t.Fatalf("batch receipts = %+v, want root and child only", receipts)
	}
	for _, taskID := range []string{"batch-postgres-root", "batch-postgres-child", "batch-postgres-skip"} {
		var agent, run string
		if err := db.QueryRow(db.Rebind(`SELECT COALESCE(checkout_agent_id, ''), COALESCE(checkout_run_id, '') FROM tasks WHERE id = ?`), taskID).
			Scan(&agent, &run); err != nil {
			t.Fatalf("read checkout %s: %v", taskID, err)
		}
		if agent != "" || run != "" {
			t.Fatalf("checkout %s = %q/%q, want released", taskID, agent, run)
		}
	}

	for _, taskID := range []string{"race-postgres-root", "race-postgres-child"} {
		createTask(taskID, v1.TaskStateTODO)
	}
	raceWrite := models.TaskStateBatchWrite{
		TaskIDs:                []string{"race-postgres-root", "race-postgres-child"},
		State:                  v1.TaskStateCancelled,
		ReleaseCheckoutTaskIDs: []string{"race-postgres-root", "race-postgres-child"},
	}
	start := make(chan struct{})
	results := make(chan []models.TaskStateChange, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	for range 2 {
		go func() {
			defer wg.Done()
			<-start
			committed, err := repo.CommitTaskStateBatch(ctx, raceWrite)
			results <- committed
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("competing PostgreSQL batch: %v", err)
		}
	}
	totalReceipts := 0
	for committed := range results {
		totalReceipts += len(committed)
	}
	if totalReceipts != 2 {
		t.Fatalf("competing batch receipts = %d, want exactly one per changed row", totalReceipts)
	}
}
