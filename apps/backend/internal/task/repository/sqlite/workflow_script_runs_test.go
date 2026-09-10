package sqlite

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func seedWorkflowScriptRunTask(t *testing.T, repo *Repository, taskID string) {
	t.Helper()
	if err := repo.CreateTask(context.Background(), &models.Task{
		ID: taskID, WorkspaceID: "ws-script-runs", WorkflowID: "wf-script-runs",
		WorkflowStepID: "step-script-runs", Title: "script run test", Priority: "medium",
	}); err != nil {
		t.Fatalf("seed script run task: %v", err)
	}
}

func newWorkflowScriptRun(taskID, occurrence, command string) *models.WorkflowScriptRun {
	return &models.WorkflowScriptRun{
		OccurrenceKey:  occurrence,
		TaskID:         taskID,
		WorkflowID:     "wf-script-runs",
		WorkflowStepID: "step-script-runs",
		Trigger:        models.WorkflowScriptRunTriggerOnEnter,
		ActionPosition: 0,
		SessionID:      "session-script-runs",
		ExecutionID:    "execution-script-runs",
		Command:        command,
		TimeoutSeconds: 600,
		FailurePolicy:  "block",
	}
}

func TestWorkflowScriptRunsSchemaIsCreatedOnRepositoryBoot(t *testing.T) {
	repo := newRepoForSessionTests(t)

	var tableName string
	err := repo.ro.QueryRowContext(context.Background(), `
		SELECT name
		FROM sqlite_master
		WHERE type = 'table' AND name = 'workflow_script_runs'
	`).Scan(&tableName)
	if err != nil {
		t.Fatalf("workflow_script_runs table is missing: %v", err)
	}
	if tableName != "workflow_script_runs" {
		t.Fatalf("table name = %q, want workflow_script_runs", tableName)
	}
}

func TestWorkflowScriptRunClaimReturnsOneImmutableWinner(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkflowScriptRunTask(t, repo, "task-script-claim")

	first := newWorkflowScriptRun("task-script-claim", "on_enter/entry-1/step-1/0", "printf first")
	winner, claimed, err := repo.ClaimWorkflowScriptRun(ctx, first)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if !claimed || winner.ID == "" || winner.ProcessRequestID != winner.ID {
		t.Fatalf("first claim = claimed %v, winner %+v; want new run with stable request ID", claimed, winner)
	}

	duplicate := newWorkflowScriptRun("task-script-claim", first.OccurrenceKey, "printf changed")
	duplicate.FailurePolicy = "continue"
	duplicate.TimeoutSeconds = 12
	loaded, claimed, err := repo.ClaimWorkflowScriptRun(ctx, duplicate)
	if err != nil {
		t.Fatalf("duplicate claim: %v", err)
	}
	if claimed {
		t.Fatal("duplicate claim reported a second winner")
	}
	if loaded.ID != winner.ID || loaded.Command != "printf first" || loaded.FailurePolicy != "block" || loaded.TimeoutSeconds != 600 {
		t.Fatalf("duplicate claim changed immutable snapshot: %+v", loaded)
	}
}

func TestWorkflowScriptRunClaimIsAtomicUnderConcurrentDuplicates(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkflowScriptRunTask(t, repo, "task-script-race")
	const concurrency = 12

	start := make(chan struct{})
	results := make(chan struct {
		run     *models.WorkflowScriptRun
		claimed bool
		err     error
	}, concurrency)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			run := newWorkflowScriptRun("task-script-race", "on_turn_complete/turn-1/step-1/0", "printf variant")
			run.ID = "candidate-" + string(rune('a'+i))
			results <- func() (result struct {
				run     *models.WorkflowScriptRun
				claimed bool
				err     error
			}) {
				result.run, result.claimed, result.err = repo.ClaimWorkflowScriptRun(ctx, run)
				return result
			}()
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)

	var winnerID string
	winners := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent claim: %v", result.err)
		}
		if result.run == nil {
			t.Fatal("concurrent claim returned nil run")
		}
		if winnerID == "" {
			winnerID = result.run.ID
		} else if result.run.ID != winnerID {
			t.Fatalf("concurrent claims returned different IDs: %q and %q", winnerID, result.run.ID)
		}
		if result.claimed {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("concurrent claims produced %d winners, want 1", winners)
	}
}

func TestWorkflowScriptRunAdmissionCompletionAndInterruptionSurviveReload(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkflowScriptRunTask(t, repo, "task-script-lifecycle")

	run, claimed, err := repo.ClaimWorkflowScriptRun(ctx, newWorkflowScriptRun(
		"task-script-lifecycle", "on_enter/entry-lifecycle/step-1/0", "printf lifecycle"))
	if err != nil || !claimed {
		t.Fatalf("claim = %v, %v", claimed, err)
	}
	started, err := repo.MarkWorkflowScriptRunStarting(ctx, run.ID, "message-script-1")
	if err != nil || !started {
		t.Fatalf("mark starting = %v, %v", started, err)
	}
	reloaded, err := repo.GetWorkflowScriptRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("reload starting run: %v", err)
	}
	if reloaded.Status != models.WorkflowScriptRunStarting || reloaded.MessageID != "message-script-1" || reloaded.AdmissionAttemptedAt == nil || reloaded.StartedAt == nil {
		t.Fatalf("admission state not durable: %+v", reloaded)
	}

	running, err := repo.MarkWorkflowScriptRunRunning(ctx, run.ID, "process-script-1")
	if err != nil || !running {
		t.Fatalf("mark running = %v, %v", running, err)
	}
	code := 124
	completed, err := repo.CompleteWorkflowScriptRun(ctx, run.ID, models.WorkflowScriptRunCompletion{
		Status: models.WorkflowScriptRunTimedOut, ProcessID: "process-script-1", ExitCode: &code,
		Output: "partial output", OutputTruncated: true, FailureReason: "deadline exceeded",
	})
	if err != nil || !completed {
		t.Fatalf("complete = %v, %v", completed, err)
	}
	completed, err = repo.CompleteWorkflowScriptRun(ctx, run.ID, models.WorkflowScriptRunCompletion{
		Status: models.WorkflowScriptRunSucceeded, ProcessID: "process-script-other",
		Output: "replacement",
	})
	if err != nil {
		t.Fatalf("duplicate completion: %v", err)
	}
	if completed {
		t.Fatal("terminal run accepted a second completion")
	}

	final, err := repo.GetWorkflowScriptRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("reload terminal run: %v", err)
	}
	if final.Status != models.WorkflowScriptRunTimedOut || final.ProcessID != "process-script-1" || final.ExitCode == nil || *final.ExitCode != code || final.Output != "partial output" || !final.OutputTruncated || final.FailureReason != "deadline exceeded" || final.CompletedAt == nil {
		t.Fatalf("terminal result not durable: %+v", final)
	}

	interruptRun, claimed, err := repo.ClaimWorkflowScriptRun(ctx, newWorkflowScriptRun(
		"task-script-lifecycle", "on_exit/transition-lifecycle/step-1/1", "printf interrupt"))
	if err != nil || !claimed {
		t.Fatalf("interrupt claim = %v, %v", claimed, err)
	}
	if ok, err := repo.MarkWorkflowScriptRunStarting(ctx, interruptRun.ID, "message-script-2"); err != nil || !ok {
		t.Fatalf("interrupt mark starting = %v, %v", ok, err)
	}
	interrupted, err := repo.InterruptWorkflowScriptRuns(ctx, "interrupted by restart")
	if err != nil || interrupted != 1 {
		t.Fatalf("interrupt active runs = %d, %v; want 1", interrupted, err)
	}
	got, err := repo.GetWorkflowScriptRun(ctx, interruptRun.ID)
	if err != nil {
		t.Fatalf("reload interrupted run: %v", err)
	}
	if got.Status != models.WorkflowScriptRunInterrupted || got.FailureReason != "interrupted by restart" || got.CompletedAt == nil {
		t.Fatalf("interruption not durable: %+v", got)
	}
	if ok, err := repo.MarkWorkflowScriptRunStarting(ctx, interruptRun.ID, "message-script-retry"); err != nil || ok {
		t.Fatalf("interrupted run was admitted again: ok=%v err=%v", ok, err)
	}

	active, err := repo.ListNonTerminalWorkflowScriptRuns(ctx)
	if err != nil {
		t.Fatalf("list non-terminal runs: %v", err)
	}
	for _, candidate := range active {
		if candidate.ID == run.ID || candidate.ID == interruptRun.ID {
			t.Fatalf("terminal run returned as active: %+v", candidate)
		}
	}
}

func TestWorkflowScriptRunWorkflowEditCannotChangeClaimedSnapshot(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkflowScriptRunTask(t, repo, "task-script-edit")

	run := newWorkflowScriptRun("task-script-edit", "on_exit/transition-edit/step-1/0", "printf original")
	claimed, inserted, err := repo.ClaimWorkflowScriptRun(ctx, run)
	if err != nil || !inserted {
		t.Fatalf("claim = %v, %v", inserted, err)
	}
	claimed.Command = "printf workflow-edited"
	claimed.FailurePolicy = "continue"
	claimed.TimeoutSeconds = 1
	if ok, err := repo.MarkWorkflowScriptRunStarting(ctx, claimed.ID, "message-edit"); err != nil || !ok {
		t.Fatalf("mark starting after edit = %v, %v", ok, err)
	}
	loaded, err := repo.GetWorkflowScriptRun(ctx, claimed.ID)
	if err != nil {
		t.Fatalf("reload after workflow edit: %v", err)
	}
	if loaded.Command != "printf original" || loaded.FailurePolicy != "block" || loaded.TimeoutSeconds != 600 {
		t.Fatalf("workflow edit mutated durable action snapshot: %+v", loaded)
	}
}

func TestWorkflowScriptRunValidationAndTaskRetention(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	_, _, err := repo.ClaimWorkflowScriptRun(ctx, &models.WorkflowScriptRun{})
	if !errors.Is(err, models.ErrWorkflowScriptRunInvalid) {
		t.Fatalf("invalid claim error = %v, want ErrWorkflowScriptRunInvalid", err)
	}

	seedWorkflowScriptRunTask(t, repo, "task-script-retention")
	run, claimed, err := repo.ClaimWorkflowScriptRun(ctx, newWorkflowScriptRun(
		"task-script-retention", "on_turn_complete/turn-retention/step-1/0", "printf retention"))
	if err != nil || !claimed {
		t.Fatalf("retention claim = %v, %v", claimed, err)
	}
	if err := repo.db.QueryRowContext(ctx, repo.db.Rebind(`SELECT COUNT(*) FROM workflow_script_runs WHERE id = ?`), run.ID).Scan(new(int)); err != nil {
		t.Fatalf("count retained run: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`DELETE FROM tasks WHERE id = ?`), "task-script-retention"); err != nil {
		t.Fatalf("delete task: %v", err)
	}
	if _, err := repo.GetWorkflowScriptRun(ctx, run.ID); !errors.Is(err, models.ErrWorkflowScriptRunNotFound) {
		t.Fatalf("run after task deletion = %v, want not found", err)
	}
}

func TestWorkflowScriptOccurrenceKeyIncludesTriggerStepAndActionPosition(t *testing.T) {
	entry := models.NewWorkflowScriptOccurrenceKey(models.WorkflowScriptRunTriggerOnEnter, "entry-1", "step-a", 0)
	turn := models.NewWorkflowScriptOccurrenceKey(models.WorkflowScriptRunTriggerOnTurnComplete, "turn-1", "step-a", 0)
	nextAction := models.NewWorkflowScriptOccurrenceKey(models.WorkflowScriptRunTriggerOnEnter, "entry-1", "step-a", 1)
	if entry == turn || entry == nextAction || turn == nextAction {
		t.Fatalf("occurrence keys are not distinct: entry=%q turn=%q next=%q", entry, turn, nextAction)
	}
	if entry == "" || !strings.Contains(entry, "entry-1") {
		t.Fatalf("occurrence key lost occurrence identity: %q", entry)
	}
}
