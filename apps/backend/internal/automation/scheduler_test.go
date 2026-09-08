package automation

import (
	"context"
	"encoding/json"
	"go.uber.org/zap"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
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

	result, err := svc.FireTrigger(ctx, a.ID, trig.ID, TriggerTypeScheduled, json.RawMessage(`{}`), "scheduled:trig:1")
	if err != nil {
		t.Fatalf("FireTrigger returned error for a skip: %v", err)
	}
	// A skip must be reported as one. Callers that tell a human what happened
	// cannot distinguish it from a real fire otherwise.
	if !result.Skipped {
		t.Fatal("expected FireTrigger to report the concurrency-cap skip")
	}
	if result.Reason == "" {
		t.Fatal("expected a reason explaining the skip")
	}

	runs, err := svc.store.ListRuns(ctx, a.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	skipped := 0
	for _, r := range runs {
		if r.Status == RunStatusSkipped {
			skipped++
			if r.DedupKey != "scheduled:trig:1" {
				t.Errorf("scheduled cap skip dedup key = %q, want original key", r.DedupKey)
			}
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

func TestFireTriggerAdmitsRunBeforePublishing(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	a := &Automation{
		WorkspaceID:       "ws-1",
		Name:              "Daily report",
		TaskTitleTemplate: "Report: {{trigger.type}}",
		WorkflowID:        "wf-1",
		WorkflowStepID:    "s-1",
		Enabled:           true,
		MaxConcurrentRuns: 1,
	}
	if err := svc.store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	trig := &AutomationTrigger{AutomationID: a.ID, Type: TriggerTypeScheduled, Config: json.RawMessage(`{}`), Enabled: true}
	if err := svc.store.CreateTrigger(ctx, trig); err != nil {
		t.Fatal(err)
	}

	var publishedRun *AutomationRun
	if _, err := svc.eventBus.Subscribe(events.AutomationTriggered, func(ctx context.Context, event *bus.Event) error {
		evt, ok := event.Data.(*AutomationTriggeredEvent)
		if !ok {
			t.Fatalf("event data = %T, want *AutomationTriggeredEvent", event.Data)
		}
		if evt.RunID == "" {
			t.Fatal("published automation event has no admitted run ID")
		}
		var err error
		publishedRun, err = svc.store.GetRun(ctx, evt.RunID)
		if err != nil {
			t.Fatalf("load admitted run during publish: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	result, err := svc.FireTrigger(ctx, a.ID, trig.ID, TriggerTypeScheduled, json.RawMessage(`{}`), "scheduled:1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Skipped || result.RunID == "" {
		t.Fatalf("FireTrigger result = %+v, want an admitted run", result)
	}
	if publishedRun == nil {
		t.Fatal("event was not observed")
	}
	if publishedRun.Status != RunStatusTriggered {
		t.Fatalf("published run status = %q, want triggered", publishedRun.Status)
	}
	if publishedRun.DisplayTitle != "Report: scheduled" {
		t.Fatalf("display title = %q, want rendered trigger title", publishedRun.DisplayTitle)
	}
}

func TestFireTrigger_MergedPRConcurrencySkipLeavesDedupKeyEmpty(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	a := &Automation{
		WorkspaceID: "ws-merged-cap", Name: "Merged cleanup", WorkflowID: "wf-1", WorkflowStepID: "s-1",
		Enabled: true, MaxConcurrentRuns: 1,
	}
	if err := svc.store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	trig := &AutomationTrigger{
		AutomationID: a.ID, Type: TriggerTypeGitHubPRMerged,
		Config: json.RawMessage(`{"all_repos":true}`), Enabled: true,
	}
	if err := svc.store.CreateTrigger(ctx, trig); err != nil {
		t.Fatal(err)
	}
	if err := svc.store.CreateRun(ctx, &AutomationRun{
		AutomationID: a.ID, TriggerID: trig.ID, TriggerType: TriggerTypeGitHubPRMerged,
		Status: RunStatusTaskCreated, DedupKey: "active", TriggerData: json.RawMessage(`{}`),
	}); err != nil {
		t.Fatal(err)
	}

	const dedupKey = "pr_merged:task-1:acme/api#7"
	result, err := svc.FireTrigger(ctx, a.ID, trig.ID, TriggerTypeGitHubPRMerged, json.RawMessage(`{}`), dedupKey)
	if err != nil {
		t.Fatalf("FireTrigger returned error: %v", err)
	}
	if !result.Skipped {
		t.Fatal("expected merged-PR trigger to be skipped at the cap")
	}

	runs, err := svc.store.ListRuns(ctx, a.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	var skipped *AutomationRun
	for _, run := range runs {
		if run.Status == RunStatusSkipped {
			skipped = run
			break
		}
	}
	if skipped == nil {
		t.Fatalf("expected a skipped run, got %+v", runs)
	}
	if skipped.DedupKey != "" {
		t.Fatalf("merged-PR cap skip dedup key = %q, want empty", skipped.DedupKey)
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

// TestFireTrigger_ConcurrencyCapCheckError_DoesNotAdvanceLastEvaluatedAt guards
// against silently missing a whole day of a @daily trigger's schedule. If
// maybeSkipForConcurrencyCap fails for an infrastructural reason (e.g. the
// reader pool backing CountActiveRuns is briefly unavailable), FireTrigger
// must return the error without having already advanced LastEvaluatedAt.
// Otherwise CronScheduler.shouldFire treats the trigger as freshly
// evaluated and suppresses the next attempt until the full cron interval
// elapses, instead of retrying on the next 30s scheduler tick.
func TestFireTrigger_ConcurrencyCapCheckError_DoesNotAdvanceLastEvaluatedAt(t *testing.T) {
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

	// Break CountActiveRuns in isolation: it queries automation_runs while
	// GetAutomation (the earlier lookup in maybeSkipForConcurrencyCap) only
	// queries automations, so dropping just the former reproduces "the cap
	// check itself failed" without masking the failure behind an earlier,
	// softly-handled lookup error.
	if _, err := svc.store.db.ExecContext(ctx, `DROP TABLE automation_runs`); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.FireTrigger(ctx, a.ID, trig.ID, TriggerTypeScheduled, json.RawMessage(`{}`), ""); err == nil {
		t.Fatal("expected FireTrigger to return the concurrency-cap check error")
	}

	triggers, err := svc.store.ListTriggers(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(triggers) != 1 {
		t.Fatalf("expected 1 trigger, got %d", len(triggers))
	}
	if triggers[0].LastEvaluatedAt != nil {
		t.Fatal("expected LastEvaluatedAt to remain nil after a concurrency-cap check error, got non-nil")
	}
}

// TestFireTrigger_ArchivedTaskRun_DoesNotBlockConcurrencyCap reproduces the
// reported bug end to end: an automation-generated task that gets archived
// (manually, via auto-archive, or via cascade) before its run is otherwise
// finalized left the run stuck at task_created forever, permanently pinning
// max_concurrent_runs=1 automations at their cap with no way to schedule
// again. A run whose task is archived no longer represents outstanding
// work and must not count toward the cap.
func TestFireTrigger_ArchivedTaskRun_DoesNotBlockConcurrencyCap(t *testing.T) {
	svc := newTestService(t)
	createTasksTable(t, svc.store)
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

	insertTask(t, svc.store, "task-archived", true)
	if err := svc.store.CreateRun(ctx, &AutomationRun{
		AutomationID: a.ID,
		TriggerID:    trig.ID,
		TriggerType:  TriggerTypeScheduled,
		Status:       RunStatusTaskCreated,
		TaskID:       "task-archived",
		DedupKey:     "prior-run",
		TriggerData:  json.RawMessage(`{}`),
	}); err != nil {
		t.Fatal(err)
	}

	result, err := svc.FireTrigger(ctx, a.ID, trig.ID, TriggerTypeScheduled, json.RawMessage(`{}`), "new-run")
	if err != nil {
		t.Fatalf("FireTrigger returned error: %v", err)
	}
	if result.Skipped {
		t.Fatalf("expected the trigger to fire, got skipped: %s", result.Reason)
	}

	runs, err := svc.store.ListRuns(ctx, a.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range runs {
		if r.Status == RunStatusSkipped {
			t.Fatalf("expected the new trigger to fire instead of skip for the cap; runs=%+v", runs)
		}
	}
}
