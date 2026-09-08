package sqlite

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// seedStaleOfficeReviewStep inserts a system-owned office-default workflow
// with a Review step whose on_enter sequence matches the pre-change shape
// (clear_decisions + queue_run_for_each_participant, no seat-ensuring
// action), simulating a workspace materialized before
// AC-OFFICE-REVIEW-SEATS-005.6 shipped.
func seedStaleOfficeReviewStep(t *testing.T, repo *Repository, workflowID, stepID string) {
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
	staleEvents := `{"on_enter":[{"type":"clear_decisions"},{"type":"queue_run_for_each_participant","config":{"role":"reviewer","reason":"task_assigned"}}]}`
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workflow_steps (
			id, workflow_id, name, position, stage_type, events, auto_advance_requires_signal, created_at, updated_at
		) VALUES (?, ?, 'Review', 2, 'review', ?, 0, ?, ?)
	`), stepID, workflowID, staleEvents, legacyTime, legacyTime)
	if err != nil {
		t.Fatalf("insert stale review step: %v", err)
	}
}

func loadReviewStepEvents(t *testing.T, repo *Repository, stepID string) *wfmodels.WorkflowStep {
	t.Helper()
	steps := loadStepsForWorkflowByID(t, repo, stepID)
	if len(steps) != 1 {
		t.Fatalf("expected exactly one step row for id %q, got %d", stepID, len(steps))
	}
	return steps[0]
}

// loadStepsForWorkflowByID mirrors loadStepsForWorkflow but filters by the
// step's own id rather than its workflow_id, since some of this file's
// scenarios seed a single step row without a full workflow of siblings.
func loadStepsForWorkflowByID(t *testing.T, repo *Repository, stepID string) []*wfmodels.WorkflowStep {
	t.Helper()
	rows, err := repo.db.QueryContext(context.Background(), repo.db.Rebind(`
		SELECT id, workflow_id, name, position, stage_type, events, auto_advance_requires_signal
		FROM workflow_steps WHERE id = ?
	`), stepID)
	if err != nil {
		t.Fatalf("query step: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*wfmodels.WorkflowStep
	for rows.Next() {
		step := &wfmodels.WorkflowStep{}
		var stage, events string
		var autoAdvanceRequiresSignal int
		if err := rows.Scan(&step.ID, &step.WorkflowID, &step.Name, &step.Position, &stage, &events, &autoAdvanceRequiresSignal); err != nil {
			t.Fatalf("scan step: %v", err)
		}
		step.AutoAdvanceRequiresSignal = autoAdvanceRequiresSignal != 0
		step.StageType = wfmodels.StageType(stage)
		if events != "" {
			if err := json.Unmarshal([]byte(events), &step.Events); err != nil {
				t.Fatalf("unmarshal events: %v", err)
			}
		}
		out = append(out, step)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	return out
}

func TestHealBuiltinWorkflowStepParticipantSeats_InsertsSeatBeforeMatchingFanout(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)

	seedStaleOfficeReviewStep(t, repo, "stale-office-seats", "stale-office-seats-review")

	if err := repo.healBuiltinWorkflowStepParticipantSeats(); err != nil {
		t.Fatalf("healBuiltinWorkflowStepParticipantSeats: %v", err)
	}

	step := loadReviewStepEvents(t, repo, "stale-office-seats-review")
	if len(step.Events.OnEnter) != 3 {
		t.Fatalf("Review.on_enter len = %d, want 3 (clear_decisions + ensure_participant_seat + queue_run_for_each_participant): %+v", len(step.Events.OnEnter), step.Events.OnEnter)
	}
	if step.Events.OnEnter[0].Type != wfmodels.OnEnterClearDecisions {
		t.Errorf("on_enter[0] = %q, want clear_decisions", step.Events.OnEnter[0].Type)
	}
	if step.Events.OnEnter[1].Type != wfmodels.OnEnterEnsureParticipantSeat {
		t.Errorf("on_enter[1] = %q, want ensure_participant_seat", step.Events.OnEnter[1].Type)
	}
	if role, _ := step.Events.OnEnter[1].Config["role"].(string); role != "reviewer" {
		t.Errorf("on_enter[1] role = %q, want reviewer", role)
	}
	if step.Events.OnEnter[2].Type != wfmodels.OnEnterQueueRunForEachParticipant {
		t.Errorf("on_enter[2] = %q, want queue_run_for_each_participant", step.Events.OnEnter[2].Type)
	}
	// The fan-out action's own config (untouched by reconciliation) must
	// survive the insertion (AC-OFFICE-REVIEW-SEATS-005.7).
	if reason, _ := step.Events.OnEnter[2].Config["reason"].(string); reason != "task_assigned" {
		t.Errorf("on_enter[2] reason = %q, want task_assigned to be preserved", reason)
	}
}

func TestHealBuiltinWorkflowStepParticipantSeats_MatchesFreshMaterializedOrder(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()

	seedStaleOfficeReviewStep(t, repo, "stale-office-match", "stale-office-match-review")
	if err := repo.healBuiltinWorkflowStepParticipantSeats(); err != nil {
		t.Fatalf("healBuiltinWorkflowStepParticipantSeats: %v", err)
	}
	reconciled := loadReviewStepEvents(t, repo, "stale-office-match-review")

	freshWorkflowID, err := repo.EnsureOfficeDefaultWorkflow(ctx, "ws-fresh")
	if err != nil {
		t.Fatalf("ensure fresh office-default: %v", err)
	}
	freshSteps := loadStepsForWorkflow(t, repo, freshWorkflowID)
	fresh := findStepByNameLocal(freshSteps, "Review")
	if fresh == nil {
		t.Fatal("fresh materialization missing Review step")
	}

	if len(reconciled.Events.OnEnter) != len(fresh.Events.OnEnter) {
		t.Fatalf("reconciled on_enter len = %d, fresh = %d", len(reconciled.Events.OnEnter), len(fresh.Events.OnEnter))
	}
	for i := range fresh.Events.OnEnter {
		gotType, wantType := reconciled.Events.OnEnter[i].Type, fresh.Events.OnEnter[i].Type
		if gotType != wantType {
			t.Errorf("on_enter[%d] type = %q, want %q (fresh materialization order)", i, gotType, wantType)
		}
		if wantType == wfmodels.OnEnterEnsureParticipantSeat || wantType == wfmodels.OnEnterQueueRunForEachParticipant {
			gotRole, _ := reconciled.Events.OnEnter[i].Config["role"].(string)
			wantRole, _ := fresh.Events.OnEnter[i].Config["role"].(string)
			if gotRole != wantRole {
				t.Errorf("on_enter[%d] role = %q, want %q", i, gotRole, wantRole)
			}
		}
	}
}

func TestHealBuiltinWorkflowStepParticipantSeats_Idempotent(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)

	seedStaleOfficeReviewStep(t, repo, "stale-office-idem", "stale-office-idem-review")

	if err := repo.healBuiltinWorkflowStepParticipantSeats(); err != nil {
		t.Fatalf("first heal: %v", err)
	}
	first := loadReviewStepEvents(t, repo, "stale-office-idem-review")

	if err := repo.healBuiltinWorkflowStepParticipantSeats(); err != nil {
		t.Fatalf("second heal: %v", err)
	}
	second := loadReviewStepEvents(t, repo, "stale-office-idem-review")

	if len(second.Events.OnEnter) != len(first.Events.OnEnter) {
		t.Fatalf("second heal changed on_enter length: %d -> %d", len(first.Events.OnEnter), len(second.Events.OnEnter))
	}
	seatCount := 0
	for _, action := range second.Events.OnEnter {
		if action.Type == wfmodels.OnEnterEnsureParticipantSeat {
			seatCount++
		}
	}
	if seatCount != 1 {
		t.Errorf("running the reconciler twice produced %d seat actions, want exactly 1", seatCount)
	}
}

func TestHealBuiltinWorkflowStepParticipantSeats_KeepsUserWorkflowUntouched(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()
	legacyTime := time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)

	_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workflows (
			id, workspace_id, name, workflow_template_id, is_system, hidden, created_at, updated_at
		) VALUES ('user-office-seats', 'ws-1', 'My Office', 'office-default', 0, 0, ?, ?)
	`), legacyTime, legacyTime)
	if err != nil {
		t.Fatalf("insert user office workflow: %v", err)
	}
	staleEvents := `{"on_enter":[{"type":"clear_decisions"},{"type":"queue_run_for_each_participant","config":{"role":"reviewer"}}]}`
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workflow_steps (
			id, workflow_id, name, position, stage_type, events, auto_advance_requires_signal, created_at, updated_at
		) VALUES ('user-office-seats-review', 'user-office-seats', 'Review', 2, 'review', ?, 0, ?, ?)
	`), staleEvents, legacyTime, legacyTime)
	if err != nil {
		t.Fatalf("insert user review step: %v", err)
	}

	if err := repo.healBuiltinWorkflowStepParticipantSeats(); err != nil {
		t.Fatalf("healBuiltinWorkflowStepParticipantSeats: %v", err)
	}

	step := loadReviewStepEvents(t, repo, "user-office-seats-review")
	if len(step.Events.OnEnter) != 2 {
		t.Errorf("user-customised (is_system=0) workflow step was modified: on_enter = %+v", step.Events.OnEnter)
	}
}

func TestRepositoryInitialization_HealsBuiltinWorkflowStepParticipantSeats(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)

	seedStaleOfficeReviewStep(t, repo, "stale-office-boot-seats", "stale-office-boot-seats-review")

	if err := repo.initSchema(); err != nil {
		t.Fatalf("reinitialize repository: %v", err)
	}

	step := loadReviewStepEvents(t, repo, "stale-office-boot-seats-review")
	found := false
	for _, action := range step.Events.OnEnter {
		if action.Type == wfmodels.OnEnterEnsureParticipantSeat {
			found = true
		}
	}
	if !found {
		t.Error("initSchema did not reconcile the participant seat action onto a stale Review step")
	}
}

func TestHealParticipantSeatRowWithRetry_ExhaustsRetriesWithoutBlockingStartup(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	core, logs := observer.New(zap.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("logger.NewFromZap: %v", err)
	}
	repo.log = log

	seedStaleOfficeReviewStep(t, repo, "stale-office-retry-exhaust", "stale-office-retry-exhaust-review")
	repo.failParticipantSeatReconcileAttempts = maxParticipantSeatReconcileAttempts + 5

	err = repo.healParticipantSeatRowWithRetry("stale-office-retry-exhaust-review", "stale-office-retry-exhaust", "office-default", "Review", "reviewer")
	if err != nil {
		t.Fatalf("healParticipantSeatRowWithRetry returned an error; AC-OFFICE-REVIEW-SEATS-005.9 requires retry exhaustion to never block startup: %v", err)
	}

	step := loadReviewStepEvents(t, repo, "stale-office-retry-exhaust-review")
	for _, action := range step.Events.OnEnter {
		if action.Type == wfmodels.OnEnterEnsureParticipantSeat {
			t.Error("step was modified despite every attempt reporting a concurrent-modification retry")
		}
	}
	if logs.Len() != 1 {
		t.Fatalf("expected exactly one warning record after retry exhaustion, got %d", logs.Len())
	}
	fields := logs.All()[0].ContextMap()
	if fields["step_name"] != "Review" || fields["role"] != "reviewer" {
		t.Errorf("warning fields = %+v, want step_name=Review role=reviewer", fields)
	}
}

func TestHealParticipantSeatRowWithRetry_SucceedsAfterTransientRetries(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)

	seedStaleOfficeReviewStep(t, repo, "stale-office-retry-ok", "stale-office-retry-ok-review")
	repo.failParticipantSeatReconcileAttempts = maxParticipantSeatReconcileAttempts - 1

	if err := repo.healParticipantSeatRowWithRetry("stale-office-retry-ok-review", "stale-office-retry-ok", "office-default", "Review", "reviewer"); err != nil {
		t.Fatalf("healParticipantSeatRowWithRetry: %v", err)
	}

	step := loadReviewStepEvents(t, repo, "stale-office-retry-ok-review")
	found := false
	for _, action := range step.Events.OnEnter {
		if action.Type == wfmodels.OnEnterEnsureParticipantSeat {
			found = true
		}
	}
	if !found {
		t.Error("expected the seat action to be inserted once transient retries were exhausted before the attempt budget")
	}
}

func TestInsertSeatAction_HeadWhenNoMatchingFanout(t *testing.T) {
	actions := []wfmodels.OnEnterAction{
		{Type: wfmodels.OnEnterClearDecisions},
	}
	got := insertSeatAction(actions, "approver")
	if len(got) != 2 || got[0].Type != wfmodels.OnEnterEnsureParticipantSeat {
		t.Fatalf("insertSeatAction with no matching fan-out = %+v, want seat action at head", got)
	}
}

func TestInsertSeatAction_BeforeMatchingRoleFanoutOnly(t *testing.T) {
	actions := []wfmodels.OnEnterAction{
		{Type: wfmodels.OnEnterClearDecisions},
		{Type: wfmodels.OnEnterQueueRunForEachParticipant, Config: map[string]interface{}{"role": "approver"}},
		{Type: wfmodels.OnEnterQueueRunForEachParticipant, Config: map[string]interface{}{"role": "reviewer"}},
	}
	got := insertSeatAction(actions, "reviewer")
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4", len(got))
	}
	if got[2].Type != wfmodels.OnEnterEnsureParticipantSeat {
		t.Fatalf("seat action inserted at index %d, want immediately before the matching-role fan-out (index 2): %+v", 2, got)
	}
	if got[3].Type != wfmodels.OnEnterQueueRunForEachParticipant {
		t.Fatalf("expected the reviewer fan-out to remain immediately after the inserted seat action: %+v", got)
	}
}

func TestHasSeatRole(t *testing.T) {
	actions := []wfmodels.OnEnterAction{
		{Type: wfmodels.OnEnterClearDecisions},
		{Type: wfmodels.OnEnterEnsureParticipantSeat, Config: map[string]interface{}{"role": "reviewer"}},
	}
	if !hasSeatRole(actions, "reviewer") {
		t.Error("hasSeatRole(reviewer) = false, want true")
	}
	if hasSeatRole(actions, "approver") {
		t.Error("hasSeatRole(approver) = true, want false (different role)")
	}
}

func TestTemplateSeatRoles_SkipsEmptyAndDedupes(t *testing.T) {
	step := wfmodels.StepDefinition{
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{
				{Type: wfmodels.OnEnterEnsureParticipantSeat, Config: map[string]interface{}{"role": "reviewer"}},
				{Type: wfmodels.OnEnterEnsureParticipantSeat, Config: map[string]interface{}{"role": "reviewer"}},
				{Type: wfmodels.OnEnterEnsureParticipantSeat, Config: map[string]interface{}{"role": ""}},
				{Type: wfmodels.OnEnterEnsureParticipantSeat},
				{Type: wfmodels.OnEnterEnsureParticipantSeat, Config: map[string]interface{}{"role": "approver"}},
				{Type: wfmodels.OnEnterClearDecisions},
			},
		},
	}
	roles := templateSeatRoles(step)
	if len(roles) != 2 || roles[0] != "reviewer" || roles[1] != "approver" {
		t.Fatalf("templateSeatRoles = %v, want [reviewer approver]", roles)
	}
}
