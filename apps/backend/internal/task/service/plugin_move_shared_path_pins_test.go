package service

import (
	"context"
	"database/sql"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/steptelemetry"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// This file pins REQ-PLUGINS-STEP-MOVE-005's (pin)-labelled acceptance
// criteria: behavior the shared move path already has, that plugin moves now
// also exercise. Each test drives MoveTaskWithOptions with exactly the
// options and context attribution (backendapp.pluginsTaskWriterAdapter).
// MoveTask sets — see internal/backendapp/services.go — so a regression in
// the shared path plugin moves depend on fails here, not silently.

// pluginMoveOptions mirrors what the plugin adapter passes as
// taskservice.MoveTaskOptions.
func pluginMoveOptions() MoveTaskOptions {
	return MoveTaskOptions{
		StepHistoryTrigger: wfmodels.StepTransitionTriggerPluginMove,
		StepHistoryActor:   wfmodels.StepTransitionActorSystem,
	}
}

// pluginMoveContext mirrors the steptelemetry.Attribution the plugin adapter
// attaches to the context before calling MoveTaskWithOptions.
func pluginMoveContext(source string) context.Context {
	return steptelemetry.WithAttribution(context.Background(), steptelemetry.Attribution{
		Trigger:   steptelemetry.TriggerPluginMove,
		ActorKind: steptelemetry.ActorIntegration,
		ActorID:   source,
	})
}

// stepTransitionTriggers reads back every task_step_transitions.trigger row
// for taskID, oldest first, using the exported Repository.DB() accessor. The
// ledger has no application-layer reader — internal/telemetrycontract's raw
// SQL is the only other consumer — so a direct query is the only way to pin
// what was actually committed, not merely what a caller requested.
func stepTransitionTriggers(t *testing.T, repo *sqliterepo.Repository, taskID string) []string {
	t.Helper()
	rows, err := repo.DB().Query(
		`SELECT trigger FROM task_step_transitions WHERE task_id = ? ORDER BY id ASC`, taskID)
	if err != nil {
		t.Fatalf("query task_step_transitions: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var triggers []string
	for rows.Next() {
		var trigger string
		if err := rows.Scan(&trigger); err != nil {
			t.Fatalf("scan trigger: %v", err)
		}
		triggers = append(triggers, trigger)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate task_step_transitions: %v", err)
	}
	return triggers
}

// createEphemeralMoveTask creates an is_ephemeral task attached to a
// workflow step, the shape AC-005.14 exercises: an ephemeral task that is
// nonetheless placed on a workflow step (e.g. a quick-chat task moved onto a
// board) and must admit without consuming that step's WIP capacity.
func createEphemeralMoveTask(t *testing.T, ctx context.Context, repo interface {
	CreateTask(context.Context, *models.Task) error
}, id, workflowID, stepID string) {
	t.Helper()
	if err := repo.CreateTask(ctx, &models.Task{
		ID:             id,
		WorkspaceID:    "ws-1",
		WorkflowID:     workflowID,
		WorkflowStepID: stepID,
		Title:          id,
		State:          v1.TaskStateTODO,
		Priority:       "medium",
		IsEphemeral:    true,
	}); err != nil {
		t.Fatalf("CreateTask(ephemeral %s): %v", id, err)
	}
}

// TestSharedMovePath_PluginAttributedVacatePromotesUnderWIPPullTrigger pins
// AC-PLUGINS-STEP-MOVE-005.8: vacating a step that has tasks queued for it
// runs the same reconciliation the board's move runs, and the resulting
// promotion is recorded under the promotion's own trigger
// (wip_pull) rather than the vacating caller's (plugin_move) — even though
// the vacating move itself was driven with plugin attribution end to end.
func TestSharedMovePath_PluginAttributedVacatePromotesUnderWIPPullTrigger(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-limited": {
			ID: "step-limited", WorkflowID: "wf-source", Name: "Limited", Position: 0,
			WIPLimit: 1, PullFromStepID: "step-feeder",
		},
		"step-feeder": {ID: "step-feeder", WorkflowID: "wf-source", Name: "Feeder", Position: 1},
		"step-target": {ID: "step-target", WorkflowID: "wf-target", Name: "Target", Position: 0},
	}})
	createMoveTask(t, ctx, repo, "task-vacating", "wf-source", "step-limited", nil)
	createMoveTask(t, ctx, repo, "task-waiting", "wf-source", "step-feeder", nil)

	moveCtx := pluginMoveContext("plugin:acme")
	if _, err := svc.MoveTaskWithOptions(moveCtx, "task-vacating", "wf-target", "step-target", 0, pluginMoveOptions()); err != nil {
		t.Fatalf("MoveTaskWithOptions: %v", err)
	}

	promoted, err := repo.GetTask(ctx, "task-waiting")
	if err != nil {
		t.Fatalf("GetTask(task-waiting): %v", err)
	}
	if promoted.WorkflowStepID != "step-limited" {
		t.Fatalf("feeder task step = %s, want promoted into step-limited", promoted.WorkflowStepID)
	}

	// Every CreateTask call writes its own task_created genesis row (see
	// genesisAttribution in step_transitions.go), so each task's ledger
	// carries that row before the move-driven row this test cares about —
	// assert the LAST row rather than the full history.
	vacatingTriggers := stepTransitionTriggers(t, repo, "task-vacating")
	if len(vacatingTriggers) == 0 || vacatingTriggers[len(vacatingTriggers)-1] != string(steptelemetry.TriggerPluginMove) {
		t.Fatalf("task-vacating ledger triggers = %v, want the last row to be %s", vacatingTriggers, steptelemetry.TriggerPluginMove)
	}
	promotedTriggers := stepTransitionTriggers(t, repo, "task-waiting")
	if len(promotedTriggers) == 0 || promotedTriggers[len(promotedTriggers)-1] != string(steptelemetry.TriggerWIPPull) {
		t.Fatalf("task-waiting ledger triggers = %v, want the last row to be %s (never the vacating move's plugin_move)",
			promotedTriggers, steptelemetry.TriggerWIPPull)
	}
}

// TestSharedMovePath_ConcurrentSameTaskSameStepNeverExceedsWIPLimit pins
// AC-PLUGINS-STEP-MOVE-005.2's serialization half: two callers moving
// different tasks onto the same WIP-limited step concurrently must never
// admit more than the step's limit, regardless of write interleaving.
func TestSharedMovePath_ConcurrentSameTaskSameStepNeverExceedsWIPLimit(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-source": {ID: "step-source", WorkflowID: "wf-source", Name: "Source", Position: 0},
		"step-full":   {ID: "step-full", WorkflowID: "wf-source", Name: "Full", Position: 1, WIPLimit: 1},
	}})
	createMoveTask(t, ctx, repo, "task-a", "wf-source", "step-source", nil)
	createMoveTask(t, ctx, repo, "task-b", "wf-source", "step-source", nil)

	var wg sync.WaitGroup
	wg.Add(2)
	for _, taskID := range []string{"task-a", "task-b"} {
		go func(id string) {
			defer wg.Done()
			_, _ = svc.MoveTaskWithOptions(pluginMoveContext("plugin:acme"), id, "wf-source", "step-full", 0, pluginMoveOptions())
		}(taskID)
	}
	wg.Wait()

	admitted := 0
	for _, taskID := range []string{"task-a", "task-b"} {
		task, err := repo.GetTask(ctx, taskID)
		if err != nil {
			t.Fatalf("GetTask(%s): %v", taskID, err)
		}
		if task.WorkflowStepID != "step-full" {
			t.Fatalf("task %s step = %s, want step-full", taskID, task.WorkflowStepID)
		}
		if task.WIPAdmitted {
			admitted++
			if task.QueuedForStepID != "" {
				t.Fatalf("admitted task %s still carries queued_for_step_id = %q", taskID, task.QueuedForStepID)
			}
		} else if task.QueuedForStepID != "step-full" {
			t.Fatalf("non-admitted task %s queued_for_step_id = %q, want step-full", taskID, task.QueuedForStepID)
		}
	}
	if admitted != 1 {
		t.Fatalf("admitted count = %d, want exactly 1 (WIP limit is 1)", admitted)
	}
}

// TestSharedMovePath_ConcurrentDifferentStepMovesRecordDistinctTransitions
// pins AC-PLUGINS-STEP-MOVE-005.10: two callers moving the same task to
// different steps concurrently must serialize on the task write, commit both,
// and record one ledger row per committed change — the writer never
// collapses two committed step changes into a single row.
func TestSharedMovePath_ConcurrentDifferentStepMovesRecordDistinctTransitions(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-source": {ID: "step-source", WorkflowID: "wf-source", Name: "Source", Position: 0},
		"step-a":      {ID: "step-a", WorkflowID: "wf-source", Name: "A", Position: 1},
		"step-b":      {ID: "step-b", WorkflowID: "wf-source", Name: "B", Position: 2},
	}})
	createMoveTask(t, ctx, repo, "task-racing", "wf-source", "step-source", nil)

	var wg sync.WaitGroup
	wg.Add(2)
	for _, dest := range []string{"step-a", "step-b"} {
		go func(step string) {
			defer wg.Done()
			_, _ = svc.MoveTaskWithOptions(pluginMoveContext("plugin:acme"), "task-racing", "wf-source", step, 0, pluginMoveOptions())
		}(dest)
	}
	wg.Wait()

	final, err := repo.GetTask(ctx, "task-racing")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if final.WorkflowStepID != "step-a" && final.WorkflowStepID != "step-b" {
		t.Fatalf("final step = %s, want step-a or step-b", final.WorkflowStepID)
	}

	// The genesis task_created row (see genesisAttribution in
	// step_transitions.go) precedes the two move-driven rows this test
	// cares about — assert the trailing two, not the full history.
	triggers := stepTransitionTriggers(t, repo, "task-racing")
	if len(triggers) != 3 {
		t.Fatalf("ledger rows for task-racing = %d (%v), want exactly 3 (1 genesis + one per committed move, not collapsed)", len(triggers), triggers)
	}
	for _, trigger := range triggers[1:] {
		if trigger != string(steptelemetry.TriggerPluginMove) {
			t.Fatalf("ledger trigger = %q, want %s for each committed move row", trigger, steptelemetry.TriggerPluginMove)
		}
	}
}

// TestSharedMovePath_SameStepMoveReportsNoTransitionAndEmptyFromStepID pins
// AC-PLUGINS-STEP-MOVE-002.6 and AC-PLUGINS-STEP-MOVE-005.1: a plugin move
// that names the step the task already occupies is a no-op write through the
// real shared path (updateMovedTask's early oldStepID == targetStep.ID
// branch, which calls the plain repository UpdateTask/updateTaskTx — not one
// of the WIP-admission call sites). System-design.md's "Both outcome fields
// come from the write transaction" requires FromStepID be empty whenever
// Transitioned is false; models.Task.FromStepID's own doc says the same
// ("Empty when no transition occurred"). This drives the real repository,
// not a mock, so a regression in updateTaskTx's FromStepID assignment fails
// here rather than only in a hand-constructed MoveTaskResult fixture.
func TestSharedMovePath_SameStepMoveReportsNoTransitionAndEmptyFromStepID(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-source": {ID: "step-source", WorkflowID: "wf-source", Name: "Source", Position: 0},
	}})
	createMoveTask(t, ctx, repo, "task-stationary", "wf-source", "step-source", nil)

	result, err := svc.MoveTaskWithOptions(pluginMoveContext("plugin:acme"), "task-stationary", "wf-source", "step-source", 0, pluginMoveOptions())
	if err != nil {
		t.Fatalf("MoveTaskWithOptions: %v", err)
	}
	if result.Transitioned {
		t.Fatalf("Transitioned = true, want false for a same-step move (no ledger row)")
	}
	if result.FromStepID != "" {
		t.Fatalf("FromStepID = %q, want empty when Transitioned is false (design: both outcome fields come from the write transaction)", result.FromStepID)
	}

	// The genesis task_created row is the only ledger row — the no-op move
	// must not have appended a second one.
	triggers := stepTransitionTriggers(t, repo, "task-stationary")
	if len(triggers) != 1 {
		t.Fatalf("ledger rows for task-stationary = %d (%v), want exactly 1 (genesis only, no row for the no-op move)", len(triggers), triggers)
	}
}

// lastStepTransitionActor reads back the actor_kind/actor_id columns of the
// most recent task_step_transitions row for taskID, mirroring
// stepTransitionTriggers' direct-SQL approach since the ledger has no
// application-layer reader.
func lastStepTransitionActor(t *testing.T, repo *sqliterepo.Repository, taskID string) (actorKind, actorID string) {
	t.Helper()
	var kind sql.NullString
	var id sql.NullString
	err := repo.DB().QueryRow(
		`SELECT actor_kind, actor_id FROM task_step_transitions WHERE task_id = ? ORDER BY id DESC LIMIT 1`, taskID,
	).Scan(&kind, &id)
	if err != nil {
		t.Fatalf("query last task_step_transitions row for %s: %v", taskID, err)
	}
	return kind.String, id.String
}

// TestSharedMovePath_PluginMoveRecordsIntegrationActorOnLedgerRow pins
// AC-003.1: a plugin-initiated move must land its actor_kind/actor_id on the
// committed task_step_transitions row exactly as
// backendapp.pluginsTaskWriterAdapter.MoveTask attaches them
// (steptelemetry.ActorIntegration + the plugin's source string), not merely
// pass them through in-memory — this drives the real repository write, so a
// regression in how recordStepTransition persists the attribution fails here.
func TestSharedMovePath_PluginMoveRecordsIntegrationActorOnLedgerRow(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-source": {ID: "step-source", WorkflowID: "wf-source", Name: "Source", Position: 0},
		"step-target": {ID: "step-target", WorkflowID: "wf-source", Name: "Target", Position: 1},
	}})
	createMoveTask(t, ctx, repo, "task-attributed", "wf-source", "step-source", nil)

	result, err := svc.MoveTaskWithOptions(pluginMoveContext("plugin:acme"), "task-attributed", "wf-source", "step-target", 0, pluginMoveOptions())
	if err != nil {
		t.Fatalf("MoveTaskWithOptions: %v", err)
	}
	if !result.Transitioned {
		t.Fatalf("Transitioned = false, want true for a cross-step move")
	}

	actorKind, actorID := lastStepTransitionActor(t, repo, "task-attributed")
	if actorKind != string(steptelemetry.ActorIntegration) {
		t.Fatalf("ledger actor_kind = %q, want %q", actorKind, steptelemetry.ActorIntegration)
	}
	if actorID != "plugin:acme" {
		t.Fatalf("ledger actor_id = %q, want %q", actorID, "plugin:acme")
	}
}

// TestSharedMovePath_NonPositiveWIPLimitAdmitsImmediately pins
// AC-PLUGINS-STEP-MOVE-005.12: an absent, zero, or negative WIP limit is
// treated as unlimited, so the move admits rather than queues, and reports
// admission via QueuedForStepID being empty (never via WIPAdmitted alone —
// AC-002.2).
func TestSharedMovePath_NonPositiveWIPLimitAdmitsImmediately(t *testing.T) {
	for name, limit := range map[string]int{"absent (zero value)": 0, "explicit zero": 0, "negative": -1} {
		t.Run(name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			ctx := context.Background()
			seedMoveWorkflows(t, ctx, repo)
			svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
				"step-source":    {ID: "step-source", WorkflowID: "wf-source", Name: "Source", Position: 0},
				"step-unlimited": {ID: "step-unlimited", WorkflowID: "wf-source", Name: "Unlimited", Position: 1, WIPLimit: limit},
			}})
			createMoveTask(t, ctx, repo, "task-moving", "wf-source", "step-source", nil)
			createMoveTask(t, ctx, repo, "task-occupant", "wf-source", "step-unlimited", nil)

			result, err := svc.MoveTaskWithOptions(pluginMoveContext("plugin:acme"), "task-moving", "wf-source", "step-unlimited", 0, pluginMoveOptions())
			if err != nil {
				t.Fatalf("MoveTaskWithOptions: %v", err)
			}
			if result.Task.QueuedForStepID != "" {
				t.Fatalf("QueuedForStepID = %q, want empty (admitted) under a non-positive WIP limit", result.Task.QueuedForStepID)
			}
			if !result.Task.WIPAdmitted {
				t.Fatal("WIPAdmitted = false, want true under a non-positive WIP limit")
			}
		})
	}
}

// TestSharedMovePath_EphemeralTaskAdmitsWithoutConsumingCapacity pins
// AC-PLUGINS-STEP-MOVE-005.14: moving an ephemeral task always admits and
// reports admission (QueuedForStepID empty) even onto a step already at its
// WIP limit, because an ephemeral task is queued for no step.
func TestSharedMovePath_EphemeralTaskAdmitsWithoutConsumingCapacity(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-source": {ID: "step-source", WorkflowID: "wf-source", Name: "Source", Position: 0},
		"step-full":   {ID: "step-full", WorkflowID: "wf-source", Name: "Full", Position: 1, WIPLimit: 1},
	}})
	createEphemeralMoveTask(t, ctx, repo, "task-ephemeral", "wf-source", "step-source")
	createMoveTask(t, ctx, repo, "task-occupant", "wf-source", "step-full", nil)

	result, err := svc.MoveTaskWithOptions(pluginMoveContext("plugin:acme"), "task-ephemeral", "wf-source", "step-full", 0, pluginMoveOptions())
	if err != nil {
		t.Fatalf("MoveTaskWithOptions: %v", err)
	}
	if result.Task.QueuedForStepID != "" {
		t.Fatalf("QueuedForStepID = %q, want empty (admitted) for an ephemeral task", result.Task.QueuedForStepID)
	}
	if result.Task.WorkflowStepID != "step-full" {
		t.Fatalf("WorkflowStepID = %s, want step-full", result.Task.WorkflowStepID)
	}
}
