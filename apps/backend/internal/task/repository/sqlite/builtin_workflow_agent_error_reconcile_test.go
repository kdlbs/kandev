package sqlite

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// seedStaleOfficeStepWithoutAgentError inserts a system-owned office-default
// workflow with a single named step whose events have no on_agent_error
// action, simulating a workspace materialized before WO-05-2 shipped.
func seedStaleOfficeStepWithoutAgentError(t *testing.T, repo *Repository, workflowID, stepID, stepName, stageType string) {
	t.Helper()
	ctx := context.Background()
	legacyTime := time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)

	_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workflows (
			id, workspace_id, name, workflow_template_id, is_system, hidden, created_at, updated_at
		) VALUES (?, 'ws-1', 'Office', 'office-default', 1, 1, ?, ?)
	`), workflowID, legacyTime, legacyTime)
	if err != nil {
		t.Fatalf("insert stale office workflow: %v", err)
	}
	staleEvents := `{"on_turn_complete":[{"type":"move_to_step","config":{"step_id":"done"}}]}`
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workflow_steps (
			id, workflow_id, name, position, stage_type, events, auto_advance_requires_signal, created_at, updated_at
		) VALUES (?, ?, ?, 2, ?, ?, 0, ?, ?)
	`), stepID, workflowID, stepName, stageType, staleEvents, legacyTime, legacyTime)
	if err != nil {
		t.Fatalf("insert stale %s step: %v", stepName, err)
	}
}

func loadStepEvents(t *testing.T, repo *Repository, stepID string) *wfmodels.WorkflowStep {
	t.Helper()
	steps := loadStepsForWorkflowByID(t, repo, stepID)
	if len(steps) != 1 {
		t.Fatalf("expected exactly one step row for id %q, got %d", stepID, len(steps))
	}
	return steps[0]
}

func TestHealBuiltinWorkflowStepOnAgentError_InsertsEscalation(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)

	seedStaleOfficeStepWithoutAgentError(t, repo, "stale-office-agent-err", "stale-office-agent-err-review", "Review", "review")

	if err := repo.healBuiltinWorkflowStepOnAgentError(); err != nil {
		t.Fatalf("healBuiltinWorkflowStepOnAgentError: %v", err)
	}

	step := loadStepEvents(t, repo, "stale-office-agent-err-review")
	if len(step.Events.OnAgentError) != 1 {
		t.Fatalf("Review.on_agent_error len = %d, want 1: %+v", len(step.Events.OnAgentError), step.Events.OnAgentError)
	}
	action := step.Events.OnAgentError[0]
	if action.Type != wfmodels.GenericActionQueueRun {
		t.Errorf("on_agent_error[0].Type = %q, want queue_run", action.Type)
	}
	if target, _ := action.Config["target"].(string); target != "workspace.ceo_agent" {
		t.Errorf("on_agent_error[0] target = %q, want workspace.ceo_agent", target)
	}
	if reason, _ := action.Config["reason"].(string); reason != "agent_error" {
		t.Errorf("on_agent_error[0] reason = %q, want agent_error", reason)
	}
	// Existing actions on other triggers must survive reconciliation.
	if len(step.Events.OnTurnComplete) != 1 {
		t.Errorf("on_turn_complete was modified by the reconciler: %+v", step.Events.OnTurnComplete)
	}
}

func TestHealBuiltinWorkflowStepOnAgentError_Idempotent(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)

	seedStaleOfficeStepWithoutAgentError(t, repo, "stale-office-agent-err-idem", "stale-office-agent-err-idem-review", "Review", "review")

	if err := repo.healBuiltinWorkflowStepOnAgentError(); err != nil {
		t.Fatalf("first heal: %v", err)
	}
	first := loadStepEvents(t, repo, "stale-office-agent-err-idem-review")

	if err := repo.healBuiltinWorkflowStepOnAgentError(); err != nil {
		t.Fatalf("second heal: %v", err)
	}
	second := loadStepEvents(t, repo, "stale-office-agent-err-idem-review")

	if len(second.Events.OnAgentError) != len(first.Events.OnAgentError) {
		t.Fatalf("second heal changed on_agent_error length: %d -> %d", len(first.Events.OnAgentError), len(second.Events.OnAgentError))
	}
	if len(second.Events.OnAgentError) != 1 {
		t.Errorf("running the reconciler twice produced %d escalation actions, want exactly 1", len(second.Events.OnAgentError))
	}
}

func TestHealBuiltinWorkflowStepOnAgentError_KeepsUserWorkflowUntouched(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()
	legacyTime := time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)

	_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workflows (
			id, workspace_id, name, workflow_template_id, is_system, hidden, created_at, updated_at
		) VALUES ('user-office-agent-err', 'ws-1', 'My Office', 'office-default', 0, 0, ?, ?)
	`), legacyTime, legacyTime)
	if err != nil {
		t.Fatalf("insert user office workflow: %v", err)
	}
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workflow_steps (
			id, workflow_id, name, position, stage_type, events, auto_advance_requires_signal, created_at, updated_at
		) VALUES ('user-office-agent-err-review', 'user-office-agent-err', 'Review', 2, 'review', '{}', 0, ?, ?)
	`), legacyTime, legacyTime)
	if err != nil {
		t.Fatalf("insert user review step: %v", err)
	}

	if err := repo.healBuiltinWorkflowStepOnAgentError(); err != nil {
		t.Fatalf("healBuiltinWorkflowStepOnAgentError: %v", err)
	}

	step := loadStepEvents(t, repo, "user-office-agent-err-review")
	if len(step.Events.OnAgentError) != 0 {
		t.Errorf("user-customised (is_system=0) workflow step was modified: on_agent_error = %+v", step.Events.OnAgentError)
	}
}

func TestRepositoryInitialization_HealsBuiltinWorkflowStepOnAgentError(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)

	seedStaleOfficeStepWithoutAgentError(t, repo, "stale-office-boot-agent-err", "stale-office-boot-agent-err-review", "Review", "review")

	if err := repo.initSchema(); err != nil {
		t.Fatalf("reinitialize repository: %v", err)
	}

	step := loadStepEvents(t, repo, "stale-office-boot-agent-err-review")
	if len(step.Events.OnAgentError) != 1 {
		t.Error("initSchema did not reconcile the on_agent_error action onto a stale Review step")
	}
}

func TestHealAgentErrorRowWithRetry_ExhaustsRetriesWithoutBlockingStartup(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	core, logs := observer.New(zap.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("logger.NewFromZap: %v", err)
	}
	repo.log = log

	seedStaleOfficeStepWithoutAgentError(t, repo, "stale-office-agent-err-retry-exhaust", "stale-office-agent-err-retry-exhaust-review", "Review", "review")
	repo.failAgentErrorReconcileAttempts = maxAgentErrorReconcileAttempts + 5

	action := wfmodels.GenericAction{
		Type:   wfmodels.GenericActionQueueRun,
		Config: map[string]interface{}{"target": "workspace.ceo_agent", "task_id": "this", "reason": "agent_error"},
	}
	err = repo.healAgentErrorRowWithRetry("stale-office-agent-err-retry-exhaust-review", "stale-office-agent-err-retry-exhaust", "office-default", "Review", action)
	if err != nil {
		t.Fatalf("healAgentErrorRowWithRetry returned an error; retry exhaustion must never block startup: %v", err)
	}

	step := loadStepEvents(t, repo, "stale-office-agent-err-retry-exhaust-review")
	if len(step.Events.OnAgentError) != 0 {
		t.Error("step was modified despite every attempt reporting a concurrent-modification retry")
	}
	if logs.Len() != 1 {
		t.Fatalf("expected exactly one warning record after retry exhaustion, got %d", logs.Len())
	}
	fields := logs.All()[0].ContextMap()
	if fields["step_name"] != "Review" || fields["target"] != "workspace.ceo_agent" {
		t.Errorf("warning fields = %+v, want step_name=Review target=workspace.ceo_agent", fields)
	}
}

func TestHealAgentErrorRowWithRetry_SucceedsAfterTransientRetries(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)

	seedStaleOfficeStepWithoutAgentError(t, repo, "stale-office-agent-err-retry-ok", "stale-office-agent-err-retry-ok-review", "Review", "review")
	repo.failAgentErrorReconcileAttempts = maxAgentErrorReconcileAttempts - 1

	action := wfmodels.GenericAction{
		Type:   wfmodels.GenericActionQueueRun,
		Config: map[string]interface{}{"target": "workspace.ceo_agent", "task_id": "this", "reason": "agent_error"},
	}
	if err := repo.healAgentErrorRowWithRetry("stale-office-agent-err-retry-ok-review", "stale-office-agent-err-retry-ok", "office-default", "Review", action); err != nil {
		t.Fatalf("healAgentErrorRowWithRetry: %v", err)
	}

	step := loadStepEvents(t, repo, "stale-office-agent-err-retry-ok-review")
	if len(step.Events.OnAgentError) != 1 {
		t.Error("expected the escalation action to be inserted once transient retries were exhausted before the attempt budget")
	}
}

func TestHasAgentErrorEscalation(t *testing.T) {
	actions := []wfmodels.GenericAction{
		{Type: wfmodels.GenericActionQueueRun, Config: map[string]interface{}{"target": "workspace.ceo_agent", "reason": "agent_error"}},
	}
	if !hasAgentErrorEscalation(actions, wfmodels.GenericAction{
		Type:   wfmodels.GenericActionQueueRun,
		Config: map[string]interface{}{"target": "workspace.ceo_agent", "reason": "agent_error"},
	}) {
		t.Error("hasAgentErrorEscalation did not find an equivalent queue_run action")
	}
	if hasAgentErrorEscalation(actions, wfmodels.GenericAction{
		Type:   wfmodels.GenericActionQueueRun,
		Config: map[string]interface{}{"target": "primary", "reason": "agent_error"},
	}) {
		t.Error("hasAgentErrorEscalation treated a different target as equivalent")
	}
}

func TestHasAgentErrorEscalation_NormalizesDefaultsAndDistinguishesActionConfig(t *testing.T) {
	actions := []wfmodels.GenericAction{
		{
			Type: wfmodels.GenericActionQueueRun,
			Config: map[string]interface{}{
				"target":  "primary",
				"task_id": "this",
				"reason":  "agent_error",
				"payload": map[string]interface{}{"source": "template"},
			},
		},
	}

	if !hasAgentErrorEscalation(actions, wfmodels.GenericAction{
		Type:   wfmodels.GenericActionQueueRun,
		Config: map[string]interface{}{"reason": "agent_error", "payload": map[string]interface{}{"source": "template"}},
	}) {
		t.Error("hasAgentErrorEscalation did not normalize omitted target and task_id defaults")
	}
	if hasAgentErrorEscalation(actions, wfmodels.GenericAction{
		Type:   wfmodels.GenericActionQueueRun,
		Config: map[string]interface{}{"target": "primary", "task_id": "child", "reason": "agent_error", "payload": map[string]interface{}{"source": "template"}},
	}) {
		t.Error("hasAgentErrorEscalation ignored a distinct task_id")
	}
	if hasAgentErrorEscalation(actions, wfmodels.GenericAction{
		Type:   wfmodels.GenericActionQueueRun,
		Config: map[string]interface{}{"target": "primary", "task_id": "this", "reason": "agent_error", "payload": map[string]interface{}{"source": "other"}},
	}) {
		t.Error("hasAgentErrorEscalation ignored a distinct payload")
	}
}

func TestTemplateAgentErrorActions_DedupesByNormalizedFullAction(t *testing.T) {
	step := wfmodels.StepDefinition{
		Events: wfmodels.StepEvents{
			OnAgentError: []wfmodels.GenericAction{
				{Type: wfmodels.GenericActionQueueRun, Config: map[string]interface{}{"target": "workspace.ceo_agent", "task_id": "this", "reason": "agent_error", "payload": map[string]interface{}{"mode": "one"}}},
				{Type: wfmodels.GenericActionQueueRun, Config: map[string]interface{}{"target": "workspace.ceo_agent", "task_id": "this", "reason": "agent_error", "payload": map[string]interface{}{"mode": "one"}}},
				{Type: wfmodels.GenericActionQueueRun, Config: map[string]interface{}{"target": "workspace.ceo_agent", "task_id": "child", "reason": "agent_error", "payload": map[string]interface{}{"mode": "one"}}},
				{Type: wfmodels.GenericActionQueueRun, Config: map[string]interface{}{"target": "workspace.ceo_agent", "task_id": "this", "reason": "agent_error", "payload": map[string]interface{}{"mode": "two"}}},
				{Type: wfmodels.GenericActionQueueRun, Config: map[string]interface{}{"reason": "agent_error"}},
				{Type: wfmodels.GenericActionQueueRun, Config: map[string]interface{}{"target": "primary", "task_id": "this", "reason": "agent_error"}},
				{Type: wfmodels.GenericActionMoveToStep, Config: map[string]interface{}{"step_id": "done"}},
			},
		},
	}
	actions := templateAgentErrorActions(step)
	if len(actions) != 4 {
		t.Fatalf("templateAgentErrorActions len = %d, want 4: %+v", len(actions), actions)
	}
	if target, _ := actions[3].Config["target"].(string); target != "" {
		t.Errorf("templateAgentErrorActions kept the default-target declaration as target %q, want it unchanged", target)
	}
}

func TestWarnNonQueueRunAgentErrorActions_LogsSkippedActionTypes(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	core, logs := observer.New(zap.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("logger.NewFromZap: %v", err)
	}
	repo.log = log

	step := wfmodels.StepDefinition{
		Name: "Review",
		Events: wfmodels.StepEvents{
			OnAgentError: []wfmodels.GenericAction{
				{Type: wfmodels.GenericActionQueueRun, Config: map[string]interface{}{"target": "workspace.ceo_agent", "reason": "agent_error"}},
				{Type: wfmodels.GenericActionMoveToStep, Config: map[string]interface{}{"step_id": "done"}},
			},
		},
	}
	repo.warnNonQueueRunAgentErrorActions("office-default", step)

	if logs.Len() != 1 {
		t.Fatalf("expected exactly one warning for the non-queue_run action, got %d", logs.Len())
	}
	fields := logs.All()[0].ContextMap()
	if fields["step_name"] != "Review" || fields["action_type"] != string(wfmodels.GenericActionMoveToStep) {
		t.Errorf("warning fields = %+v, want step_name=Review action_type=%s", fields, wfmodels.GenericActionMoveToStep)
	}
}

func TestWarnNonQueueRunAgentErrorActions_NilLoggerNoop(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	step := wfmodels.StepDefinition{
		Name: "Review",
		Events: wfmodels.StepEvents{
			OnAgentError: []wfmodels.GenericAction{
				{Type: wfmodels.GenericActionMoveToStep, Config: map[string]interface{}{"step_id": "done"}},
			},
		},
	}
	repo.warnNonQueueRunAgentErrorActions("office-default", step)
}
