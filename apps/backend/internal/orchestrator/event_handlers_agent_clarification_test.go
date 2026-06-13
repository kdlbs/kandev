package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

type repoBackedTurnService struct {
	repo testRepo
}

type testRepo interface {
	CreateTurn(ctx context.Context, turn *models.Turn) error
	CompleteTurn(ctx context.Context, id string) error
	GetActiveTurnBySessionID(ctx context.Context, sessionID string) (*models.Turn, error)
	UpdateTurn(ctx context.Context, turn *models.Turn) error
}

func (s *repoBackedTurnService) StartTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	turn := &models.Turn{
		ID:            "turn-" + sessionID,
		TaskSessionID: sessionID,
		StartedAt:     time.Now().UTC(),
	}
	if err := s.repo.CreateTurn(ctx, turn); err != nil {
		return nil, err
	}
	return turn, nil
}

func (s *repoBackedTurnService) CompleteTurn(ctx context.Context, turnID string) error {
	return s.repo.CompleteTurn(ctx, turnID)
}

func (s *repoBackedTurnService) GetActiveTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	return s.repo.GetActiveTurnBySessionID(ctx, sessionID)
}

func (s *repoBackedTurnService) UpdateTurn(ctx context.Context, turn *models.Turn) error {
	return s.repo.UpdateTurn(ctx, turn)
}

func (s *repoBackedTurnService) AbandonOpenTurns(ctx context.Context, sessionID string) error {
	for {
		turn, err := s.repo.GetActiveTurnBySessionID(ctx, sessionID)
		if err != nil || turn == nil {
			return err
		}
		if err := s.repo.CompleteTurn(ctx, turn.ID); err != nil {
			return err
		}
	}
}

func TestHandleAgentReady_BlocksOnTurnCompleteWhileClarificationPending(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	setSessionExecID(t, repo, "s1", "exec-1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Plan", Position: 0,
		Events: wfmodels.StepEvents{
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{
				{Type: wfmodels.OnTurnCompleteMoveToNext},
			},
		},
	}
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Implement", Position: 1,
	}

	svc := createEngineService(t, repo, stepGetter, &mockAgentManager{repoForExecutionLookup: repo})

	t.Run("advances when no pending clarification", func(t *testing.T) {
		svc.handleAgentReady(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"})

		task, err := repo.GetTask(ctx, "t1")
		if err != nil {
			t.Fatalf("get task: %v", err)
		}
		if task.WorkflowStepID != "step2" {
			t.Fatalf("expected workflow step step2, got %q", task.WorkflowStepID)
		}
	})

	t.Run("does not advance while clarification is pending", func(t *testing.T) {
		// Reset task to plan step after the subtest above advanced it.
		task, err := repo.GetTask(ctx, "t1")
		if err != nil {
			t.Fatalf("get task: %v", err)
		}
		task.WorkflowStepID = "step1"
		if err := repo.UpdateTask(ctx, task); err != nil {
			t.Fatalf("reset task step: %v", err)
		}
		session, err := repo.GetTaskSession(ctx, "s1")
		if err != nil {
			t.Fatalf("get session: %v", err)
		}
		session.State = models.TaskSessionStateRunning
		if err := repo.UpdateTaskSession(ctx, session); err != nil {
			t.Fatalf("reset session state: %v", err)
		}

		now := time.Now().UTC()
		requireNoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "turn-clarify", TaskSessionID: "s1", TaskID: "t1", StartedAt: now}))
		requireNoError(t, repo.CreateMessage(ctx, &models.Message{
			ID:            "clarify-1",
			TaskSessionID: "s1",
			TaskID:        "t1",
			TurnID:        "turn-clarify",
			AuthorType:    models.MessageAuthorAgent,
			Type:          "clarification_request",
			Content:       "Which approach?",
			CreatedAt:     now,
			Metadata: map[string]interface{}{
				"pending_id":  "pending-1",
				"question_id": "q1",
				"status":      "pending",
			},
		}))

		svc.handleAgentReady(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"})

		task, err = repo.GetTask(ctx, "t1")
		if err != nil {
			t.Fatalf("get task: %v", err)
		}
		if task.WorkflowStepID != "step1" {
			t.Fatalf("expected workflow step to remain step1, got %q", task.WorkflowStepID)
		}

		session, err = repo.GetTaskSession(ctx, "s1")
		if err != nil {
			t.Fatalf("get session: %v", err)
		}
		if session.State != models.TaskSessionStateWaitingForInput {
			t.Fatalf("expected session %q, got %q", models.TaskSessionStateWaitingForInput, session.State)
		}
	})
}

func TestHandleAgentCompleted_BlocksOnTurnCompleteWhileClarificationPending(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	setSessionExecID(t, repo, "s1", "exec-1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Plan", Position: 0,
		Events: wfmodels.StepEvents{
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{
				{Type: wfmodels.OnTurnCompleteMoveToNext},
			},
		},
	}
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Implement", Position: 1,
	}

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, stepGetter, newMockTaskRepo(), agentMgr)
	svc.turnService = &repoBackedTurnService{repo: repo}

	t.Run("advances when no pending clarification", func(t *testing.T) {
		svc.handleAgentCompleted(ctx, watcher.AgentEventData{
			TaskID:           "t1",
			SessionID:        "s1",
			AgentExecutionID: "exec-1",
		})

		task, err := repo.GetTask(ctx, "t1")
		if err != nil {
			t.Fatalf("get task: %v", err)
		}
		if task.WorkflowStepID != "step2" {
			t.Fatalf("expected workflow step step2, got %q", task.WorkflowStepID)
		}
	})

	t.Run("does not advance while clarification is pending", func(t *testing.T) {
		task, err := repo.GetTask(ctx, "t1")
		if err != nil {
			t.Fatalf("get task: %v", err)
		}
		task.WorkflowStepID = "step1"
		if err := repo.UpdateTask(ctx, task); err != nil {
			t.Fatalf("reset task step: %v", err)
		}
		session, err := repo.GetTaskSession(ctx, "s1")
		if err != nil {
			t.Fatalf("get session: %v", err)
		}
		session.State = models.TaskSessionStateRunning
		if err := repo.UpdateTaskSession(ctx, session); err != nil {
			t.Fatalf("reset session state: %v", err)
		}

		now := time.Now().UTC()
		requireNoError(t, repo.CreateTurn(ctx, &models.Turn{
			ID:            "turn-clarify-completed",
			TaskSessionID: "s1",
			TaskID:        "t1",
			StartedAt:     now,
		}))
		requireNoError(t, repo.CreateMessage(ctx, &models.Message{
			ID:            "clarify-completed-1",
			TaskSessionID: "s1",
			TaskID:        "t1",
			TurnID:        "turn-clarify-completed",
			AuthorType:    models.MessageAuthorAgent,
			Type:          "clarification_request",
			Content:       "Which approach?",
			CreatedAt:     now,
			Metadata: map[string]interface{}{
				"pending_id":  "pending-1",
				"question_id": "q1",
				"status":      "pending",
			},
		}))

		svc.handleAgentCompleted(ctx, watcher.AgentEventData{
			TaskID:           "t1",
			SessionID:        "s1",
			AgentExecutionID: "exec-1",
		})

		task, err = repo.GetTask(ctx, "t1")
		if err != nil {
			t.Fatalf("get task: %v", err)
		}
		if task.WorkflowStepID != "step1" {
			t.Fatalf("expected workflow step to remain step1, got %q", task.WorkflowStepID)
		}

		session, err = repo.GetTaskSession(ctx, "s1")
		if err != nil {
			t.Fatalf("get session: %v", err)
		}
		if session.State != models.TaskSessionStateWaitingForInput {
			t.Fatalf("expected session %q, got %q", models.TaskSessionStateWaitingForInput, session.State)
		}
		turn, err := svc.turnService.GetActiveTurn(ctx, "s1")
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("get active turn: %v", err)
		}
		if turn != nil {
			t.Fatalf("expected active turn to be completed before deferring, got %q", turn.ID)
		}
	})
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
