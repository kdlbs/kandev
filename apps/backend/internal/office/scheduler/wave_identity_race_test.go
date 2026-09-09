package scheduler

import (
	"context"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	runsservice "github.com/kandev/kandev/internal/runs/service"
)

// TestWaveIdentityRace_CascadeVsEngineRoutedPath_CollapsesToOneRun is
// Task 07's race test: P1's direct-insert path (SchedulerService.QueueRunCtx
// -> queueRun -> ss.repo.CreateRun, classified by
// runssqlite.IsWakeWaveUniqueViolation) races an engine-routed path
// (runsservice.Service.QueueRun -> insertRun -> CreateRunTx, classified by
// Task 02's own errWakeWaveKeyConflict handling) for the exact same wave
// identity and target agent, through two genuinely concurrent goroutines
// started together via a shared barrier channel — not a sequential call
// pair, which would only prove the classification logic works when
// invoked twice in a row on one goroutine. idx_run_wake_wave is the only
// thing that can make this deterministic: SQLite serializes the two
// INSERTs, but which of the two goroutines reaches it first is left to
// the Go scheduler, so the outcome asserted below must hold regardless
// of which side wins.
func TestWaveIdentityRace_CascadeVsEngineRoutedPath_CollapsesToOneRun(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	eb := bus.NewMemoryEventBus(log)
	runsSvc := runsservice.New(repo.RunsRepository(), eb, log, nil)

	const (
		waveKey    = "task_children_completed:parent-1:deadbeef"
		waveString = "parent-1|child-1,child-2"
	)
	ctx := context.Background()
	start := make(chan struct{})

	var wg sync.WaitGroup
	var cascadeErr error
	var engineOutcome runsservice.QueueOutcome
	var engineErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		cascadeErr = ss.QueueRunCtx(ctx, "agent-1", RunContext{
			Reason:         RunReasonTaskChildrenCompleted,
			TaskID:         "parent-1",
			IdempotencyKey: "task_children_completed:parent-1:agent-1:cascade",
			WaveKey:        waveKey,
			WaveString:     waveString,
		})
	}()
	go func() {
		defer wg.Done()
		<-start
		engineOutcome, engineErr = runsSvc.QueueRun(ctx, runsservice.QueueRunRequest{
			Reason:         RunReasonTaskChildrenCompleted,
			AgentProfileID: "agent-1",
			TaskID:         "parent-1",
			IdempotencyKey: "task_children_completed:parent-1:agent-1:engine",
			WakeWaveKey:    waveKey,
			WakeWaveString: waveString,
		})
	}()
	close(start)
	wg.Wait()

	if cascadeErr != nil {
		t.Fatalf("cascade (P1) path must never surface the losing side as an error: %v", cascadeErr)
	}
	if engineErr != nil {
		t.Fatalf("engine-routed path must never surface the losing side as an error: %v", engineErr)
	}
	if engineOutcome != runsservice.QueueOutcomeQueued && engineOutcome != runsservice.QueueOutcomeDeduped {
		t.Fatalf("engine-routed outcome = %q, want Queued or Deduped", engineOutcome)
	}

	if got := runsCountForReason(t, ss, RunReasonTaskChildrenCompleted); got != 1 {
		t.Fatalf("runs for wave %q = %d, want exactly 1 (the two producers must collapse to one row)", waveKey, got)
	}
}
