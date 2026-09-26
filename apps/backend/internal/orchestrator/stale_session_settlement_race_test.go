package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// The task moves after reconciliation reads its step but before the settlement
// transaction commits. The old turn may close, but it cannot complete step2.
func TestSettleStaleSessionMoveBeforeCommitSupersedesOldIntent(t *testing.T) {
	ctx := context.Background()
	svc, repo, request := seedStaleSettlement(t)
	stepGetter := newMockStepGetter()
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
		Events: wfmodels.StepEvents{
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{{Type: wfmodels.OnTurnCompleteMoveToNext}},
		},
	}
	stepGetter.steps["step3"] = &wfmodels.WorkflowStep{ID: "step3", WorkflowID: "wf1", Name: "Step 3", Position: 2}
	svc.SetWorkflowStepGetter(stepGetter)
	svc.repo = &moveBeforeSettlementCommitRepository{Repository: repo, t: t}

	if _, err := svc.SettleStaleSession(ctx, request); err != nil {
		t.Fatalf("SettleStaleSession: %v", err)
	}
	intent, err := repo.GetCompletionIntent(ctx, "intent-1")
	if err != nil || intent.State != models.CompletionIntentStateSuperseded {
		t.Fatalf("intent = (%+v, %v), want superseded", intent, err)
	}
	task, err := repo.GetTask(ctx, "t1")
	if err != nil || task.WorkflowStepID != "step2" {
		t.Fatalf("task = (%+v, %v), want unchanged destination step2", task, err)
	}
	turn, err := repo.GetTurn(ctx, "turn-1")
	if err != nil || turn.CompletedAt == nil {
		t.Fatalf("turn = (%+v, %v), want completed old turn", turn, err)
	}
	var result string
	if err := repo.DB().QueryRowContext(ctx, `SELECT result FROM session_control_events WHERE target_turn_id = ?`, "turn-1").Scan(&result); err != nil || result != string(models.CompletionIntentStateSuperseded) {
		t.Fatalf("audit result = (%q, %v), want superseded", result, err)
	}
}

func TestSettleStaleSessionMoveAfterCommitDoesNotEvaluateDestination(t *testing.T) {
	ctx := context.Background()
	svc, repo, request := seedStaleSettlement(t)
	stepGetter := newMockStepGetter()
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
		Events: wfmodels.StepEvents{
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{{Type: wfmodels.OnTurnCompleteMoveToNext}},
		},
	}
	stepGetter.steps["step3"] = &wfmodels.WorkflowStep{ID: "step3", WorkflowID: "wf1", Name: "Step 3", Position: 2}
	svc.SetWorkflowStepGetter(stepGetter)
	agentMgr := svc.agentManager.(*mockAgentManager)
	agentMgr.isAgentRunning = true
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	session.AgentProfileID = "profile-a"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("UpdateTaskSession: %v", err)
	}
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	seedExecutorRunning(t, repo, "s1", "t1", "exec-post-commit-move")
	dispatched := make(chan struct{})
	svc.onQueuedMessageExecutionComplete = func() { close(dispatched) }
	svc.turnService = &moveAfterSettlementCommitTurnService{
		TurnService: svc.turnService, repo: repo, t: t,
		afterMove: func(ctx context.Context) error {
			_, err := svc.messageQueue.QueueMessage(ctx, "s1", "t1", "destination handoff", "", messagequeue.QueuedByWorkflow, true, nil)
			return err
		},
	}

	if _, err := svc.SettleStaleSession(ctx, request); err != nil {
		t.Fatalf("SettleStaleSession: %v", err)
	}
	task, err := repo.GetTask(ctx, "t1")
	if err != nil || task.WorkflowStepID != "step2" {
		t.Fatalf("task = (%+v, %v), want unchanged destination step2", task, err)
	}
	select {
	case <-dispatched:
	case <-time.After(3 * time.Second):
		t.Fatal("destination handoff stayed queued after the old turn settled")
	}
	if len(agentMgr.capturedPrompts) != 1 || strings.Count(agentMgr.capturedPrompts[0], "destination handoff") != 1 {
		t.Fatalf("dispatched prompts = %q, want one destination handoff", agentMgr.capturedPrompts)
	}
}

type moveAfterSettlementCommitTurnService struct {
	TurnService
	repo      *sqliterepo.Repository
	t         *testing.T
	afterMove func(context.Context) error
}

func (s *moveAfterSettlementCommitTurnService) CompleteTurn(ctx context.Context, turnID string) error {
	s.t.Helper()
	task, err := s.repo.GetTask(ctx, "t1")
	if err != nil {
		s.t.Fatalf("GetTask after commit: %v", err)
	}
	task.WorkflowStepID = "step2"
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		s.t.Fatalf("UpdateTask after settlement commit: %v", err)
	}
	if s.afterMove != nil {
		if err := s.afterMove(ctx); err != nil {
			s.t.Fatalf("queue destination handoff: %v", err)
		}
	}
	return s.TurnService.CompleteTurn(ctx, turnID)
}

type moveBeforeSettlementCommitRepository struct {
	*sqliterepo.Repository
	t *testing.T
}

func (r *moveBeforeSettlementCommitRepository) CompleteTurnAndTransitionCompletionIntent(
	ctx context.Context, turnID, intentID string, from models.CompletionIntentState, settledAt time.Time,
	event *models.SessionControlEvent,
) (models.CompletionIntentState, bool, error) {
	r.t.Helper()
	task, err := r.GetTask(ctx, "t1")
	if err != nil {
		r.t.Fatalf("GetTask before move: %v", err)
	}
	task.WorkflowStepID = "step2"
	if err := r.UpdateTask(ctx, task); err != nil {
		r.t.Fatalf("UpdateTask before settlement commit: %v", err)
	}
	return r.Repository.CompleteTurnAndTransitionCompletionIntent(ctx, turnID, intentID, from, settledAt, event)
}
