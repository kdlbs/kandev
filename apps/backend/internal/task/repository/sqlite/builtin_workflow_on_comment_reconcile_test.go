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

// seedStaleOfficeStepWithoutOnComment inserts a system-owned office-default
// workflow with a single named step whose events carry an on_enter fan-out
// but no on_comment action, simulating a workspace materialized before the
// gate-comment-wake fan-out shipped.
func seedStaleOfficeStepWithoutOnComment(t *testing.T, repo *Repository, workflowID, stepID, stepName, stageType, role string) {
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
	staleEvents := `{"on_enter":[{"type":"queue_run_for_each_participant","config":{"role":"` + role + `","reason":"task_assigned"}}],` +
		`"on_turn_complete":[{"type":"move_to_step","config":{"step_id":"done"}}]}`
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workflow_steps (
			id, workflow_id, name, position, stage_type, events, auto_advance_requires_signal, created_at, updated_at
		) VALUES (?, ?, ?, 2, ?, ?, 0, ?, ?)
	`), stepID, workflowID, stepName, stageType, staleEvents, legacyTime, legacyTime)
	if err != nil {
		t.Fatalf("insert stale %s step: %v", stepName, err)
	}
}

func TestHealBuiltinWorkflowStepOnCommentFanOut_InsertsReviewerFanOut(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)

	seedStaleOfficeStepWithoutOnComment(t, repo, "stale-office-on-comment", "stale-office-on-comment-review", "Review", "review", "reviewer")

	if err := repo.healBuiltinWorkflowStepOnCommentFanOut(); err != nil {
		t.Fatalf("healBuiltinWorkflowStepOnCommentFanOut: %v", err)
	}

	step := loadStepEvents(t, repo, "stale-office-on-comment-review")
	if len(step.Events.OnComment) != 1 {
		t.Fatalf("Review.on_comment len = %d, want 1: %+v", len(step.Events.OnComment), step.Events.OnComment)
	}
	action := step.Events.OnComment[0]
	if action.Type != wfmodels.GenericActionQueueRunForEachParticipant {
		t.Errorf("on_comment[0].Type = %q, want queue_run_for_each_participant", action.Type)
	}
	if role, _ := action.Config["role"].(string); role != "reviewer" {
		t.Errorf("on_comment[0] role = %q, want reviewer", role)
	}
	if reason, _ := action.Config["reason"].(string); reason != "task_comment" {
		t.Errorf("on_comment[0] reason = %q, want task_comment", reason)
	}
	if skipDecided, _ := action.Config["skip_decided"].(bool); !skipDecided {
		t.Errorf("on_comment[0] skip_decided = %v, want true", action.Config["skip_decided"])
	}
	payload, _ := action.Config["payload"].(map[string]interface{})
	if stageType, _ := payload["stage_type"].(string); stageType != "review" {
		t.Errorf("on_comment[0] payload.stage_type = %q, want review", stageType)
	}
	// Existing actions on other triggers must survive reconciliation.
	if len(step.Events.OnEnter) != 1 {
		t.Errorf("on_enter was modified by the reconciler: %+v", step.Events.OnEnter)
	}
	if len(step.Events.OnTurnComplete) != 1 {
		t.Errorf("on_turn_complete was modified by the reconciler: %+v", step.Events.OnTurnComplete)
	}
}

func TestHealBuiltinWorkflowStepOnCommentFanOut_InsertsApproverFanOut(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)

	seedStaleOfficeStepWithoutOnComment(t, repo, "stale-office-on-comment-appr", "stale-office-on-comment-approval", "Approval", "approval", "approver")

	if err := repo.healBuiltinWorkflowStepOnCommentFanOut(); err != nil {
		t.Fatalf("healBuiltinWorkflowStepOnCommentFanOut: %v", err)
	}

	step := loadStepEvents(t, repo, "stale-office-on-comment-approval")
	if len(step.Events.OnComment) != 1 {
		t.Fatalf("Approval.on_comment len = %d, want 1: %+v", len(step.Events.OnComment), step.Events.OnComment)
	}
	action := step.Events.OnComment[0]
	if role, _ := action.Config["role"].(string); role != "approver" {
		t.Errorf("on_comment[0] role = %q, want approver", role)
	}
	payload, _ := action.Config["payload"].(map[string]interface{})
	if stageType, _ := payload["stage_type"].(string); stageType != "approval" {
		t.Errorf("on_comment[0] payload.stage_type = %q, want approval", stageType)
	}
}

func TestHealBuiltinWorkflowStepOnCommentFanOut_Idempotent(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)

	seedStaleOfficeStepWithoutOnComment(t, repo, "stale-office-on-comment-idem", "stale-office-on-comment-idem-review", "Review", "review", "reviewer")

	if err := repo.healBuiltinWorkflowStepOnCommentFanOut(); err != nil {
		t.Fatalf("first heal: %v", err)
	}
	first := loadStepEvents(t, repo, "stale-office-on-comment-idem-review")

	if err := repo.healBuiltinWorkflowStepOnCommentFanOut(); err != nil {
		t.Fatalf("second heal: %v", err)
	}
	second := loadStepEvents(t, repo, "stale-office-on-comment-idem-review")

	if len(second.Events.OnComment) != len(first.Events.OnComment) {
		t.Fatalf("second heal changed on_comment length: %d -> %d", len(first.Events.OnComment), len(second.Events.OnComment))
	}
	if len(second.Events.OnComment) != 1 {
		t.Errorf("running the reconciler twice produced %d fan-out actions, want exactly 1", len(second.Events.OnComment))
	}
}

func TestHealBuiltinWorkflowStepOnCommentFanOut_KeepsUserWorkflowUntouched(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()
	legacyTime := time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)

	_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workflows (
			id, workspace_id, name, workflow_template_id, is_system, hidden, created_at, updated_at
		) VALUES ('user-office-on-comment', 'ws-1', 'My Office', 'office-default', 0, 0, ?, ?)
	`), legacyTime, legacyTime)
	if err != nil {
		t.Fatalf("insert user office workflow: %v", err)
	}
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workflow_steps (
			id, workflow_id, name, position, stage_type, events, auto_advance_requires_signal, created_at, updated_at
		) VALUES ('user-office-on-comment-review', 'user-office-on-comment', 'Review', 2, 'review', '{}', 0, ?, ?)
	`), legacyTime, legacyTime)
	if err != nil {
		t.Fatalf("insert user review step: %v", err)
	}

	if err := repo.healBuiltinWorkflowStepOnCommentFanOut(); err != nil {
		t.Fatalf("healBuiltinWorkflowStepOnCommentFanOut: %v", err)
	}

	step := loadStepEvents(t, repo, "user-office-on-comment-review")
	if len(step.Events.OnComment) != 0 {
		t.Errorf("user-customised (is_system=0) workflow step was modified: on_comment = %+v", step.Events.OnComment)
	}
}

// TestHealBuiltinWorkflowStepOnCommentFanOut_LeavesSameRoleFanOutUnchanged
// pins AC-OFFICE-GATE-COMMENT-004.3/.5: a step that already declares an
// on_comment queue_run_for_each_participant action for the template's role is
// left alone, even though its reason/payload differ from the template's.
func TestHealBuiltinWorkflowStepOnCommentFanOut_LeavesSameRoleFanOutUnchanged(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()
	legacyTime := time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)

	_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workflows (
			id, workspace_id, name, workflow_template_id, is_system, hidden, created_at, updated_at
		) VALUES ('stale-office-on-comment-custom', 'ws-1', 'Office', 'office-default', 1, 1, ?, ?)
	`), legacyTime, legacyTime)
	if err != nil {
		t.Fatalf("insert stale office workflow: %v", err)
	}
	customEvents := `{"on_comment":[{"type":"queue_run_for_each_participant","config":{"role":"reviewer","reason":"custom_reason"}}]}`
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workflow_steps (
			id, workflow_id, name, position, stage_type, events, auto_advance_requires_signal, created_at, updated_at
		) VALUES ('stale-office-on-comment-custom-review', 'stale-office-on-comment-custom', 'Review', 2, 'review', ?, 0, ?, ?)
	`), customEvents, legacyTime, legacyTime)
	if err != nil {
		t.Fatalf("insert custom review step: %v", err)
	}

	if err := repo.healBuiltinWorkflowStepOnCommentFanOut(); err != nil {
		t.Fatalf("healBuiltinWorkflowStepOnCommentFanOut: %v", err)
	}

	step := loadStepEvents(t, repo, "stale-office-on-comment-custom-review")
	if len(step.Events.OnComment) != 1 {
		t.Fatalf("on_comment len = %d, want 1 (unchanged): %+v", len(step.Events.OnComment), step.Events.OnComment)
	}
	if reason, _ := step.Events.OnComment[0].Config["reason"].(string); reason != "custom_reason" {
		t.Errorf("existing same-role on_comment action was overwritten: reason = %q, want custom_reason", reason)
	}
}

func TestRepositoryInitialization_HealsBuiltinWorkflowStepOnCommentFanOut(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)

	seedStaleOfficeStepWithoutOnComment(t, repo, "stale-office-boot-on-comment", "stale-office-boot-on-comment-review", "Review", "review", "reviewer")

	if err := repo.initSchema(); err != nil {
		t.Fatalf("reinitialize repository: %v", err)
	}

	step := loadStepEvents(t, repo, "stale-office-boot-on-comment-review")
	if len(step.Events.OnComment) != 1 {
		t.Error("initSchema did not reconcile the on_comment fan-out action onto a stale Review step")
	}
}

func TestHealOnCommentRowWithRetry_ExhaustsRetriesWithoutBlockingStartup(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	core, logs := observer.New(zap.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("logger.NewFromZap: %v", err)
	}
	repo.log = log

	seedStaleOfficeStepWithoutOnComment(t, repo, "stale-office-on-comment-retry-exhaust", "stale-office-on-comment-retry-exhaust-review", "Review", "review", "reviewer")
	repo.failOnCommentReconcileAttempts = maxOnCommentReconcileAttempts + 5

	action := wfmodels.GenericAction{
		Type: wfmodels.GenericActionQueueRunForEachParticipant,
		Config: map[string]interface{}{
			"role": "reviewer", "reason": "task_comment", "skip_decided": true,
			"payload": map[string]interface{}{"stage_type": "review"},
		},
	}
	err = repo.healOnCommentRowWithRetry(
		"stale-office-on-comment-retry-exhaust-review", "stale-office-on-comment-retry-exhaust", "office-default", "Review", action,
	)
	if err != nil {
		t.Fatalf("healOnCommentRowWithRetry returned an error; retry exhaustion must never block startup: %v", err)
	}

	step := loadStepEvents(t, repo, "stale-office-on-comment-retry-exhaust-review")
	if len(step.Events.OnComment) != 0 {
		t.Error("step was modified despite every attempt reporting a concurrent-modification retry")
	}
	if logs.Len() != 1 {
		t.Fatalf("expected exactly one warning record after retry exhaustion, got %d", logs.Len())
	}
	fields := logs.All()[0].ContextMap()
	if fields["step_name"] != "Review" || fields["role"] != "reviewer" {
		t.Errorf("warning fields = %+v, want step_name=Review role=reviewer", fields)
	}
}

func TestHealOnCommentRowWithRetry_SucceedsAfterTransientRetries(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)

	seedStaleOfficeStepWithoutOnComment(t, repo, "stale-office-on-comment-retry-ok", "stale-office-on-comment-retry-ok-review", "Review", "review", "reviewer")
	repo.failOnCommentReconcileAttempts = maxOnCommentReconcileAttempts - 1

	action := wfmodels.GenericAction{
		Type: wfmodels.GenericActionQueueRunForEachParticipant,
		Config: map[string]interface{}{
			"role": "reviewer", "reason": "task_comment", "skip_decided": true,
			"payload": map[string]interface{}{"stage_type": "review"},
		},
	}
	if err := repo.healOnCommentRowWithRetry(
		"stale-office-on-comment-retry-ok-review", "stale-office-on-comment-retry-ok", "office-default", "Review", action,
	); err != nil {
		t.Fatalf("healOnCommentRowWithRetry: %v", err)
	}

	step := loadStepEvents(t, repo, "stale-office-on-comment-retry-ok-review")
	if len(step.Events.OnComment) != 1 {
		t.Error("expected the fan-out action to be inserted once transient retries were exhausted before the attempt budget")
	}
}

func TestHasOnCommentFanOutRole(t *testing.T) {
	actions := []wfmodels.GenericAction{
		{Type: wfmodels.GenericActionQueueRunForEachParticipant, Config: map[string]interface{}{"role": "reviewer"}},
	}
	if !hasOnCommentFanOutRole(actions, "reviewer") {
		t.Error("hasOnCommentFanOutRole did not find the reviewer role")
	}
	if hasOnCommentFanOutRole(actions, "approver") {
		t.Error("hasOnCommentFanOutRole treated a different role as present")
	}
}

func TestTemplateOnCommentFanOutActions_DedupesByRole(t *testing.T) {
	step := wfmodels.StepDefinition{
		Events: wfmodels.StepEvents{
			OnComment: []wfmodels.GenericAction{
				{Type: wfmodels.GenericActionQueueRunForEachParticipant, Config: map[string]interface{}{"role": "reviewer", "reason": "task_comment"}},
				{Type: wfmodels.GenericActionQueueRunForEachParticipant, Config: map[string]interface{}{"role": "reviewer", "reason": "task_comment_dup"}},
				{Type: wfmodels.GenericActionQueueRunForEachParticipant, Config: map[string]interface{}{"role": "", "reason": "no_role"}},
				{Type: wfmodels.GenericActionMoveToStep, Config: map[string]interface{}{"step_id": "done"}},
			},
		},
	}
	actions := templateOnCommentFanOutActions(step)
	if len(actions) != 1 {
		t.Fatalf("templateOnCommentFanOutActions len = %d, want 1: %+v", len(actions), actions)
	}
	if reason, _ := actions[0].Config["reason"].(string); reason != "task_comment" {
		t.Errorf("templateOnCommentFanOutActions kept %q, want the first reviewer action", reason)
	}
}
