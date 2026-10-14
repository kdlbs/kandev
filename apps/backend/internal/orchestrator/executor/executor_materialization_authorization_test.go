package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.7
func TestLaunchPreparedSessionForeignTaskCannotReleaseMaterialization(t *testing.T) {
	repo := newMockRepository()
	env := seedOwnedCreatingClaim(repo, models.TaskSessionStateCreated)
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	_, err := exec.LaunchPreparedSession(context.Background(), &v1.Task{ID: "foreign-task"}, releaseSessionID,
		LaunchOptions{AgentProfileID: "profile-123", StartAgent: true})
	if err == nil {
		t.Fatal("foreign task launch was admitted")
	}
	if env.Status != models.TaskEnvironmentStatusCreating || env.MaterializationSessionID != releaseSessionID {
		t.Fatalf("foreign task changed materialization: status=%q owner=%q", env.Status, env.MaterializationSessionID)
	}
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.1
func TestLaunchPreparedSessionReleasesCommittedReelectionAfterReadFailure(t *testing.T) {
	repo := newMockRepository()
	env := seedOwnedCreatingClaim(repo, models.TaskSessionStateCreated)
	env.Status = models.TaskEnvironmentStatusFailed
	env.MaterializationSessionID = ""
	repo.getTaskEnvironmentFunc = func(context.Context, string) (*models.TaskEnvironment, error) {
		if len(repo.reelectFailedCalls) > 0 {
			return nil, errors.New("environment read failed after election")
		}
		snapshot := *env
		return &snapshot, nil
	}
	launched := false
	exec := newTestExecutor(t, &mockAgentManager{
		launchAgentFunc: func(context.Context, *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			launched = true
			return nil, errors.New("runtime launch was not expected")
		},
	}, repo)
	_, err := exec.LaunchPreparedSession(context.Background(), releaseTask(), releaseSessionID,
		LaunchOptions{AgentProfileID: "profile-123", StartAgent: true})
	if err == nil || len(repo.reelectFailedCalls) != 1 || launched {
		t.Fatalf("post-election read failure: err=%v elections=%d launched=%v", err, len(repo.reelectFailedCalls), launched)
	}
	if env.Status != models.TaskEnvironmentStatusFailed || env.MaterializationSessionID != "" {
		t.Fatalf("committed election remained claimed after read failure: status=%q owner=%q", env.Status, env.MaterializationSessionID)
	}
}
