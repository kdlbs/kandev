package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func failedLaunchEnvironment(taskID string) *models.TaskEnvironment {
	return &models.TaskEnvironment{
		ID: releaseEnvID, TaskID: taskID, ExecutorType: string(models.ExecutorTypeLocal),
		Status: models.TaskEnvironmentStatusFailed,
	}
}

func releaseSession() *models.TaskSession {
	return &models.TaskSession{ID: releaseSessionID, TaskID: releaseTaskID, TaskEnvironmentID: releaseEnvID}
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.10
func TestLaunchPreparedSessionFailedEnvironmentWithEvidenceRequiresReset(t *testing.T) {
	repo := newMockRepository()
	env := seedOwnedCreatingClaim(repo, models.TaskSessionStateFailed)
	env.Status = models.TaskEnvironmentStatusFailed
	env.MaterializationSessionID = ""
	env.WorkspacePath = "/tasks/release/repo"
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	_, err := exec.LaunchPreparedSession(context.Background(), releaseTask(), releaseSessionID,
		LaunchOptions{AgentProfileID: "profile-123", StartAgent: true})

	if !errors.Is(err, models.ErrTaskEnvironmentResetRequired) {
		t.Fatalf("LaunchPreparedSession error = %v, want reset required", err)
	}
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.5
func TestReelectFailedEnvironmentForLaunchKeepsInheritedEnvironmentClosed(t *testing.T) {
	repo := newMockRepository()
	env := failedLaunchEnvironment("parent-task")
	repo.taskEnvironments[env.ID] = env
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	_, err := exec.reelectFailedTaskEnvironmentForLaunch(context.Background(), releaseTask(), releaseSession(), env)

	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) || errors.Is(err, models.ErrTaskEnvironmentResetRequired) {
		t.Fatalf("inherited failed environment error = %v, want reuse unsafe without reset guidance", err)
	}
	if len(repo.reelectFailedCalls) != 0 {
		t.Fatalf("re-election requested for an inherited environment: %v", repo.reelectFailedCalls)
	}
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.6
func TestReelectFailedEnvironmentForLaunchAdoptsWinnerAfterLostRace(t *testing.T) {
	repo := newMockRepository()
	env := failedLaunchEnvironment(releaseTaskID)
	repo.taskEnvironments[env.ID] = env
	repo.reelectFailedFunc = func(context.Context, string, string) (bool, error) {
		env.Status = models.TaskEnvironmentStatusReady
		env.WorkspacePath = "/tasks/release/repo"
		return false, nil
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	got, err := exec.reelectFailedTaskEnvironmentForLaunch(context.Background(), releaseTask(), releaseSession(), env)

	if err != nil {
		t.Fatalf("lost re-election error = %v, want the winner's ready environment", err)
	}
	if got.Status != models.TaskEnvironmentStatusReady {
		t.Fatalf("adopted environment status = %q, want ready", got.Status)
	}
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.6
func TestReelectFailedEnvironmentForLaunchWaitsForWinnerAfterLostRace(t *testing.T) {
	repo := newMockRepository()
	env := failedLaunchEnvironment(releaseTaskID)
	repo.reelectFailedFunc = func(context.Context, string, string) (bool, error) { return false, nil }
	lookups := 0
	repo.getTaskEnvironmentFunc = func(context.Context, string) (*models.TaskEnvironment, error) {
		lookups++
		current := *env
		current.Status = models.TaskEnvironmentStatusCreating
		current.MaterializationSessionID = "session-winner"
		if lookups > 2 {
			current.Status = models.TaskEnvironmentStatusReady
			current.MaterializationSessionID = ""
		}
		return &current, nil
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	got, err := exec.reelectFailedTaskEnvironmentForLaunch(context.Background(), releaseTask(), releaseSession(), env)

	if err != nil {
		t.Fatalf("lost re-election error = %v, want to wait for the winner", err)
	}
	if got.Status != models.TaskEnvironmentStatusReady {
		t.Fatalf("awaited environment status = %q, want ready", got.Status)
	}
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.10
func TestReelectFailedEnvironmentForLaunchRequiresResetWhenEvidenceAppears(t *testing.T) {
	repo := newMockRepository()
	env := failedLaunchEnvironment(releaseTaskID)
	env.Repos = []*models.TaskEnvironmentRepo{{ID: "row-1", WorktreeID: "worktree-1", Status: "active"}}
	repo.taskEnvironments[env.ID] = env
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	_, err := exec.reelectFailedTaskEnvironmentForLaunch(context.Background(), releaseTask(), releaseSession(), env)

	if !errors.Is(err, models.ErrTaskEnvironmentResetRequired) {
		t.Fatalf("failed environment with inventory error = %v, want reset required", err)
	}
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.6
func TestReelectFailedEnvironmentForLaunchRequiresResetWhenKubernetesEvidenceBlocksReelection(t *testing.T) {
	repo := newMockRepository()
	env := failedLaunchEnvironment(releaseTaskID)
	repo.taskEnvironments[env.ID] = env
	// The store refuses the re-election because the Kubernetes inventory row
	// recorded retained runtime metadata the environment snapshot cannot
	// carry; the row itself still looks empty to this check.
	repo.reelectFailedFunc = func(context.Context, string, string) (bool, error) { return false, nil }
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	_, err := exec.reelectFailedTaskEnvironmentForLaunch(context.Background(), releaseTask(), releaseSession(), env)

	if !errors.Is(err, models.ErrTaskEnvironmentResetRequired) {
		t.Fatalf("refused failed re-election error = %v, want reset required", err)
	}
}
