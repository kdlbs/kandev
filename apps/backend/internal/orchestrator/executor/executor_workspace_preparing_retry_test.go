package executor

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// workspacePreparingRepo implements the transactional workspace-binding creator
// the production repository provides, so a test can drive PrepareSession through
// the ErrWorkspacePreparing path. Its onBinding hook lets the first binding be
// refused as a lost election and later calls succeed.
type workspacePreparingRepo struct {
	*mockRepository
	onBinding          func(call int) error
	calls              int
	sharedGroupIDs     []string
	boundEnvironmentID string
	onSession          func(*models.TaskSession)
}

func (r *workspacePreparingRepo) CreateTaskSessionWithWorkspaceBinding(
	ctx context.Context, session *models.TaskSession, candidate *models.TaskEnvironment,
) error {
	r.calls++
	if r.onBinding != nil {
		if err := r.onBinding(r.calls); err != nil {
			return err
		}
	}
	if r.onSession != nil {
		r.onSession(session)
	}
	session.TaskEnvironmentID = r.boundEnvironmentID
	return r.CreateTaskSession(ctx, session)
}

func (r *workspacePreparingRepo) CreateTaskSessionWithSharedGroupWorkspaceBinding(
	ctx context.Context, session *models.TaskSession, candidate *models.TaskEnvironment, groupID string,
) error {
	r.sharedGroupIDs = append(r.sharedGroupIDs, groupID)
	return r.CreateTaskSessionWithWorkspaceBinding(ctx, session, candidate)
}

func preparingRefusal(environmentID string) error {
	return &models.WorkspacePreparingError{EnvironmentID: environmentID, Reason: "retry after the workspace launch completes"}
}

// A launch that lost the workspace election must wait for the elected
// environment to become ready and retry the binding once, instead of failing
// the run. The shared-group case uses an environment owned by another member
// task, which a lookup by the launching task would not find.
func TestPrepareSessionRetriesAfterWorkspacePreparing(t *testing.T) {
	sharedGroup := map[string]interface{}{
		"workspace": map[string]interface{}{"mode": "shared_group", "group_id": "group-retry"},
	}
	for _, tc := range []struct {
		name       string
		envTask    string
		loseTask   string
		metadata   map[string]interface{}
		wantGroups []string
	}{
		{name: "same task", envTask: "task-retry", loseTask: "task-retry"},
		{
			name: "shared group member", envTask: "task-group-winner", loseTask: "task-group-loser",
			metadata: sharedGroup, wantGroups: []string{"group-retry", "group-retry"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &workspacePreparingRepo{mockRepository: newMockRepository()}
			env := &models.TaskEnvironment{
				ID:                       "env-retry",
				TaskID:                   tc.envTask,
				Status:                   models.TaskEnvironmentStatusCreating,
				MaterializationSessionID: "winner-session",
			}
			repo.taskEnvironments[env.ID] = env
			repo.boundEnvironmentID = env.ID
			lookups := 0
			repo.getTaskEnvironmentFunc = func(_ context.Context, id string) (*models.TaskEnvironment, error) {
				require.Equal(t, env.ID, id)
				lookups++
				if lookups > 1 {
					env.Status = models.TaskEnvironmentStatusReady
				}
				return env, nil
			}
			repo.onBinding = func(call int) error {
				if call == 1 {
					return preparingRefusal(env.ID)
				}
				return nil
			}

			exec := newTestExecutor(t, &mockAgentManager{}, repo)
			task := &v1.Task{ID: tc.loseTask, WorkspaceID: "ws-retry", Title: "t", Description: "d", Metadata: tc.metadata}

			sessionID, err := exec.PrepareSession(context.Background(), task, "profile-1", "executor-1", "", "")
			require.NoError(t, err)
			require.NotEmpty(t, sessionID)
			require.Equal(t, 2, repo.calls, "the binding must be retried once after the workspace becomes ready")
			require.Equal(t, tc.wantGroups, repo.sharedGroupIDs, "a shared-group member must bind through the group binder")
			require.GreaterOrEqual(t, lookups, 2, "the retry must wait for the environment to leave creating")
			require.NotNil(t, repo.sessions[sessionID])
			require.Equal(t, env.ID, repo.sessions[sessionID].TaskEnvironmentID)
		})
	}
}

// A workspace that reaches failed must keep the original refusal so the run
// still fails as it did before.
func TestPrepareSessionKeepsWorkspacePreparingWhenEnvironmentFails(t *testing.T) {
	repo := &workspacePreparingRepo{mockRepository: newMockRepository()}
	env := &models.TaskEnvironment{
		ID:                       "env-failed",
		TaskID:                   "task-failed",
		Status:                   models.TaskEnvironmentStatusCreating,
		MaterializationSessionID: "winner-session",
	}
	repo.taskEnvironments[env.ID] = env
	lookups := 0
	repo.getTaskEnvironmentFunc = func(_ context.Context, id string) (*models.TaskEnvironment, error) {
		require.Equal(t, env.ID, id)
		lookups++
		if lookups > 1 {
			env.Status = models.TaskEnvironmentStatusFailed
		}
		return env, nil
	}
	repo.onBinding = func(int) error { return preparingRefusal(env.ID) }

	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	task := &v1.Task{ID: "task-failed", WorkspaceID: "ws-failed", Title: "t", Description: "d"}

	_, err := exec.PrepareSession(context.Background(), task, "profile-1", "executor-1", "", "")
	require.ErrorIs(t, err, models.ErrWorkspacePreparing)
	require.Equal(t, 1, repo.calls, "a failed workspace must not be retried")
}

// Any other binding error, or a preparing refusal that names no environment,
// keeps its existing single-attempt behavior.
func TestPrepareSessionDoesNotRetryOtherBindingErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "reuse unsafe", err: models.ErrWorkspaceReuseUnsafe},
		{name: "preparing without environment", err: models.ErrWorkspacePreparing},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &workspacePreparingRepo{mockRepository: newMockRepository()}
			repo.onBinding = func(int) error { return tc.err }

			exec := newTestExecutor(t, &mockAgentManager{}, repo)
			task := &v1.Task{ID: "task-other", WorkspaceID: "ws-other", Title: "t", Description: "d"}

			_, err := exec.PrepareSession(context.Background(), task, "profile-1", "executor-1", "", "")
			require.ErrorIs(t, err, tc.err)
			require.Equal(t, 1, repo.calls, "the binding must not be retried")
		})
	}
}

// Waiting on the environment this session already owns would wait on itself.
func TestRetryPreparedSessionAfterWorkspaceReadySkipsSelfOwnedEnvironment(t *testing.T) {
	repo := newMockRepository()
	repo.taskEnvironments["env-self"] = &models.TaskEnvironment{
		ID: "env-self", TaskID: "task-self", Status: models.TaskEnvironmentStatusCreating,
		MaterializationSessionID: "session-self",
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	created := false
	err := exec.retryPreparedSessionAfterWorkspaceReady(
		context.Background(), "session-self", preparingRefusal("env-self"),
		func() error { created = true; return nil },
	)
	require.ErrorIs(t, err, models.ErrWorkspacePreparing)
	require.False(t, created, "an environment this session owns must not be awaited")
}

// A workspace that never becomes ready within the caller's deadline keeps the
// original refusal rather than blocking forever.
func TestRetryPreparedSessionAfterWorkspaceReadyTimesOut(t *testing.T) {
	repo := newMockRepository()
	repo.taskEnvironments["env-stuck"] = &models.TaskEnvironment{
		ID: "env-stuck", TaskID: "task-stuck", Status: models.TaskEnvironmentStatusCreating,
		MaterializationSessionID: "winner-session",
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	created := false
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := exec.retryPreparedSessionAfterWorkspaceReady(
		ctx, "session-stuck", preparingRefusal("env-stuck"),
		func() error { created = true; return nil },
	)
	require.ErrorIs(t, err, models.ErrWorkspacePreparing)
	require.False(t, created, "a workspace that never becomes ready must not be retried")
}

// Both starts saw no session and planned to become the task's first, primary
// session. The election winner creates that session while the loser waits, so
// the loser's retry must leave the winner primary and must not claim the
// first-session origin.
func TestPrepareSessionRetryKeepsTheWinnerPrimary(t *testing.T) {
	repo := &workspacePreparingRepo{mockRepository: newMockRepository()}
	env := &models.TaskEnvironment{
		ID:                       "env-primary",
		TaskID:                   "task-primary",
		Status:                   models.TaskEnvironmentStatusCreating,
		MaterializationSessionID: "winner-session",
	}
	repo.taskEnvironments[env.ID] = env
	winner := &models.TaskSession{
		ID: "winner-session", TaskID: "task-primary", IsPrimary: true,
		Metadata: map[string]interface{}{models.SessionMetaKeyOrigin: models.SessionOriginTaskInitial},
	}
	lookups := 0
	repo.getTaskEnvironmentFunc = func(ctx context.Context, id string) (*models.TaskEnvironment, error) {
		lookups++
		if lookups == 2 {
			require.NoError(t, repo.CreateTaskSession(ctx, winner))
			env.Status = models.TaskEnvironmentStatusReady
		}
		return env, nil
	}
	repo.onBinding = func(call int) error {
		if call == 1 {
			return preparingRefusal(env.ID)
		}
		return nil
	}

	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	task := &v1.Task{ID: "task-primary", WorkspaceID: "ws-primary", Title: "t", Description: "d"}

	sessionID, err := exec.PrepareSession(context.Background(), task, "profile-1", "executor-1", "", "")
	require.NoError(t, err)
	loser := repo.sessions[sessionID]
	require.NotNil(t, loser)
	require.False(t, loser.IsPrimary, "the retried session must not take the primary flag")
	require.True(t, repo.sessions[winner.ID].IsPrimary, "the winner must stay primary")
	require.NotContains(t, repo.setSessionPrimaryCalls, sessionID)
	_, hasOrigin := loser.Metadata[models.SessionMetaKeyOrigin]
	require.False(t, hasOrigin, "the retried session must not claim the first-session origin")
}
