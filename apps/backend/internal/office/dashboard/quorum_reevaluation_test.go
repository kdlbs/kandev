package dashboard_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
)

// TestRequestTaskChanges_HumanVetoUnsticksReviewerGuardedStep is the AC-63/10
// end-to-end proof this feature exists for: a human's request-changes veto
// must actually move a reviewer-guarded task off its current step, not just
// record a decision row. Review round 1 flagged that no office-tier test
// could drive this — engine_test_helpers_test.go's fakeSessionResolver
// always reports "no active session," so RecordParticipantDecision's
// re-evaluation subpath (and the TransitionStore it would call) was never
// reached by anything in this package. This test swaps in
// newTestEngineDispatcherWithReevaluation, which wires a real session and a
// real (DB-backed) TransitionStore so the guarded transition can genuinely
// fire.
func TestRequestTaskChanges_HumanVetoUnsticksReviewerGuardedStep(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "vt1", "ws-v", "V", "in_review", 2)

	const fromStep = "step-vt1"
	const toStep = "step-vt1-rework"
	store := newDashboardTransitionStore(deps.db)
	store.steps[fromStep] = engine.StepSpec{
		ID: fromStep, WorkflowID: store.workflowID, Position: 1,
		Events: map[engine.Trigger][]engine.Action{
			engine.TriggerOnTurnComplete: {
				{
					Kind: engine.ActionMoveToStep,
					Guard: &engine.TransitionGuard{
						WaitForQuorum: &engine.WaitForQuorumGuard{
							Role:      models.ParticipantRoleApprover,
							Threshold: engine.QuorumAnyReject,
						},
					},
					MoveToStep: &engine.MoveToStepAction{StepID: toStep},
				},
			},
		},
	}
	session := &taskmodels.TaskSession{ID: "sess-vt1"}
	deps.svc.SetWorkflowEngineDispatcher(
		newTestEngineDispatcherWithReevaluation(deps.wfRepo, logger.Default(), store, "vt1", session),
	)

	_, err := deps.svc.RequestTaskChanges(context.Background(),
		models.DeciderTypeUser, "human-1", "vt1", "please fix the approach")
	if err != nil {
		t.Fatalf("RequestTaskChanges: %v", err)
	}

	var gotStepID string
	if err := deps.db.QueryRow(
		`SELECT workflow_step_id FROM tasks WHERE id = ?`, "vt1",
	).Scan(&gotStepID); err != nil {
		t.Fatalf("read task step: %v", err)
	}
	if gotStepID != toStep {
		t.Fatalf("workflow_step_id = %q, want %q (human veto should have unstuck the reviewer-guarded step)",
			gotStepID, toStep)
	}
}
