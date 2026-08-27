package orchestrator

// Ledger attribution coverage for the orchestrator's workflowStore: a plain
// ApplyTransition (as the engine's applyEngineTransition reaches it) records
// engine_transition/agent from its sessionID argument; a caller that already
// declared a trigger (as applyPendingMove does with mcp_deferred_move) wins
// over that default; and a repeated transition to the same target writes no
// second row — the ledger's own no-op guard, which is what makes an engine
// retry (already deduplicated upstream by OperationID) safe even if a retry
// ever reached this layer.

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/steptelemetry"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// TestExecuteStepTransitionRecordsEngineTransitionOnLegacyPath drives the
// legacy path directly (executeStepTransition, reached by ProcessOnTurnStart
// and ProcessOnTurnComplete) end to end and asserts the resulting ledger row
// — every other orchestrator ledger test exercises engineTransitionAttribution
// only through the newer workflowStore.ApplyTransition call site
// (TestApplyTransitionRecordsEngineTransitionFromSessionID) or in isolation
// (TestEngineTransitionAttributionPreservesUserCancellationTrigger), never
// through executeStepTransition's own call to it with a real ledger write.
// Table-driven over both legacy triggers so each of ProcessOnTurnStart's and
// ProcessOnTurnComplete's real call sites into executeStepTransition is
// proven to reach a correct session-originated ledger row on its own, not
// just one of the two. This does NOT distinguish on_turn_start from
// on_turn_complete: engineTriggerIsSessionOriginated puts both in the same
// case arm, so a bug that swapped the triggerOnEnter->engine.Trigger mapping
// at the executeStepTransition call site (event_handlers_workflow.go) would
// still produce byte-identical rows here and go undetected — only a change
// that stopped treating one of the two as session-originated would surface
// as a failure.
func TestExecuteStepTransitionRecordsEngineTransitionOnLegacyPath(t *testing.T) {
	tests := []struct {
		name           string
		triggerOnEnter bool
		wantTrigger    engine.Trigger
	}{
		{name: "on_turn_start", triggerOnEnter: false, wantTrigger: engine.TriggerOnTurnStart},
		{name: "on_turn_complete", triggerOnEnter: true, wantTrigger: engine.TriggerOnTurnComplete},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedSession(t, repo, "t1", "s1", "step1")

			steps := newMockStepGetter()
			fromStep := &wfmodels.WorkflowStep{ID: "step1", WorkflowID: "wf1", Name: "Source"}
			steps.steps["step2"] = &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1", Name: "Target"}
			svc := createTestService(repo, steps, newMockTaskRepo())

			svc.executeStepTransition(ctx, "t1", "s1", fromStep, "step2", tc.triggerOnEnter)

			rows := stepTransitionRowsForTaskOrchestrator(t, repo, "t1")
			if len(rows) == 0 {
				t.Fatalf("expected a ledger row, got none")
			}
			last := rows[len(rows)-1]
			if last.trigger != string(steptelemetry.TriggerEngineTransition) {
				t.Fatalf("trigger = %q, want %q", last.trigger, steptelemetry.TriggerEngineTransition)
			}
			if last.actorKind != string(steptelemetry.ActorAgent) {
				t.Fatalf("actor_kind = %q, want %q (legacy %s is session-originated)", last.actorKind, steptelemetry.ActorAgent, tc.wantTrigger)
			}
			if last.actorID == nil || *last.actorID != "s1" {
				t.Fatalf("actor_id = %v, want s1", last.actorID)
			}
			if last.sessionID == nil || *last.sessionID != "s1" {
				t.Fatalf("session_id = %v, want s1", last.sessionID)
			}
		})
	}
}

func stepTransitionRowsForTaskOrchestrator(t *testing.T, repo *sqliterepo.Repository, taskID string) []struct {
	trigger   string
	actorKind string
	actorID   *string
	sessionID *string
} {
	t.Helper()
	rows, err := repo.DB().QueryContext(context.Background(), `
		SELECT trigger, actor_kind, actor_id, session_id FROM task_step_transitions
		WHERE task_id = ? ORDER BY occurred_at ASC, id ASC
	`, taskID)
	if err != nil {
		t.Fatalf("query task_step_transitions: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var out []struct {
		trigger   string
		actorKind string
		actorID   *string
		sessionID *string
	}
	for rows.Next() {
		var r struct {
			trigger   string
			actorKind string
			actorID   *string
			sessionID *string
		}
		if err := rows.Scan(&r.trigger, &r.actorKind, &r.actorID, &r.sessionID); err != nil {
			t.Fatalf("scan row: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	return out
}

func lastStepTransitionWorkflowIDsForTaskOrchestrator(t *testing.T, repo *sqliterepo.Repository, taskID string) (fromWorkflowID, toWorkflowID string) {
	t.Helper()
	row := repo.DB().QueryRowContext(context.Background(), `
		SELECT from_workflow_id, to_workflow_id FROM task_step_transitions
		WHERE task_id = ? ORDER BY occurred_at DESC, id DESC LIMIT 1
	`, taskID)
	var from, to *string
	if err := row.Scan(&from, &to); err != nil {
		t.Fatalf("scan last row workflow ids: %v", err)
	}
	if from != nil {
		fromWorkflowID = *from
	}
	if to != nil {
		toWorkflowID = *to
	}
	return fromWorkflowID, toWorkflowID
}

func TestApplyTransitionCrossWorkflowRecordsDifferingWorkflowIDs(t *testing.T) {
	// GIVEN an engine action that switches a task to a step in a different
	// workflow, the ledger row's from/to workflow IDs must differ and the
	// trigger must remain engine_transition — mirrors
	// TestWorkflowStore_ApplyTransitionSyncsWorkflowIDAcrossWorkflows's setup,
	// which only asserts on task.WorkflowID and never touches the ledger.
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1") // seedSession puts the task in wf1

	if err := repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "wf2", WorkspaceID: "ws1", Name: "Other Workflow",
	}); err != nil {
		t.Fatalf("CreateWorkflow wf2: %v", err)
	}

	stepGetter := newMockStepGetter()
	stepGetter.steps["step-wf2"] = &wfmodels.WorkflowStep{
		ID: "step-wf2", WorkflowID: "wf2", Name: "Target", Position: 0,
	}
	store := newWorkflowStore(repo, stepGetter, nil, noopPublisher, testLogger())

	if err := store.ApplyTransition(ctx, "t1", "s1", "step1", "step-wf2", "on_turn_complete"); err != nil {
		t.Fatalf("ApplyTransition: %v", err)
	}

	rows := stepTransitionRowsForTaskOrchestrator(t, repo, "t1")
	last := rows[len(rows)-1]
	if last.trigger != string(steptelemetry.TriggerEngineTransition) {
		t.Fatalf("trigger = %q, want %q", last.trigger, steptelemetry.TriggerEngineTransition)
	}

	fromWorkflowID, toWorkflowID := lastStepTransitionWorkflowIDsForTaskOrchestrator(t, repo, "t1")
	if fromWorkflowID == "" || toWorkflowID == "" {
		t.Fatalf("expected both workflow ids set, got from=%q to=%q", fromWorkflowID, toWorkflowID)
	}
	if fromWorkflowID == toWorkflowID {
		t.Fatalf("expected from_workflow_id != to_workflow_id, both were %q", fromWorkflowID)
	}
	if toWorkflowID != "wf2" {
		t.Fatalf("to_workflow_id = %q, want %q", toWorkflowID, "wf2")
	}
}

// TestApplyTransitionRecordsEngineTransitionFromSessionID is table-driven
// over all four triggers engineTriggerIsSessionOriginated (Review round 2's
// fixup) treats as genuinely session-caused (spec.md:191-194). Before this,
// only on_turn_complete was exercised through the real attribution-decision
// code path — on_enter's only other test (TestApplyTransitionPreservesOuter-
// CallerTrigger) presets attribution on ctx, which short-circuits
// engineTransitionAttribution entirely via ApplyTransition's !HasTrigger
// guard, and on_exit/on_turn_start had no ledger-level coverage at all. A
// future edit dropping either from that whitelist would flip real rows from
// agent to system with a fully green suite otherwise.
func TestApplyTransitionRecordsEngineTransitionFromSessionID(t *testing.T) {
	sessionOriginatedTriggers := []engine.Trigger{
		engine.TriggerOnTurnStart, engine.TriggerOnTurnComplete, engine.TriggerOnEnter, engine.TriggerOnExit,
	}
	for _, trigger := range sessionOriginatedTriggers {
		t.Run(string(trigger), func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedSession(t, repo, "t1", "s1", "step1")

			store := newWorkflowStore(repo, newMockStepGetter(), nil, noopPublisher, testLogger())
			if err := store.ApplyTransition(ctx, "t1", "s1", "step1", "step2", trigger); err != nil {
				t.Fatalf("ApplyTransition: %v", err)
			}

			rows := stepTransitionRowsForTaskOrchestrator(t, repo, "t1")
			last := rows[len(rows)-1]
			if last.trigger != string(steptelemetry.TriggerEngineTransition) {
				t.Fatalf("trigger = %q, want %q", last.trigger, steptelemetry.TriggerEngineTransition)
			}
			if last.actorKind != string(steptelemetry.ActorAgent) {
				t.Fatalf("actor_kind = %q, want %q", last.actorKind, steptelemetry.ActorAgent)
			}
			if last.actorID == nil || *last.actorID != "s1" {
				t.Fatalf("actor_id = %v, want s1", last.actorID)
			}
			if last.sessionID == nil || *last.sessionID != "s1" {
				t.Fatalf("session_id = %v, want s1", last.sessionID)
			}
		})
	}
}

func TestApplyTransitionOnChildrenCompletedRecordsSystemActorNotSession(t *testing.T) {
	// A children-completed rollup resolves the parent's active session only
	// to satisfy the engine's MachineState API shape — that session did not
	// cause the transition, the children finishing did. Per spec.md:191-194,
	// this must record actor_kind=system with no actor_id/session_id, not
	// actor_kind=agent attributed to a session that had nothing to do with
	// it. Contrast with TestApplyTransitionRecordsEngineTransitionFromSessionID,
	// where "on_turn_complete" is genuinely session-turn-originated and agent
	// is correct.
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	store := newWorkflowStore(repo, newMockStepGetter(), nil, noopPublisher, testLogger())
	if err := store.ApplyTransition(ctx, "t1", "s1", "step1", "step2", "on_children_completed"); err != nil {
		t.Fatalf("ApplyTransition: %v", err)
	}

	rows := stepTransitionRowsForTaskOrchestrator(t, repo, "t1")
	last := rows[len(rows)-1]
	if last.trigger != string(steptelemetry.TriggerEngineTransition) {
		t.Fatalf("trigger = %q, want %q", last.trigger, steptelemetry.TriggerEngineTransition)
	}
	if last.actorKind != string(steptelemetry.ActorSystem) {
		t.Fatalf("actor_kind = %q, want %q (the resolved session did not cause this transition)", last.actorKind, steptelemetry.ActorSystem)
	}
	if last.actorID != nil {
		t.Fatalf("actor_id = %v, want nil", last.actorID)
	}
	if last.sessionID != nil {
		t.Fatalf("session_id = %v, want nil", last.sessionID)
	}
}

func TestApplyTransitionPreservesOuterCallerTrigger(t *testing.T) {
	// Mirrors applyPendingMove: it wraps ctx with mcp_deferred_move before
	// calling ApplyTransition, and that must survive rather than being
	// overwritten by ApplyTransition's own engine_transition default.
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	deferredCtx := steptelemetry.WithAttribution(ctx, steptelemetry.Attribution{
		Trigger: steptelemetry.TriggerMCPDeferredMove, ActorKind: steptelemetry.ActorAgent,
		ActorID: "s1", SessionID: "s1",
	})

	store := newWorkflowStore(repo, newMockStepGetter(), nil, noopPublisher, testLogger())
	if err := store.ApplyTransition(deferredCtx, "t1", "s1", "step1", "step2", "on_enter"); err != nil {
		t.Fatalf("ApplyTransition: %v", err)
	}

	rows := stepTransitionRowsForTaskOrchestrator(t, repo, "t1")
	last := rows[len(rows)-1]
	if last.trigger != string(steptelemetry.TriggerMCPDeferredMove) {
		t.Fatalf("trigger = %q, want %q (outer caller must win)", last.trigger, steptelemetry.TriggerMCPDeferredMove)
	}
}

func TestApplyTransitionRepeatedToSameTargetWritesNoSecondRow(t *testing.T) {
	// The engine already deduplicates retried triggers by OperationID before
	// ever reaching ApplyTransition, so no ledger-level idempotency key
	// exists. This proves the independent safety net: even if a retry
	// somehow reached this layer, the ledger's own no-op guard (identical
	// from/to step) means it still cannot write a second row.
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	store := newWorkflowStore(repo, newMockStepGetter(), nil, noopPublisher, testLogger())
	if err := store.ApplyTransition(ctx, "t1", "s1", "step1", "step2", "on_turn_complete"); err != nil {
		t.Fatalf("first ApplyTransition: %v", err)
	}
	before := stepTransitionRowsForTaskOrchestrator(t, repo, "t1")

	if err := store.ApplyTransition(ctx, "t1", "s1", "step1", "step2", "on_turn_complete"); err != nil {
		t.Fatalf("retried ApplyTransition: %v", err)
	}
	after := stepTransitionRowsForTaskOrchestrator(t, repo, "t1")

	if len(after) != len(before) {
		t.Fatalf("rows after retried transition = %d, want unchanged %d", len(after), len(before))
	}
}

// TestApplyTransitionOnUserCancellationRecordsHumanActorNotAgent covers
// carlosflorencio's PR #2623 review point 2b: an explicit user cancellation
// reaches ApplyTransition through the same on_turn_complete code path as a
// natural completion, but must not be attributed to the session's agent.
// cancellationTransitionAttribution pre-wraps ctx (as processOnTurnComplete-
// WithCause and processOnTurnCompleteViaEngineWithCause do when cause is
// turnCompletionCauseUserCancellation) so the row records the cancelling
// human's identity instead — see spec.md's user_cancellation carve-out.
func TestApplyTransitionOnUserCancellationRecordsHumanActorNotAgent(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-1", Role: authn.RoleMember})
	cancelCtx := cancellationTransitionAttribution(ctx)

	store := newWorkflowStore(repo, newMockStepGetter(), nil, noopPublisher, testLogger())
	if err := store.ApplyTransition(cancelCtx, "t1", "s1", "step1", "step2", engine.TriggerOnTurnComplete); err != nil {
		t.Fatalf("ApplyTransition: %v", err)
	}

	rows := stepTransitionRowsForTaskOrchestrator(t, repo, "t1")
	last := rows[len(rows)-1]
	if last.trigger != string(steptelemetry.TriggerUserCancellation) {
		t.Fatalf("trigger = %q, want %q", last.trigger, steptelemetry.TriggerUserCancellation)
	}
	if last.actorKind != string(steptelemetry.ActorHuman) {
		t.Fatalf("actor_kind = %q, want %q (cancellation is human-caused, not agent)", last.actorKind, steptelemetry.ActorHuman)
	}
	if last.actorID == nil || *last.actorID != "user-1" {
		t.Fatalf("actor_id = %v, want user-1", last.actorID)
	}
	if last.sessionID != nil {
		t.Fatalf("session_id = %v, want nil (the session is where the cancellation landed, not who caused it)", last.sessionID)
	}
}

// TestApplyTransitionOnUserCancellationWithNoIdentityRecordsSystemActor
// covers the unauthenticated-caller case (e.g. an automated cancellation with
// no request-scoped identity): cancellationTransitionAttribution must fall
// back to system, mirroring HumanOrSystemActor's manual_move behavior, not
// silently produce an empty/unknown actor.
func TestApplyTransitionOnUserCancellationWithNoIdentityRecordsSystemActor(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	cancelCtx := cancellationTransitionAttribution(context.Background())

	store := newWorkflowStore(repo, newMockStepGetter(), nil, noopPublisher, testLogger())
	if err := store.ApplyTransition(cancelCtx, "t1", "s1", "step1", "step2", engine.TriggerOnTurnComplete); err != nil {
		t.Fatalf("ApplyTransition: %v", err)
	}

	rows := stepTransitionRowsForTaskOrchestrator(t, repo, "t1")
	last := rows[len(rows)-1]
	if last.trigger != string(steptelemetry.TriggerUserCancellation) {
		t.Fatalf("trigger = %q, want %q", last.trigger, steptelemetry.TriggerUserCancellation)
	}
	if last.actorKind != string(steptelemetry.ActorSystem) {
		t.Fatalf("actor_kind = %q, want %q", last.actorKind, steptelemetry.ActorSystem)
	}
	if last.actorID != nil {
		t.Fatalf("actor_id = %v, want nil", last.actorID)
	}
}

// TestEngineTransitionAttributionPreservesUserCancellationTrigger pins the
// regression executeStepTransition's legacy on_turn_complete path would
// otherwise hit: before this fix, engineTransitionAttribution unconditionally
// overwrote whatever attribution ctx already carried, so a cancellation
// pre-wrap set by the caller would have been silently clobbered back to
// engine_transition/agent by executeStepTransition's own call. It must now
// respect the same outermost-caller-wins rule ApplyTransition's HasTrigger
// guard already enforces.
func TestEngineTransitionAttributionPreservesUserCancellationTrigger(t *testing.T) {
	cancelCtx := cancellationTransitionAttribution(context.Background())

	got := engineTransitionAttribution(cancelCtx, "s1", engine.TriggerOnTurnComplete)

	attribution := steptelemetry.FromContext(got)
	if attribution.Trigger != steptelemetry.TriggerUserCancellation {
		t.Fatalf("trigger = %q, want %q (outer caller must win)", attribution.Trigger, steptelemetry.TriggerUserCancellation)
	}
	if attribution.ActorKind == steptelemetry.ActorAgent {
		t.Fatalf("actor_kind = %q, must not fall back to agent once a cancellation trigger is set", attribution.ActorKind)
	}
}
