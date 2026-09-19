package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type cancelingReservedTurnAttemptMarker struct {
	TurnService
	cancel context.CancelFunc
	err    error
}

func (s cancelingReservedTurnAttemptMarker) MarkReservedTurnDispatchAttempted(
	context.Context,
	*models.Turn,
) error {
	s.cancel()
	return s.err
}

type cancelingStartTurnService struct {
	TurnService
	cancel context.CancelFunc
	err    error
}

func (s cancelingStartTurnService) StartTurn(context.Context, string) (*models.Turn, error) {
	s.cancel()
	return nil, s.err
}

func TestDetachedClarificationRollsBackClaimAfterMarkerCallCancelsContext(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const taskID = "task-marker-cancel"
	const sessionID = "session-marker-cancel"
	seedSession(t, repo, taskID, sessionID, "step-1")
	seedExecutorRunning(t, repo, sessionID, taskID, "exec-marker-cancel")
	if err := repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateWaitingForInput, ""); err != nil {
		t.Fatalf("set session waiting: %v", err)
	}

	requestCtx, cancel := context.WithCancel(ctx)
	persistenceErr := errors.New("persist dispatch attempt marker")
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createEngineService(t, repo, newMockStepGetter(), agentMgr)
	svc.turnService = cancelingReservedTurnAttemptMarker{
		TurnService: &repoBackedTurnService{repo: repo},
		cancel:      cancel,
		err:         persistenceErr,
	}

	err := svc.ResumeDetachedClarification(requestCtx, clarification.DetachedClarificationResume{
		TaskID: taskID, SessionID: sessionID, PendingID: "pending-marker-cancel",
		Question: "Continue?", AnswerText: "Continue",
	})
	if !errors.Is(err, persistenceErr) {
		t.Fatalf("resume error = %v, want %v", err, persistenceErr)
	}
	if pending := svc.reservedPromptTurnID(sessionID); pending != "" {
		t.Fatalf("private reservation = %q, want rollback after marker failure", pending)
	}
	turns, err := repo.ListTurnsBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("list turns after marker failure: %v", err)
	}
	if len(turns) != 0 {
		t.Fatalf("turns after marker failure = %#v, want rolled-back reservation", turns)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("load session after marker failure: %v", err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state after marker failure = %q, want waiting", session.State)
	}
}

func TestLifecyclePromptRollsBackClaimAfterTurnCallCancelsContext(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const taskID = "task-turn-cancel"
	const sessionID = "session-turn-cancel"
	seedSession(t, repo, taskID, sessionID, "step-1")
	if err := repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateWaitingForInput, ""); err != nil {
		t.Fatalf("set session waiting: %v", err)
	}

	requestCtx, cancel := context.WithCancel(ctx)
	persistenceErr := errors.New("persist lifecycle turn")
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateReview)
	svc := createTestService(repo, newMockStepGetter(), taskRepo)
	svc.turnService = cancelingStartTurnService{
		TurnService: &repoBackedTurnService{repo: repo},
		cancel:      cancel,
		err:         persistenceErr,
	}

	_, _, err := svc.claimLifecyclePromptDispatch(requestCtx, taskID, sessionID, "", nil)
	if !errors.Is(err, persistenceErr) {
		t.Fatalf("claim error = %v, want %v", err, persistenceErr)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("load session after turn failure: %v", err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state after turn failure = %q, want waiting", session.State)
	}
	task, err := taskRepo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("load task after turn failure: %v", err)
	}
	if task.State != v1.TaskStateReview {
		t.Fatalf("task state after turn failure = %q, want review", task.State)
	}
}

func TestOrdinaryPromptRollsBackClaimAfterTurnCallCancelsContext(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const taskID = "task-ordinary-turn-cancel"
	const sessionID = "session-ordinary-turn-cancel"
	seedSession(t, repo, taskID, sessionID, "step-1")
	if err := repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateWaitingForInput, ""); err != nil {
		t.Fatalf("set session waiting: %v", err)
	}

	requestCtx, cancel := context.WithCancel(ctx)
	persistenceErr := errors.New("persist ordinary turn")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.turnService = cancelingStartTurnService{
		TurnService: &repoBackedTurnService{repo: repo},
		cancel:      cancel,
		err:         persistenceErr,
	}

	_, _, err := svc.claimPromptDispatch(
		requestCtx,
		taskID,
		sessionID,
		"",
		false,
		false,
		nil,
		nil,
		nil,
		"",
		false,
	)
	if !errors.Is(err, persistenceErr) {
		t.Fatalf("claim error = %v, want %v", err, persistenceErr)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("load session after turn failure: %v", err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state after turn failure = %q, want waiting", session.State)
	}
}
