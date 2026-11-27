package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestResumeSessionSynchronousStartFailureRespectsAttemptIdentity(t *testing.T) {
	for _, successor := range []bool{false, true} {
		name := "owned-attempt"
		if successor {
			name = "successor-attempt"
		}
		t.Run(name, func(t *testing.T) {
			repo := newMockRepository()
			setupLiveResumeTestFixture(repo)
			startErr := errors.New("synchronous start failed")
			agentManager := &mockAgentManager{
				launchAgentFunc: func(context.Context, *LaunchAgentRequest) (*LaunchAgentResponse, error) {
					return &LaunchAgentResponse{
						AgentExecutionID: "exec-resumed",
						WorkspacePath:    "/tasks/task-1/materialized",
						Status:           v1.AgentStatusStarting,
					}, nil
				},
				startAgentProcessFunc: func(context.Context, string) error {
					if successor {
						repo.sessions["sess-1"].Metadata[models.SessionMetaKeyAgentStartAttemptID] = "successor"
					}
					return startErr
				},
			}
			exec := newTestExecutor(t, agentManager, repo)
			cleanupClaims := 0
			exec.SetOnExecutionCleanupClaim(func(sessionID, executionID string) bool {
				cleanupClaims++
				if sessionID != "sess-1" || executionID != "exec-resumed" {
					t.Fatalf("cleanup claim = (%q, %q), want exact resume execution", sessionID, executionID)
				}
				return true
			})
			execution, err := exec.ResumeSessionWithOptions(context.Background(), repo.sessions["sess-1"], true,
				ResumeOptions{StartAgentSynchronously: true})
			if !errors.Is(err, startErr) {
				t.Fatalf("ResumeSessionWithOptions error = %v, want %v", err, startErr)
			}
			if execution == nil || execution.AgentExecutionID != "exec-resumed" {
				t.Fatalf("ResumeSessionWithOptions execution = %+v, want allocated execution on startup failure", execution)
			}
			want := models.TaskSessionStateWaitingForInput
			if successor {
				if !errors.Is(err, ErrSessionStateSuperseded) {
					t.Fatalf("ResumeSessionWithOptions error = %v, want superseded attempt cause", err)
				}
				want = models.TaskSessionStateStarting
				if got := repo.sessions["sess-1"].Metadata[models.SessionMetaKeyAgentStartAttemptID]; got != "successor" {
					t.Fatalf("attempt identity = %v, want successor", got)
				}
			}
			if got := repo.sessions["sess-1"].State; got != want {
				t.Fatalf("session state = %s, want %s", got, want)
			}
			if cleanupClaims != 1 {
				t.Fatalf("execution cleanup claims = %d, want 1", cleanupClaims)
			}
		})
	}
}

func TestResumeSessionSynchronousStartFailurePreservesFailedStateDuringCleanup(t *testing.T) {
	repo := newMockRepository()
	setupLiveResumeTestFixture(repo)
	repo.sessions["sess-1"].State = models.TaskSessionStateFailed
	ownerRegistered := false
	stopCalls := 0
	agentManager := &mockAgentManager{
		launchAgentFunc: func(context.Context, *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			return &LaunchAgentResponse{
				AgentExecutionID: "exec-resumed",
				WorkspacePath:    "/tasks/task-1/materialized",
				Status:           v1.AgentStatusStarting,
			}, nil
		},
		startAgentProcessFunc: func(context.Context, string) error {
			return errors.New("synchronous start failed")
		},
		stopAgentWithReasonFunc: func(context.Context, string, string, bool) error {
			stopCalls++
			if !ownerRegistered {
				repo.sessions["sess-1"].State = models.TaskSessionStateCancelled
			}
			return nil
		},
	}
	exec := newTestExecutor(t, agentManager, repo)
	exec.SetOnExecutionCleanupClaim(func(sessionID, executionID string) bool {
		if sessionID != "sess-1" || executionID != "exec-resumed" {
			t.Fatalf("cleanup claim = (%q, %q), want exact resume execution", sessionID, executionID)
		}
		ownerRegistered = true
		return true
	})

	_, err := exec.ResumeSessionWithOptions(context.Background(), repo.sessions["sess-1"], true,
		ResumeOptions{StartAgentSynchronously: true})
	if err == nil {
		t.Fatal("ResumeSessionWithOptions succeeded, want synchronous start failure")
	}
	if stopCalls != 1 {
		t.Fatalf("cleanup stop calls = %d, want 1", stopCalls)
	}
	if got := repo.sessions["sess-1"].State; got != models.TaskSessionStateFailed {
		t.Fatalf("session state = %s, want FAILED after failed resume", got)
	}
}

func TestResumeSessionSynchronousStartFailureDoesNotDuplicateOwnedCancellation(t *testing.T) {
	repo := newMockRepository()
	setupLiveResumeTestFixture(repo)
	repo.sessions["sess-1"].State = models.TaskSessionStateFailed
	stopCalls := 0
	agentManager := &mockAgentManager{
		launchAgentFunc: func(context.Context, *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			return &LaunchAgentResponse{
				AgentExecutionID: "exec-resumed",
				WorkspacePath:    "/tasks/task-1/materialized",
				Status:           v1.AgentStatusStarting,
			}, nil
		},
		startAgentProcessFunc: func(context.Context, string) error {
			repo.sessions["sess-1"].State = models.TaskSessionStateCancelled
			return errors.New("synchronous start failed")
		},
		stopAgentWithReasonFunc: func(context.Context, string, string, bool) error {
			stopCalls++
			return nil
		},
	}
	exec := newTestExecutor(t, agentManager, repo)
	exec.SetOnExecutionCleanupClaim(func(sessionID, executionID string) bool {
		if sessionID != "sess-1" || executionID != "exec-resumed" {
			t.Fatalf("cleanup claim = (%q, %q), want exact resume execution", sessionID, executionID)
		}
		return false
	})

	_, err := exec.ResumeSessionWithOptions(context.Background(), repo.sessions["sess-1"], true,
		ResumeOptions{StartAgentSynchronously: true})
	if !errors.Is(err, ErrSessionStateSuperseded) {
		t.Fatalf("ResumeSessionWithOptions error = %v, want superseded cancellation", err)
	}
	if stopCalls != 0 {
		t.Fatalf("cleanup stop calls = %d, want 0 after another owner claimed cancellation", stopCalls)
	}
	if got := repo.sessions["sess-1"].State; got != models.TaskSessionStateCancelled {
		t.Fatalf("session state = %s, want CANCELLED", got)
	}
}
