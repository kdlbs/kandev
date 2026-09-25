package executor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

type concurrentRunningAfterStartingReadRepo struct {
	*mockRepository
	changed bool
}

func (r *concurrentRunningAfterStartingReadRepo) GetTaskSession(
	ctx context.Context,
	id string,
) (*models.TaskSession, error) {
	current, err := r.mockRepository.GetTaskSession(ctx, id)
	if err != nil || current == nil {
		return current, err
	}
	snapshot := cloneMockTaskSession(current)
	if !r.changed && snapshot.State == models.TaskSessionStateStarting {
		r.changed = true
		if err := r.UpdateTaskSessionState(
			ctx, id, models.TaskSessionStateRunning, "",
		); err != nil {
			return nil, err
		}
	}
	return snapshot, nil
}

func TestRollbackResumeStateAfterFailureDoesNotOverwriteConcurrentRunning(t *testing.T) {
	baseRepo := newMockRepository()
	setupLiveResumeTestFixture(baseRepo)
	baseRepo.sessions["sess-1"].State = models.TaskSessionStateStarting
	repo := &concurrentRunningAfterStartingReadRepo{mockRepository: baseRepo}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	exec.SetOnSessionStateTransition(func(
		ctx context.Context,
		_ string,
		sessionID string,
		expectedState *models.TaskSessionState,
		nextState models.TaskSessionState,
		errorMessage string,
		_ func(),
	) (bool, models.TaskSessionState, error) {
		current, err := repo.GetTaskSession(ctx, sessionID)
		if err != nil {
			return false, "", err
		}
		if expectedState == nil {
			expectedState = &current.State
		}
		changed, _, err := repo.UpdateTaskSessionStateIfCurrent(
			ctx, sessionID, *expectedState, nextState, errorMessage,
		)
		return changed, repo.sessions[sessionID].State, err
	})

	exec.rollbackResumeStateAfterFailure(
		context.Background(),
		"task-1",
		"sess-1",
		models.TaskSessionStateRunning,
		errors.New("launch failed"),
		nil,
	)

	if got := baseRepo.sessions["sess-1"].State; got != models.TaskSessionStateRunning {
		t.Fatalf("session state after concurrent transition = %s, want %s", got, models.TaskSessionStateRunning)
	}
}

func TestTerminalRollbackState(t *testing.T) {
	tests := []struct {
		name       string
		priorState models.TaskSessionState
		want       models.TaskSessionState
	}{
		{name: "running redirects to failed", priorState: models.TaskSessionStateRunning, want: models.TaskSessionStateFailed},
		{name: "starting redirects to failed", priorState: models.TaskSessionStateStarting, want: models.TaskSessionStateFailed},
		{name: "failed is preserved", priorState: models.TaskSessionStateFailed, want: models.TaskSessionStateFailed},
		{name: "cancelled is preserved", priorState: models.TaskSessionStateCancelled, want: models.TaskSessionStateCancelled},
		{name: "waiting for input is preserved", priorState: models.TaskSessionStateWaitingForInput, want: models.TaskSessionStateWaitingForInput},
		{name: "idle is preserved", priorState: models.TaskSessionStateIdle, want: models.TaskSessionStateIdle},
		{name: "created is preserved", priorState: models.TaskSessionStateCreated, want: models.TaskSessionStateCreated},
		{name: "completed is preserved", priorState: models.TaskSessionStateCompleted, want: models.TaskSessionStateCompleted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := terminalRollbackState(tt.priorState); got != tt.want {
				t.Fatalf("terminalRollbackState(%s) = %s, want %s", tt.priorState, got, tt.want)
			}
		})
	}
}

func TestResumeSession_RedirectsRunningToFailedWhenRelaunchFails(t *testing.T) {
	repo := newMockRepository()
	setupLiveResumeTestFixture(repo)
	repo.sessions["sess-1"].State = models.TaskSessionStateRunning
	repo.sessions["sess-1"].UpdatedAt = time.Now().Add(-time.Minute)

	launchErr := errors.New("repository workspace failed validation")
	agentMgr := &mockAgentManager{
		launchAgentFunc: func(ctx context.Context, _ *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			current, err := repo.GetTaskSession(ctx, "sess-1")
			if err != nil {
				return nil, err
			}
			if current.State != models.TaskSessionStateStarting {
				return nil, fmt.Errorf("session state at launch = %s, want %s", current.State, models.TaskSessionStateStarting)
			}
			return nil, launchErr
		},
	}
	exec := newTestExecutor(t, agentMgr, repo)
	var transitionCalls int
	exec.SetOnSessionStateTransition(func(
		ctx context.Context,
		_ string,
		sessionID string,
		expectedState *models.TaskSessionState,
		nextState models.TaskSessionState,
		errorMessage string,
		_ func(),
	) (bool, models.TaskSessionState, error) {
		transitionCalls++
		if expectedState == nil || *expectedState != models.TaskSessionStateStarting {
			t.Fatalf("expected state = %v, want STARTING", expectedState)
		}
		changed, _, err := repo.UpdateTaskSessionStateIfCurrent(
			ctx, sessionID, *expectedState, nextState, errorMessage,
		)
		return changed, repo.sessions[sessionID].State, err
	})

	if _, err := exec.ResumeSession(context.Background(), repo.sessions["sess-1"], true); !errors.Is(err, launchErr) {
		t.Fatalf("ResumeSession error = %v, want %v", err, launchErr)
	}
	if transitionCalls != 1 {
		t.Fatalf("state transition callback calls = %d, want 1", transitionCalls)
	}
	current := repo.sessions["sess-1"]
	if current.State != models.TaskSessionStateFailed {
		t.Fatalf("session state after failed relaunch = %s, want %s", current.State, models.TaskSessionStateFailed)
	}
	if !strings.Contains(current.ErrorMessage, launchErr.Error()) {
		t.Fatalf("session error = %q, want launch error", current.ErrorMessage)
	}
}
