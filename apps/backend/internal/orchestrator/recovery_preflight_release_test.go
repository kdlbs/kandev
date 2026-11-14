//go:build unix

package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/system/sessioncapacity"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func retainedPreflightFixture(t *testing.T) (*completedRelocationServiceFixture, string, string) {
	t.Helper()
	f := newCompletedRelocationServiceFixture(t, 1)
	ctx := context.Background()
	_, err := f.db.ExecContext(ctx, f.db.Rebind(`UPDATE task_environments SET status = ? WHERE id = ?`),
		string(models.TaskEnvironmentStatusFailed), f.environmentID)
	require.NoError(t, err)
	const firstOwner, sibling = "preflight-first-owner", "preflight-sibling"
	for _, id := range []string{firstOwner, sibling} {
		require.NoError(t, f.repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: id, TaskID: f.taskID, TaskEnvironmentID: f.environmentID, State: models.TaskSessionStateCompleted,
		}))
	}
	retained, err := f.repo.GetTaskEnvironment(ctx, f.environmentID)
	require.NoError(t, err)
	selected, err := f.repo.ReserveTaskEnvironmentReplacement(ctx, retained, &models.TaskSession{
		ID: firstOwner, TaskID: f.taskID, TaskEnvironmentID: f.environmentID, State: models.TaskSessionStateCompleted,
	}, &models.TaskEnvironment{
		TaskID: f.taskID, ExecutorType: string(models.ExecutorTypeWorktree), ExecutorID: models.ExecutorIDWorktree,
		TaskDirName: "preflight-unused-successor",
	})
	require.NoError(t, err)
	released, err := f.repo.FailTaskEnvironmentMaterialization(ctx, firstOwner)
	require.NoError(t, err)
	require.True(t, released)
	return f, selected.ID, sibling
}

func assertPreflightReplacementReleased(t *testing.T, f *completedRelocationServiceFixture, environmentID, sibling string) {
	t.Helper()
	ctx := context.Background()
	environment, err := f.repo.GetTaskEnvironment(ctx, environmentID)
	require.NoError(t, err)
	require.Equal(t, models.TaskEnvironmentStatusFailed, environment.Status)
	require.Empty(t, environment.MaterializationSessionID)
	retained, err := f.repo.GetTaskEnvironment(ctx, f.environmentID)
	require.NoError(t, err)
	require.True(t, retained.IsRetired())
	require.Len(t, retained.Repos, 1)
	selected, err := f.repo.ReserveTaskEnvironmentReplacement(ctx, retained, &models.TaskSession{
		ID: sibling, TaskID: f.taskID, TaskEnvironmentID: f.environmentID, State: models.TaskSessionStateCompleted,
	}, &models.TaskEnvironment{TaskID: f.taskID, ExecutorType: string(models.ExecutorTypeWorktree)})
	require.NoError(t, err, "a sibling must not wait for an unused preflight claim to expire")
	require.Equal(t, environmentID, selected.ID)
	require.Equal(t, sibling, selected.MaterializationSessionID)
}

func TestRecoveryPreflightReleasesReplacementWhenCapacityDefers(t *testing.T) {
	f, environmentID, sibling := retainedPreflightFixture(t)
	f.svc.SetExecutionCapacity(sessioncapacity.Effective{MaxSessions: 1, Policy: sessioncapacity.PolicyAll, GradeLimits: map[string]int{"work": 0}})
	f.svc.sessionCeiling.resolveGrade = func(context.Context, admissionRequest) (string, error) { return "work", nil }
	_, err := f.svc.RecoverSessionWithOptions(context.Background(), f.taskID, f.sessionID, "resume", RecoverSessionOptions{})
	require.NoError(t, err)
	require.Zero(t, f.launchStarts)
	assertPreflightReplacementReleased(t, f, environmentID, sibling)
}

type preflightReloadFailingRepository struct {
	*tasksqlite.Repository
	environmentID string
	failure       error
	beforeFailure func()
}

func (r *preflightReloadFailingRepository) GetTaskEnvironment(ctx context.Context, id string) (*models.TaskEnvironment, error) {
	if id == r.environmentID {
		if r.beforeFailure != nil {
			r.beforeFailure()
		}
		return nil, r.failure
	}
	return r.Repository.GetTaskEnvironment(ctx, id)
}

func TestRecoveryPreflightReleasesReplacementOnInspectionError(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(fmt.Sprintf("canceled=%v", canceled), func(t *testing.T) {
			f, environmentID, sibling := retainedPreflightFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failure := errors.New("selected successor reload failed")
			repo := &preflightReloadFailingRepository{Repository: f.repo, environmentID: environmentID, failure: failure}
			if canceled {
				repo.beforeFailure = cancel
			}
			f.svc.executor = withTestCapabilities(executor.NewExecutor(f.agent, repo, testLogger(), executor.ExecutorConfig{}))
			f.svc.executor.SetRepoCloner(f.cloner, nil)
			_, err := f.svc.RecoverSessionWithOptions(ctx, f.taskID, f.sessionID, "resume", RecoverSessionOptions{})
			require.ErrorContains(t, err, failure.Error())
			require.Zero(t, f.launchStarts)
			assertPreflightReplacementReleased(t, f, environmentID, sibling)
		})
	}
}
