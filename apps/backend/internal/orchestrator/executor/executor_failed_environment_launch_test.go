package executor

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.4
func TestLaunchPreparedSessionRematerializesOwnFailedEnvironment(t *testing.T) {
	repo := newMockRepository()
	// A start that returned early released this session's claim and left it CREATED.
	env := seedOwnedCreatingClaim(repo, models.TaskSessionStateCreated)
	env.Status = models.TaskEnvironmentStatusFailed
	env.MaterializationSessionID = ""
	launched := 0
	exec := newTestExecutor(t, &mockAgentManager{
		launchAgentFunc: func(_ context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			launched++
			if req.TaskEnvironmentID != releaseEnvID {
				t.Errorf("launch environment = %q, want %q", req.TaskEnvironmentID, releaseEnvID)
			}
			return &LaunchAgentResponse{AgentExecutionID: "exec-rematerialized", Status: v1.AgentStatusStarting}, nil
		},
	}, repo)

	if _, err := exec.LaunchPreparedSession(context.Background(), releaseTask(), releaseSessionID,
		LaunchOptions{AgentProfileID: "profile-123", StartAgent: true}); err != nil {
		t.Fatalf("LaunchPreparedSession: %v", err)
	}
	if launched != 1 {
		t.Fatalf("runtime launches = %d, want 1", launched)
	}
	stored := repo.taskEnvironments[releaseEnvID]
	if stored.Status != models.TaskEnvironmentStatusReady || stored.MaterializationSessionID != "" {
		t.Fatalf("environment = status %q owner %q, want ready after re-materialization", stored.Status, stored.MaterializationSessionID)
	}
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.5
func TestLaunchPreparedSessionKeepsFailedEnvironmentWithEvidenceClosed(t *testing.T) {
	repo := newMockRepository()
	env := seedOwnedCreatingClaim(repo, models.TaskSessionStateFailed)
	env.Status = models.TaskEnvironmentStatusFailed
	env.MaterializationSessionID = ""
	env.WorkspacePath = "/tasks/release/repo"
	launched := false
	exec := newTestExecutor(t, &mockAgentManager{
		launchAgentFunc: func(context.Context, *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			launched = true
			return &LaunchAgentResponse{}, nil
		},
	}, repo)

	_, err := exec.LaunchPreparedSession(context.Background(), releaseTask(), releaseSessionID,
		LaunchOptions{AgentProfileID: "profile-123", StartAgent: true})

	if err == nil {
		t.Fatal("LaunchPreparedSession succeeded against a failed environment with a workspace")
	}
	if launched {
		t.Fatal("runtime launched against a failed environment with a workspace")
	}
	if env.Status != models.TaskEnvironmentStatusFailed || env.WorkspacePath == "" {
		t.Fatalf("environment = status %q path %q, want the failed row unchanged", env.Status, env.WorkspacePath)
	}
}
