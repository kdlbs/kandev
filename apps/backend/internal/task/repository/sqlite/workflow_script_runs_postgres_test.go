package sqlite

import (
	"context"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresWorkflowScriptRunClaimAndLifecycle(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, title, created_at, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`), "task-pg-script", "postgres script"); err != nil {
		t.Fatalf("seed postgres task: %v", err)
	}

	first, claimed, err := repo.ClaimWorkflowScriptRun(ctx, newWorkflowScriptRun(
		"task-pg-script", "on_enter/entry-pg/step-pg/0", "printf postgres"))
	if err != nil || !claimed {
		t.Fatalf("postgres first claim = %v, %v", claimed, err)
	}
	duplicate := newWorkflowScriptRun("task-pg-script", first.OccurrenceKey, "printf edited")
	loaded, claimed, err := repo.ClaimWorkflowScriptRun(ctx, duplicate)
	if err != nil || claimed || loaded.ID != first.ID || loaded.Command != "printf postgres" {
		t.Fatalf("postgres duplicate claim = run=%+v claimed=%v err=%v", loaded, claimed, err)
	}
	if ok, err := repo.MarkWorkflowScriptRunStarting(ctx, first.ID, "message-pg-script"); err != nil || !ok {
		t.Fatalf("postgres mark starting = %v, %v", ok, err)
	}
	if ok, err := repo.MarkWorkflowScriptRunRunning(ctx, first.ID, "process-pg-script"); err != nil || !ok {
		t.Fatalf("postgres mark running = %v, %v", ok, err)
	}
	code := 0
	if ok, err := repo.CompleteWorkflowScriptRun(ctx, first.ID, models.WorkflowScriptRunCompletion{
		Status: models.WorkflowScriptRunSucceeded, ProcessID: "process-pg-script", ExitCode: &code,
		Output: "postgres output",
	}); err != nil || !ok {
		t.Fatalf("postgres complete = %v, %v", ok, err)
	}
	final, err := repo.GetWorkflowScriptRun(ctx, first.ID)
	if err != nil {
		t.Fatalf("reload postgres run: %v", err)
	}
	if final.Status != models.WorkflowScriptRunSucceeded || final.MessageID != "message-pg-script" || final.ProcessID != "process-pg-script" || final.ExitCode == nil || *final.ExitCode != 0 {
		t.Fatalf("postgres lifecycle mismatch: %+v", final)
	}
}

func TestPostgresWorkflowScriptRunConcurrentClaimHasOneWinner(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, title, created_at, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`), "task-pg-script-race", "postgres script race"); err != nil {
		t.Fatalf("seed postgres task: %v", err)
	}

	const concurrency = 8
	start := make(chan struct{})
	results := make(chan struct {
		run     *models.WorkflowScriptRun
		claimed bool
		err     error
	}, concurrency)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			run := newWorkflowScriptRun("task-pg-script-race", "on_exit/transition-pg/step-pg/0", "printf race")
			results <- func() (result struct {
				run     *models.WorkflowScriptRun
				claimed bool
				err     error
			}) {
				result.run, result.claimed, result.err = repo.ClaimWorkflowScriptRun(ctx, run)
				return result
			}()
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	var winnerID string
	winners := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("postgres concurrent claim: %v", result.err)
		}
		if winnerID == "" {
			winnerID = result.run.ID
		} else if result.run.ID != winnerID {
			t.Fatalf("postgres concurrent IDs differ: %q and %q", winnerID, result.run.ID)
		}
		if result.claimed {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("postgres concurrent claims produced %d winners, want 1", winners)
	}
}
