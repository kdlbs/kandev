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

func TestRollbackResumeStateAfterFailureDoesNotOverwriteConcurrentRunning(t *testing.T) {
	repo := newMockRepository()
	setupLiveResumeTestFixture(repo)
	repo.sessions["sess-1"].State = models.TaskSessionStateStarting
	repo.sessions["sess-1"].Metadata = map[string]interface{}{
		models.SessionMetaKeyAgentStartAttemptID: "attempt-old",
	}
	repo.updateTaskSessionResumeStateIfCurrentAttemptFunc = func(
		_ context.Context,
		_, sessionID, _ string,
		_, _ models.TaskSessionState,
		_ string,
		_, _, _ bool,
		_ interface{},
	) (bool, time.Time, error) {
		current := repo.sessions[sessionID]
		current.State = models.TaskSessionStateRunning
		current.Metadata[models.SessionMetaKeyAgentStartAttemptID] = "successor-attempt"
		return false, time.Time{}, nil
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	var releases int
	exec.SetOnCeilingReservationRelease(func(string) { releases++ })

	exec.rollbackResumeStateAfterFailure(
		context.Background(),
		"task-1",
		"sess-1",
		"attempt-old",
		models.TaskSessionStateRunning,
		errors.New("launch failed"),
		nil,
	)

	if got := repo.sessions["sess-1"].State; got != models.TaskSessionStateRunning {
		t.Fatalf("session state after concurrent transition = %s, want %s", got, models.TaskSessionStateRunning)
	}
	if releases != 0 {
		t.Fatalf("reservation releases = %d, want 0 for superseded attempt", releases)
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
	exec.SetOnResumeFailureRollback(func(ctx context.Context, request ResumeFailureRollbackRequest) (bool, error) {
		transitionCalls++
		if request.ExpectedState != models.TaskSessionStateStarting || request.NextState != models.TaskSessionStateFailed {
			t.Fatalf("rollback transition = %s -> %s, want STARTING -> FAILED", request.ExpectedState, request.NextState)
		}
		updater := resumeStateAttemptUpdater(repo)
		changed, _, err := updater.UpdateTaskSessionResumeStateIfCurrentAttempt(
			ctx, request.TaskID, request.SessionID, request.AttemptID,
			request.ExpectedState, request.NextState, request.ErrorMessage,
			true, request.CredentialSnapshot != nil,
			request.CredentialSnapshot != nil && request.CredentialSnapshot.Present,
			credentialSnapshotValue(request.CredentialSnapshot),
		)
		return changed, err
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
