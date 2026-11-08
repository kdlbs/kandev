package executor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// seedProfileChangeLaunch prepares a session whose executors_running row is
// bound to profile-old, so launching it with profile-new must replace the
// previous execution.
func seedProfileChangeLaunch(t *testing.T) (*mockRepository, *models.TaskSession) {
	t.Helper()
	repo := newMockRepository()
	session := &models.TaskSession{
		ID:                 "session-prepared",
		TaskID:             "task-office",
		AgentProfileID:     "agent-1",
		ExecutionProfileID: "profile-old",
		State:              models.TaskSessionStateCreated,
		StartedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}
	repo.sessions[session.ID] = session
	repo.executorsRunning[session.ID] = &models.ExecutorRunning{
		SessionID:          session.ID,
		TaskID:             session.TaskID,
		AgentExecutionID:   "exec-old",
		ExecutionProfileID: "profile-old",
	}
	repo.deleteExecutorRunningFunc = func(_ context.Context, sessionID string) error {
		repo.mu.Lock()
		defer repo.mu.Unlock()
		if _, ok := repo.executorsRunning[sessionID]; !ok {
			return fmt.Errorf("%w for session: %s", models.ErrExecutorRunningNotFound, sessionID)
		}
		delete(repo.executorsRunning, sessionID)
		return nil
	}
	return repo, session
}

func launchWithProfile(exec *Executor, session *models.TaskSession, profileID string) error {
	_, err := exec.LaunchPreparedSession(context.Background(),
		&v1.Task{ID: session.TaskID, WorkspaceID: "ws-1"}, session.ID,
		LaunchOptions{AgentProfileID: profileID, Prompt: "test"})
	return err
}

func TestLaunchPreparedSession_ProfileChangeToleratesRowClearedByStop(t *testing.T) {
	repo, session := seedProfileChangeLaunch(t)
	var stopped string
	launches := 0
	exec := newTestExecutor(t, &mockAgentManager{
		stopAgentWithReasonFunc: func(_ context.Context, executionID, _ string, _ bool) error {
			stopped = executionID
			// The synchronous agent-stopped handler removes the session's row.
			repo.mu.Lock()
			delete(repo.executorsRunning, session.ID)
			repo.mu.Unlock()
			return nil
		},
		launchAgentFunc: func(_ context.Context, _ *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			launches++
			return &LaunchAgentResponse{AgentExecutionID: "exec-new", Status: v1.AgentStatusStarting}, nil
		},
	}, repo)
	exec.SetOnExecutionReplacementStopOwnerRegistration(func(context.Context, string, string) bool { return true })

	if err := launchWithProfile(exec, session, "profile-new"); err != nil {
		t.Fatalf("LaunchPreparedSession() error = %v, want nil", err)
	}
	if stopped != "exec-old" {
		t.Fatalf("stopped execution = %q, want exec-old", stopped)
	}
	if launches != 1 {
		t.Fatalf("LaunchAgent calls = %d, want 1", launches)
	}
}

func TestLaunchPreparedSession_ProfileChangePropagatesOtherDeleteErrors(t *testing.T) {
	repo, session := seedProfileChangeLaunch(t)
	deleteErr := errors.New("environment recovery in progress")
	repo.deleteExecutorRunningFunc = func(context.Context, string) error { return deleteErr }
	launched := false
	exec := newTestExecutor(t, &mockAgentManager{
		launchAgentFunc: func(_ context.Context, _ *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			launched = true
			return &LaunchAgentResponse{AgentExecutionID: "exec-new", Status: v1.AgentStatusStarting}, nil
		},
	}, repo)
	exec.SetOnExecutionReplacementStopOwnerRegistration(func(context.Context, string, string) bool { return true })

	err := launchWithProfile(exec, session, "profile-new")
	if !errors.Is(err, deleteErr) || !strings.Contains(err.Error(), "clear previous execution profile") {
		t.Fatalf("LaunchPreparedSession() error = %v, want wrapped delete error", err)
	}
	if launched {
		t.Fatal("LaunchAgent was called after the previous profile could not be cleared")
	}
}
