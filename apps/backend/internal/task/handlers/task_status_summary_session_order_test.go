package handlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/task/statussummary"
)

func TestTaskDTOEnrichmentReconcilesWithSessionsReadAfterStatusSummary(t *testing.T) {
	for _, testCase := range []struct {
		name          string
		before        models.TaskSessionState
		after         models.TaskSessionState
		beforeRunning bool
		afterRunning  bool
	}{
		{
			name:          "secondary session starts after the old batch",
			before:        models.TaskSessionStateWaitingForInput,
			after:         models.TaskSessionStateRunning,
			beforeRunning: false,
			afterRunning:  true,
		},
		{
			name:          "secondary session stops after the old batch",
			before:        models.TaskSessionStateRunning,
			after:         models.TaskSessionStateWaitingForInput,
			beforeRunning: true,
			afterRunning:  false,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			calls := []string{}
			repo := &statusSummarySessionOrderRepo{
				httpTaskRepo: &httpTaskRepo{},
				calls:        &calls,
				session: &models.TaskSession{
					ID: "secondary", TaskID: "task-b", State: testCase.before,
				},
			}
			summaries := &statusSummarySessionOrderStore{
				calls: &calls,
				current: statussummary.TaskStatusSummary{
					Revision: 1, HasRunningSession: boolPointer(testCase.beforeRunning),
				},
			}
			// The summary read is the interleaving point: the initial handler batch
			// has the old session state when this callback advances both records.
			summaries.beforeRead = func() {
				repo.session.State = testCase.after
				summaries.current = statussummary.TaskStatusSummary{
					Revision: 2, HasRunningSession: boolPointer(testCase.afterRunning),
				}
			}

			svc := service.NewService(service.Repos{
				Workspaces: repo, Tasks: repo, TaskRepos: repo,
				Workflows: repo, Messages: repo, Turns: repo,
				Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
				Executors: repo, Environments: repo, TaskEnvironments: repo,
				Reviews: repo, ResourceCleanups: repo, StatusSummaries: summaries,
			}, nil, newTestLogger(t), service.RepositoryDiscoveryConfig{})

			_, err := buildTaskDTOsWithSessionInfo(
				context.Background(), svc, newTestLogger(t), nil, nil,
				[]*models.Task{{ID: "task-b", WorkspaceID: "ws-b", Title: "Task"}},
			)
			require.NoError(t, err)
			require.GreaterOrEqual(t, len(calls), 3)
			require.Equal(t, []string{"sessions", "status-summary", "sessions"}, calls[:3],
				"the repair observation must follow the summary revision it may replace")
			require.Equal(t, uint64(2), summaries.current.Revision,
				"the handler must not publish a repair based on the earlier session batch")
			require.Equal(t, testCase.afterRunning, *summaries.current.HasRunningSession)
			require.Zero(t, summaries.writeCalls)
		})
	}
}

func TestTaskSummaryProjectionCombinesPrimaryAndSessionListReads(t *testing.T) {
	repo := &taskSummaryProjectionNavigationRepo{
		httpTaskRepo: &httpTaskRepo{},
		observations: map[string][]*models.TaskSessionSummaryObservation{
			"task-projection": {{
				ID: "session-projection-primary", TaskID: "task-projection", IsPrimary: true,
				State: models.TaskSessionStateWaitingForInput, ReviewStatus: models.ReviewStatusApproved,
				ExecutorID: "executor-projection", ExecutorProfileID: "executor-profile-projection",
				ExecutorType: "local", ExecutorName: "Local runner", AgentProfileID: "agent-profile-projection",
				AgentProfileName: "Coding agent", RepositoryPath: "/workspace/project",
			}, {
				ID: "session-projection-secondary", TaskID: "task-projection",
				State: models.TaskSessionStateRunning,
			}},
		},
	}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo, Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo, Executors: repo, Environments: repo,
		TaskEnvironments: repo, Reviews: repo, ResourceCleanups: repo,
	}, nil, newTestLogger(t), service.RepositoryDiscoveryConfig{})

	got, err := buildTaskDTOsWithSessionInfo(
		context.Background(), svc, newTestLogger(t), nil, nil,
		[]*models.Task{{ID: "task-projection", WorkspaceID: "ws-projection", Title: "Projection"}},
	)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.NotNil(t, got[0].PrimarySessionID)
	require.NotNil(t, got[0].SessionCount)
	require.Equal(t, "session-projection-primary", *got[0].PrimarySessionID)
	require.Equal(t, 2, *got[0].SessionCount)
	require.Equal(t, "WAITING_FOR_INPUT", *got[0].PrimarySessionState)
	require.Equal(t, models.ReviewStatusApproved, got[0].ReviewStatus)
	require.Equal(t, "executor-projection", *got[0].PrimaryExecutorID)
	require.Equal(t, "Local runner", *got[0].PrimaryExecutorName)
	require.Equal(t, "Coding agent", *got[0].PrimaryAgentName)
	require.Equal(t, "/workspace/project", *got[0].PrimaryWorkingDirectory)
	require.Equal(t, 2, repo.summaryCalls, "the post-status-summary observation must use the same narrow projection")
	require.Zero(t, repo.fullSessionBatchCalls, "navigation must not decode full session rows")
	require.Zero(t, repo.primaryInfoCalls, "primary information must come from the existing summary batch")
}

type taskSummaryProjectionNavigationRepo struct {
	*httpTaskRepo
	observations          map[string][]*models.TaskSessionSummaryObservation
	summaryCalls          int
	fullSessionBatchCalls int
	primaryInfoCalls      int
}

func (r *taskSummaryProjectionNavigationRepo) BatchGetTaskSessionSummaryObservations(
	_ context.Context,
	taskIDs []string,
) (map[string][]*models.TaskSessionSummaryObservation, error) {
	r.summaryCalls++
	result := make(map[string][]*models.TaskSessionSummaryObservation, len(taskIDs))
	for _, taskID := range taskIDs {
		result[taskID] = r.observations[taskID]
	}
	return result, nil
}

func (r *taskSummaryProjectionNavigationRepo) ListTaskSessionSummaryObservations(
	_ context.Context,
	taskID string,
) ([]*models.TaskSessionSummaryObservation, error) {
	return r.observations[taskID], nil
}

func (r *taskSummaryProjectionNavigationRepo) BatchGetSessionsByTaskIDs(
	ctx context.Context,
	taskIDs []string,
) (map[string][]*models.TaskSession, error) {
	r.fullSessionBatchCalls++
	return r.httpTaskRepo.BatchGetSessionsByTaskIDs(ctx, taskIDs)
}

func (r *taskSummaryProjectionNavigationRepo) GetPrimarySessionInfoByTaskIDs(
	ctx context.Context,
	taskIDs []string,
) (map[string]*models.TaskSession, error) {
	r.primaryInfoCalls++
	return r.httpTaskRepo.GetPrimarySessionInfoByTaskIDs(ctx, taskIDs)
}

type statusSummarySessionOrderRepo struct {
	*httpTaskRepo
	calls   *[]string
	session *models.TaskSession
}

func boolPointer(value bool) *bool {
	return &value
}

func (r *statusSummarySessionOrderRepo) BatchGetSessionsByTaskIDs(
	_ context.Context,
	taskIDs []string,
) (map[string][]*models.TaskSession, error) {
	*r.calls = append(*r.calls, "sessions")
	for _, taskID := range taskIDs {
		if taskID == r.session.TaskID {
			copy := *r.session
			return map[string][]*models.TaskSession{taskID: {&copy}}, nil
		}
	}
	return map[string][]*models.TaskSession{}, nil
}

type statusSummarySessionOrderStore struct {
	calls      *[]string
	current    statussummary.TaskStatusSummary
	beforeRead func()
	writeCalls int
}

func (r *statusSummarySessionOrderStore) LoadTaskStatusSummaries(
	_ context.Context,
	taskIDs []string,
) (map[string]*statussummary.TaskStatusSummary, error) {
	*r.calls = append(*r.calls, "status-summary")
	if r.beforeRead != nil {
		r.beforeRead()
		r.beforeRead = nil
	}
	result := make(map[string]*statussummary.TaskStatusSummary, len(taskIDs))
	for _, taskID := range taskIDs {
		copy := r.current
		result[taskID] = &copy
	}
	return result, nil
}

func (r *statusSummarySessionOrderStore) CompareAndUpdateTaskStatusSummary(
	_ context.Context,
	stored *statussummary.StoredTaskStatusSummary,
) (bool, error) {
	r.writeCalls++
	if stored.Summary.Revision != r.current.Revision+1 {
		return false, nil
	}
	r.current = stored.Summary
	return true, nil
}

func (r *statusSummarySessionOrderStore) DeleteTaskStatusSummary(context.Context, string) error {
	return nil
}

var _ repository.TaskStatusSummaryRepository = (*statusSummarySessionOrderStore)(nil)
