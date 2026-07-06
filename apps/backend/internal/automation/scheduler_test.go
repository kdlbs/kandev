package automation

import (
	"context"
	"encoding/json"
	"go.uber.org/zap"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

// TestFireTrigger_SkippedForConcurrencyCap_UpdatesLastEvaluatedAt guards
// against the scheduler retrying a scheduled trigger on every check tick
// once max_concurrent_runs is reached. FireTrigger must record that the
// trigger was evaluated even when the run itself is skipped, otherwise
// CronScheduler.shouldFire sees a stale LastEvaluatedAt forever and fires
// again on the very next tick instead of waiting out the configured
// interval.
func TestFireTrigger_SkippedForConcurrencyCap_UpdatesLastEvaluatedAt(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	a := &Automation{
		WorkspaceID:       "ws-1",
		Name:              "Daily rebase",
		WorkflowID:        "wf-1",
		WorkflowStepID:    "s-1",
		Enabled:           true,
		MaxConcurrentRuns: 1,
	}
	if err := svc.store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}

	cfg, _ := json.Marshal(ScheduledTriggerConfig{CronExpression: "@daily"})
	trig := &AutomationTrigger{AutomationID: a.ID, Type: TriggerTypeScheduled, Config: cfg, Enabled: true}
	if err := svc.store.CreateTrigger(ctx, trig); err != nil {
		t.Fatal(err)
	}

	// Seed an already-active run so the automation sits at its concurrency cap.
	active := &AutomationRun{
		AutomationID: a.ID,
		TriggerID:    trig.ID,
		TriggerType:  TriggerTypeScheduled,
		Status:       RunStatusTaskCreated,
		DedupKey:     "seed-active-run",
		TriggerData:  json.RawMessage(`{}`),
	}
	if err := svc.store.CreateRun(ctx, active); err != nil {
		t.Fatal(err)
	}

	if err := svc.FireTrigger(ctx, a.ID, trig.ID, TriggerTypeScheduled, json.RawMessage(`{}`), "scheduled:trig:1"); err != nil {
		t.Fatalf("FireTrigger returned error for a skip: %v", err)
	}

	runs, err := svc.store.ListRuns(ctx, a.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	skipped := 0
	for _, r := range runs {
		if r.Status == RunStatusSkipped {
			skipped++
		}
	}
	if skipped != 1 {
		t.Fatalf("expected 1 skipped run, got %d (runs=%+v)", skipped, runs)
	}

	triggers, err := svc.store.ListTriggers(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(triggers) != 1 {
		t.Fatalf("expected 1 trigger, got %d", len(triggers))
	}
	if triggers[0].LastEvaluatedAt == nil {
		t.Fatal("expected LastEvaluatedAt to be set after a concurrency-cap skip, got nil")
	}
}

// TestCronScheduler_DailyTrigger_DoesNotRefireNextTick reproduces the
// reported bug end to end: a daily scheduled trigger that gets skipped for
// max_concurrent_runs must not be considered due again on the scheduler's
// next check tick (30s later in production). Before the fix, a stale
// LastEvaluatedAt made shouldFire return true on every tick, spamming a new
// "max_concurrent_runs=n reached" skipped run roughly once a minute forever.
func TestCronScheduler_DailyTrigger_DoesNotRefireNextTick(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	log, _ := logger.NewFromZap(zap.NewNop())
	cs := NewCronScheduler(svc, log)

	a := &Automation{
		WorkspaceID:       "ws-1",
		Name:              "Daily rebase",
		WorkflowID:        "wf-1",
		WorkflowStepID:    "s-1",
		Enabled:           true,
		MaxConcurrentRuns: 1,
	}
	if err := svc.store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}

	cfg, _ := json.Marshal(ScheduledTriggerConfig{CronExpression: "@daily"})
	trig := &AutomationTrigger{AutomationID: a.ID, Type: TriggerTypeScheduled, Config: cfg, Enabled: true}
	if err := svc.store.CreateTrigger(ctx, trig); err != nil {
		t.Fatal(err)
	}

	active := &AutomationRun{
		AutomationID: a.ID,
		TriggerID:    trig.ID,
		TriggerType:  TriggerTypeScheduled,
		Status:       RunStatusTaskCreated,
		DedupKey:     "seed-active-run",
		TriggerData:  json.RawMessage(`{}`),
	}
	if err := svc.store.CreateRun(ctx, active); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	cs.fire(ctx, trig, now)

	triggers, err := svc.store.ListTriggers(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(triggers) != 1 {
		t.Fatalf("expected 1 trigger, got %d", len(triggers))
	}
	updated := triggers[0]

	// One minute later — well within the @daily interval — the scheduler
	// must not treat the trigger as due again.
	if cs.shouldFire(&updated, now.Add(time.Minute)) {
		t.Fatal("expected shouldFire to be false one minute after a skipped evaluation of a daily trigger")
	}
}
