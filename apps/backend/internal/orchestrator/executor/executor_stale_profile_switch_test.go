package executor

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestLaunchPreparedSession_ProfileSwitchIgnoresMissingStaleExecution(t *testing.T) {
	repo := newMockRepository()
	session := &models.TaskSession{
		ID: "session-123", TaskID: "task-123", AgentProfileID: "office-cto",
		State: models.TaskSessionStateCreated, StartedAt: time.Now(), UpdatedAt: time.Now(),
	}
	repo.sessions[session.ID] = session
	repo.executorsRunning[session.ID] = &models.ExecutorRunning{
		ID: session.ID, SessionID: session.ID, TaskID: session.TaskID,
		AgentExecutionID: "stale-codex-exec", ExecutionProfileID: "codex-profile",
	}
	launched := false
	agentManager := &mockAgentManager{
		stopAgentFunc: func(context.Context, string, bool) error {
			return fmt.Errorf("stale execution: %w", lifecycle.ErrExecutionNotFound)
		},
		launchAgentFunc: func(context.Context, *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			launched = true
			return &LaunchAgentResponse{AgentExecutionID: "claude-exec"}, nil
		},
	}
	exec := newTestExecutor(t, agentManager, repo)
	exec.SetOnExecutionReplacementStopOwnerRegistration(func(context.Context, string, string) bool { return true })
	task := &v1.Task{ID: session.TaskID, WorkspaceID: "workspace-123", Title: "Test Task"}

	_, err := exec.LaunchPreparedSession(context.Background(), task, session.ID, LaunchOptions{
		AgentProfileID: "claude-profile", OfficeAgentProfileID: "office-cto", StartAgent: true,
	})
	if err != nil {
		t.Fatalf("LaunchPreparedSession: %v", err)
	}
	if !launched {
		t.Fatal("expected profile switch to continue with a fresh launch")
	}
}
