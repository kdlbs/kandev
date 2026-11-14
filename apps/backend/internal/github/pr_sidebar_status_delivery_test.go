package github

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/statussummary"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type summaryPublication struct {
	TaskID      string
	WorkspaceID string
	Summary     statussummary.TaskStatusSummary
}

// newPRSidebarDeliveryTest wires the real watch store, the real service apply
// path, the real event bus, and the real task-status-summary projector.
func newPRSidebarDeliveryTest(t *testing.T) (*Service, *taskrepo.Repository, *progressBatchExecutor, func() []summaryPublication) {
	t.Helper()
	ctx := context.Background()
	database, _ := prWatchRetirementTaskTemplate.Open(t)
	taskRepo := taskrepo.NewWithInitializedDB(database, database, nil)
	store, err := NewStore(sqlx.NewDb(taskRepo.DB(), "sqlite3"), sqlx.NewDb(taskRepo.DB(), "sqlite3"))
	require.NoError(t, err)

	seedPRSidebarDeliveryTask(t, taskRepo, "task-inactive-pr", "session-inactive-pr", "feature/inactive", v1.TaskStateReview)
	seedPRSidebarDeliveryTask(t, taskRepo, "task-searching-sibling", "session-searching-sibling", "feature/searching", v1.TaskStateReview)

	log, logErr := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, logErr)
	eventBus := bus.NewMemoryEventBus(log)
	executor := &progressBatchExecutor{prNumber: progressTestPRNumber}
	client := &progressGraphQLClient{MockClient: NewMockClient(), inner: executor}
	service := NewService(client, AuthMethodPAT, nil, store, eventBus, log)
	t.Cleanup(service.Stop)
	configureTestWorkspaceAuth(t, service, client, testWorkspaceID)
	service.SetTaskIssueStore(taskRepo)

	projector := statussummary.NewProjector(statussummary.ProjectorConfig{
		Store:    taskRepo,
		EventBus: eventBus,
		ResolveWorkspace: func(ctx context.Context, taskID string) (string, error) {
			task, err := taskRepo.GetTask(ctx, taskID)
			if err != nil || task == nil {
				return "", err
			}
			return task.WorkspaceID, nil
		},
		LoadPullRequests: func(ctx context.Context, taskID string) ([]statussummary.PullRequestInput, error) {
			return loadTaskStatusSummaryPRs(ctx, service, taskID)
		},
		Logger:       log,
		RetryBackoff: func(context.Context, int) error { return nil },
	})
	_, err = eventBus.Subscribe(events.GitHubTaskPRUpdated, func(ctx context.Context, event *bus.Event) error {
		return projector.HandleEvent(ctx, event)
	})
	require.NoError(t, err)

	var publications []summaryPublication
	_, err = eventBus.Subscribe(events.TaskStatusSummaryUpdated, func(_ context.Context, event *bus.Event) error {
		payload, ok := event.Data.(statussummary.SummaryUpdated)
		if !ok {
			return nil
		}
		publications = append(publications, summaryPublication{
			TaskID: payload.TaskID, WorkspaceID: payload.WorkspaceID, Summary: payload.Summary,
		})
		return nil
	})
	require.NoError(t, err)

	watch := withTestWorkspace(&PRWatch{
		SessionID: "session-inactive-pr", TaskID: "task-inactive-pr",
		RepositoryID: "repo-1", Owner: "acme", Repo: "widget",
		PRNumber: progressTestPRNumber, Branch: "feature/inactive", LastCheckStatus: "pending",
	})
	require.NoError(t, store.CreatePRWatch(ctx, watch))
	searching := withTestWorkspace(&PRWatch{
		SessionID: "session-searching-sibling", TaskID: "task-searching-sibling",
		RepositoryID: "repo-1", Owner: "acme", Repo: "widget", Branch: "feature/searching",
	})
	require.NoError(t, store.CreatePRWatch(ctx, searching))

	return service, taskRepo, executor, func() []summaryPublication {
		return append([]summaryPublication(nil), publications...)
	}
}

func loadTaskStatusSummaryPRs(
	ctx context.Context, service *Service, taskID string,
) ([]statussummary.PullRequestInput, error) {
	rows, err := service.ListTaskPRsByTaskIDs(ctx, []string{taskID})
	if err != nil {
		return nil, err
	}
	inputs := make([]statussummary.PullRequestInput, 0, len(rows[taskID]))
	for _, pr := range rows[taskID] {
		if pr == nil {
			continue
		}
		inputs = append(inputs, statussummary.PullRequestInput{
			Key:                   fmt.Sprintf("%s#%d", pr.RepositoryID, pr.PRNumber),
			Owner:                 pr.Owner,
			Repo:                  pr.Repo,
			State:                 pr.State,
			Number:                pr.PRNumber,
			URL:                   pr.PRURL,
			ReviewState:           pr.ReviewState,
			ChecksState:           pr.ChecksState,
			MergeableState:        pr.MergeableState,
			UnresolvedReviewCount: pr.UnresolvedReviewThreads,
			PendingReviewCount:    pr.PendingReviewCount,
			ChecksTotal:           pr.ChecksTotal,
			ChecksPassing:         pr.ChecksPassing,
			HeadSHA:               pr.HeadSHA,
		})
	}
	return inputs, nil
}

func seedPRSidebarDeliveryTask(
	t *testing.T, repo *taskrepo.Repository, taskID, sessionID, branch string, state v1.TaskState,
) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	if taskID == "task-inactive-pr" {
		require.NoError(t, repo.CreateWorkspace(ctx, &taskmodels.Workspace{
			ID: testWorkspaceID, Name: "Sidebar delivery", CreatedAt: now, UpdatedAt: now,
		}))
		require.NoError(t, repo.CreateWorkflow(ctx, &taskmodels.Workflow{
			ID: "workflow-sidebar", WorkspaceID: testWorkspaceID, Name: "Sidebar delivery",
			CreatedAt: now, UpdatedAt: now,
		}))
		require.NoError(t, repo.CreateRepository(ctx, &taskmodels.Repository{
			ID: "repo-1", WorkspaceID: testWorkspaceID, Name: "widget", SourceType: "provider",
			Provider: "github", ProviderOwner: "acme", ProviderName: "widget", CreatedAt: now, UpdatedAt: now,
		}))
	}
	require.NoError(t, repo.CreateTask(ctx, &taskmodels.Task{
		ID: taskID, WorkspaceID: testWorkspaceID, WorkflowID: "workflow-sidebar",
		WorkflowStepID: "step-review", Title: taskID, State: state, CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &taskmodels.TaskSession{
		ID: sessionID, TaskID: taskID, State: taskmodels.TaskSessionStateRunning, StartedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskRepository(ctx, &taskmodels.TaskRepository{
		ID: "task-repo-" + taskID, TaskID: taskID, RepositoryID: "repo-1",
		CheckoutBranch: branch, CreatedAt: now, UpdatedAt: now,
	}))
}

// @covers AC-INTEGRATIONS-GITHUB-PR-POLLING-001.8
func TestPRSidebarDelivery_InactiveTaskReceivesSummary(t *testing.T) {
	service, taskRepo, executor, publications := newPRSidebarDeliveryTest(t)
	subjects := &fallbackCountingClient{Client: service.client}
	service.client = subjects
	configureTestWorkspaceAuth(t, service, subjects, testWorkspaceID)
	executor.numberedState = []string{"PENDING", "SUCCESS", "SUCCESS"}
	executor.deferredBranches = 1
	ctx := pollerContext(context.Background())

	watches, err := service.store.ListActivePRWatches(ctx)
	require.NoError(t, err)
	require.Len(t, watches, 2)

	beforeTask, err := taskRepo.GetTask(ctx, "task-inactive-pr")
	require.NoError(t, err)
	beforeSession, err := taskRepo.GetTaskSession(ctx, "session-inactive-pr")
	require.NoError(t, err)

	outcome := service.syncWorkspaceWatchesBatchedForPoller(ctx, testWorkspaceID, watches)
	require.ErrorAs(t, outcome.Deferred, new(*AdmissionDeferredError),
		"the searching sibling must still be deferred after the known-PR phase publishes")
	first := publications()
	require.Len(t, first, 1, "the completed known-PR phase publishes one summary")
	require.Equal(t, "task-inactive-pr", first[0].TaskID)
	require.Equal(t, testWorkspaceID, first[0].WorkspaceID)
	require.NotNil(t, first[0].Summary.PullRequest)
	require.Equal(t, progressTestPRNumber, first[0].Summary.PullRequest.Number)
	require.Equal(t, "pending", first[0].Summary.PullRequest.AggregateState)
	firstRevision := first[0].Summary.Revision

	// A searching sibling must not have published anything for its own task.
	for _, publication := range first {
		require.NotEqual(t, "task-searching-sibling", publication.TaskID)
	}
	rows, err := service.store.ListTaskPRsByTask(ctx, "task-searching-sibling")
	require.NoError(t, err)
	require.Empty(t, rows, "deferred discovery must not invent a PR association")

	// The newer provider result reaches a newer summary revision for the
	// inactive task, without any task interaction.
	outcome = service.syncWorkspaceWatchesBatchedForPoller(ctx, testWorkspaceID, watches)
	require.NoError(t, outcome.Err)
	all := publications()
	require.Len(t, all, 2)
	latest := all[len(all)-1]
	require.Equal(t, "task-inactive-pr", latest.TaskID)
	require.Equal(t, "passing", latest.Summary.PullRequest.AggregateState)
	require.Greater(t, latest.Summary.Revision, firstRevision)

	// Repeating unchanged provider evidence must not duplicate the summary.
	outcome = service.syncWorkspaceWatchesBatchedForPoller(ctx, testWorkspaceID, watches)
	require.NoError(t, outcome.Err)
	require.Len(t, publications(), 2, "unchanged state must not republish the summary")

	afterTask, err := taskRepo.GetTask(ctx, "task-inactive-pr")
	require.NoError(t, err)
	require.Equal(t, beforeTask.State, afterTask.State, "provider monitoring must not change task state")
	require.Equal(t, beforeTask.UpdatedAt.UTC(), afterTask.UpdatedAt.UTC())
	afterSession, err := taskRepo.GetTaskSession(ctx, "session-inactive-pr")
	require.NoError(t, err)
	require.Equal(t, beforeSession.State, afterSession.State)
	require.Equal(t, beforeSession.UpdatedAt.UTC(), afterSession.UpdatedAt.UTC())
}
