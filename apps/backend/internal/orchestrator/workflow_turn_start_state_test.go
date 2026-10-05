package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-TASKS-RESUME-PROMPT-QUEUE-001.10
func TestPrepareWorkflowTurnStartSessionState(t *testing.T) {
	for _, tc := range []struct {
		state     models.TaskSessionState
		wantState models.TaskSessionState
		wantTask  v1.TaskState
		wantError string
	}{
		{state: models.TaskSessionStateCreated, wantState: models.TaskSessionStateWaitingForInput, wantTask: v1.TaskStateReview},
		{state: models.TaskSessionStateIdle, wantState: models.TaskSessionStateWaitingForInput, wantTask: v1.TaskStateReview},
		{state: models.TaskSessionStateStarting, wantState: models.TaskSessionStateStarting, wantTask: v1.TaskStateInProgress},
		{state: models.TaskSessionStateRunning, wantState: models.TaskSessionStateRunning, wantTask: v1.TaskStateInProgress},
		{state: models.TaskSessionStateWaitingForInput, wantState: models.TaskSessionStateWaitingForInput, wantTask: v1.TaskStateInProgress},
		{state: models.TaskSessionStateFailed, wantState: models.TaskSessionStateFailed, wantTask: v1.TaskStateInProgress, wantError: "resume failed"},
		{state: models.TaskSessionStateCancelled, wantState: models.TaskSessionStateCancelled, wantTask: v1.TaskStateInProgress, wantError: "cancelled"},
		{state: models.TaskSessionStateCompleted, wantState: models.TaskSessionStateCompleted, wantTask: v1.TaskStateInProgress, wantError: "completed"},
	} {
		t.Run(string(tc.state), func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedSession(t, repo, "turn-start-task", "turn-start-session", "step1")
			if err := repo.UpdateTaskSessionState(ctx, "turn-start-session", tc.state, tc.wantError); err != nil {
				t.Fatalf("set initial state: %v", err)
			}
			taskRepo := newMockTaskRepo()
			seedMockTaskState(taskRepo, "turn-start-task", v1.TaskStateInProgress)
			svc := createTestService(repo, newMockStepGetter(), taskRepo)

			if err := svc.prepareWorkflowTurnStartSessionState(ctx, "turn-start-task", "turn-start-session"); err != nil {
				t.Fatalf("prepare turn-start state: %v", err)
			}

			session, err := repo.GetTaskSession(ctx, "turn-start-session")
			if err != nil {
				t.Fatalf("load session: %v", err)
			}
			if session.State != tc.wantState {
				t.Fatalf("session state = %s, want %s", session.State, tc.wantState)
			}
			if session.ErrorMessage != tc.wantError {
				t.Fatalf("session error = %q, want %q", session.ErrorMessage, tc.wantError)
			}
			taskRepo.mu.Lock()
			gotTaskState := taskRepo.updatedStates["turn-start-task"]
			taskRepo.mu.Unlock()
			if gotTaskState == "" {
				gotTaskState = v1.TaskStateInProgress
			}
			if gotTaskState != tc.wantTask {
				t.Fatalf("task state publication = %s, want %s", gotTaskState, tc.wantTask)
			}
		})
	}
}

type turnStartCASRaceRepository struct {
	*sqlite.Repository
}

func (r *turnStartCASRaceRepository) UpdateTaskSessionStateIfCurrent(
	ctx context.Context,
	sessionID string,
	expected, next models.TaskSessionState,
	errorMessage string,
) (bool, time.Time, error) {
	if expected == models.TaskSessionStateCreated && next == models.TaskSessionStateWaitingForInput {
		if err := r.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateStarting, "resume claimed startup"); err != nil {
			return false, time.Time{}, err
		}
	}
	return r.Repository.UpdateTaskSessionStateIfCurrent(ctx, sessionID, expected, next, errorMessage)
}

// @covers AC-TASKS-RESUME-PROMPT-QUEUE-001.10
func TestPrepareWorkflowTurnStartSessionStatePreservesLostWrite(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "turn-start-task", "turn-start-session", "step1")
	if err := repo.UpdateTaskSessionState(ctx, "turn-start-session", models.TaskSessionStateCreated, ""); err != nil {
		t.Fatalf("set created state: %v", err)
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "turn-start-task", v1.TaskStateInProgress)
	svc := createTestService(repo, newMockStepGetter(), taskRepo)
	svc.repo = &turnStartCASRaceRepository{Repository: repo}

	if err := svc.prepareWorkflowTurnStartSessionState(ctx, "turn-start-task", "turn-start-session"); err != nil {
		t.Fatalf("prepare turn-start state after losing conditional write: %v", err)
	}

	session, err := repo.GetTaskSession(ctx, "turn-start-session")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if session.State != models.TaskSessionStateStarting {
		t.Fatalf("session state = %s, want concurrent STARTING state", session.State)
	}
	if session.ErrorMessage != "resume claimed startup" {
		t.Fatalf("session error = %q, want resume claim preserved", session.ErrorMessage)
	}
	taskRepo.mu.Lock()
	defer taskRepo.mu.Unlock()
	if writes := taskRepo.stateHistory["turn-start-task"]; len(writes) != 0 {
		t.Fatalf("task state writes = %v, want none after losing conditional write", writes)
	}
}

type turnStartReadFailureRepository struct {
	*sqlite.Repository
	err error
}

func (r *turnStartReadFailureRepository) GetTaskSession(context.Context, string) (*models.TaskSession, error) {
	return nil, r.err
}

// @covers AC-TASKS-RESUME-PROMPT-QUEUE-001.11
func TestPrepareWorkflowTurnStartSessionStateRejectsForeignOrUnreadableRecipient(t *testing.T) {
	ctx := context.Background()
	t.Run("foreign task", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "turn-start-task", "turn-start-session", "step1")
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

		if err := svc.prepareWorkflowTurnStartSessionState(ctx, "other-task", "turn-start-session"); err == nil {
			t.Fatal("prepare turn-start state succeeded for a foreign task")
		}
		session, err := repo.GetTaskSession(ctx, "turn-start-session")
		if err != nil {
			t.Fatalf("load session: %v", err)
		}
		if session.State != models.TaskSessionStateRunning {
			t.Fatalf("foreign session state = %s, want RUNNING", session.State)
		}
	})

	t.Run("read failure", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "turn-start-task", "turn-start-session", "step1")
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		svc.repo = &turnStartReadFailureRepository{Repository: repo, err: errors.New("session read failed")}

		if err := svc.prepareWorkflowTurnStartSessionState(ctx, "turn-start-task", "turn-start-session"); err == nil {
			t.Fatal("prepare turn-start state succeeded after a session read failure")
		}
		session, err := repo.GetTaskSession(ctx, "turn-start-session")
		if err != nil {
			t.Fatalf("load session: %v", err)
		}
		if session.State != models.TaskSessionStateRunning {
			t.Fatalf("unreadable session state = %s, want RUNNING", session.State)
		}
	})
}
