package github

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
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

const workflowAttentionMonitorTaskCount = 30

var (
	workflowAttentionStatusRepo = regexp.MustCompile(`repo(\d+): repository\(owner: "([^"]+)", name: "([^"]+)"\)`)
	workflowAttentionStatusPR   = regexp.MustCompile(`pr(\d+): pullRequest\(number: (\d+)\)`)
)

// workflowAttentionMonitorExecutor serves the status batch and delegates the
// dedicated workflow queries to the workflow fixture.
type workflowAttentionMonitorExecutor struct {
	statusCalls     atomic.Int64
	branchCalls     atomic.Int64
	branchDeferrals int
	workflow        *workflowTestExecutor
}

func (e *workflowAttentionMonitorExecutor) ExecuteGraphQL(
	ctx context.Context, query string, variables map[string]any, out any,
) error {
	switch {
	case strings.Contains(query, "query WorkflowSuites"), strings.Contains(query, "query WorkflowSuiteJobs"):
		return e.workflow.ExecuteGraphQL(ctx, query, variables, out)
	case strings.Contains(query, "query Branches"):
		e.branchCalls.Add(1)
		if e.branchDeferrals > 0 {
			e.branchDeferrals--
			return &AdmissionDeferredError{Delay: time.Minute, Reason: "test branch deferral"}
		}
		return json.Unmarshal([]byte(`{"data":{}}`), out)
	case strings.Contains(query, "query Batch"):
		e.statusCalls.Add(1)
		return e.respondStatusBatch(query, out)
	default:
		return fmt.Errorf("unexpected monitor test query: %s", query)
	}
}

func (e *workflowAttentionMonitorExecutor) respondStatusBatch(query string, out any) error {
	repoAlias := "repo0"
	if match := workflowAttentionStatusRepo.FindStringSubmatch(query); len(match) > 0 {
		repoAlias = "repo" + match[1]
	}
	prs := make(map[string]any)
	for _, match := range workflowAttentionStatusPR.FindAllStringSubmatch(query, -1) {
		number, _ := strconv.Atoi(match[2])
		prs["pr"+match[1]] = workflowAttentionStatusFixture(number)
	}
	payload, err := json.Marshal(map[string]any{"data": map[string]any{repoAlias: prs}})
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, out)
}

func workflowAttentionStatusFixture(number int) map[string]any {
	taskNumber := number - 5000
	head := fmt.Sprintf("head-%d", taskNumber)
	branch := fmt.Sprintf("feature-%d", taskNumber)
	return map[string]any{
		"state": "OPEN", "title": fmt.Sprintf("PR %d", number),
		"url":     fmt.Sprintf("https://github.com/acme/widget/pull/%d", number),
		"isDraft": false, "mergeable": "MERGEABLE", "mergeStateStatus": "CLEAN",
		"headRefName": branch, "baseRefName": "main", "headRefOid": head, "baseRefOid": "base-sha",
		"headRepository": map[string]any{
			"id": "R_fork", "name": "widget-fork", "nameWithOwner": "contributor/widget-fork",
			"url": "https://github.com/contributor/widget-fork",
		},
		"headRepositoryOwner": map[string]any{"login": "contributor"},
		"additions":           1, "deletions": 0, "changedFiles": 1,
		"author":    map[string]any{"login": "alice"},
		"createdAt": "2026-10-29T00:00:00Z", "updatedAt": "2026-10-29T01:00:00Z",
		"reviews":        map[string]any{"nodes": []any{}},
		"reviewRequests": map[string]any{"totalCount": 0},
		"reviewThreads": map[string]any{
			"totalCount": 0, "nodes": []any{},
			"pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""},
		},
		"commits": map[string]any{"nodes": []any{map[string]any{"commit": map[string]any{
			"statusCheckRollup": map[string]any{"state": "PENDING"},
		}}}},
	}
}

type workflowAttentionMonitorClient struct {
	*MockClient
	admission *RateAdmission
	inner     *workflowAttentionMonitorExecutor
}

func (c *workflowAttentionMonitorClient) ExecuteGraphQL(
	ctx context.Context, query string, variables map[string]any, out any,
) error {
	release, err := c.admission.acquire(ctx, ResourceGraphQL)
	if err != nil {
		return err
	}
	defer release()
	return c.inner.ExecuteGraphQL(ctx, query, variables, out)
}

func newWorkflowAttentionMonitorTest(
	t *testing.T,
) (*Poller, *Service, *Store, *workflowAttentionMonitorExecutor, func() []summaryPublication) {
	t.Helper()
	ctx := context.Background()
	database, _ := prWatchRetirementTaskTemplate.Open(t)
	taskRepo := taskrepo.NewWithInitializedDB(database, database, nil)
	store, err := NewStore(sqlx.NewDb(taskRepo.DB(), "sqlite3"), sqlx.NewDb(taskRepo.DB(), "sqlite3"))
	require.NoError(t, err)

	now := time.Now().UTC()
	require.NoError(t, taskRepo.CreateWorkspace(ctx, &taskmodels.Workspace{
		ID: testWorkspaceID, Name: "Workflow freshness", CreatedAt: now, UpdatedAt: now,
	}))
	seedTestWorkspaceConnection(t, store, testWorkspaceID, "workflow-monitor", 1)
	require.NoError(t, taskRepo.CreateWorkflow(ctx, &taskmodels.Workflow{
		ID: "workflow-freshness", WorkspaceID: testWorkspaceID, Name: "Workflow freshness",
		CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, taskRepo.CreateRepository(ctx, &taskmodels.Repository{
		ID: "repo-1", WorkspaceID: testWorkspaceID, Name: "widget", SourceType: "provider",
		Provider: "github", ProviderOwner: "acme", ProviderName: "widget", CreatedAt: now, UpdatedAt: now,
	}))

	workflowExecutor := newWorkflowTestExecutor()
	for number := 1; number <= workflowAttentionMonitorTaskCount; number++ {
		taskID := workflowAttentionMonitorTaskID(number)
		sessionID := "session-" + taskID
		branch := fmt.Sprintf("feature-%d", number)
		require.NoError(t, taskRepo.CreateTask(ctx, &taskmodels.Task{
			ID: taskID, WorkspaceID: testWorkspaceID, WorkflowID: "workflow-freshness",
			WorkflowStepID: "step-review", Title: taskID, State: v1.TaskStateReview,
			CreatedAt: now, UpdatedAt: now,
		}))
		require.NoError(t, taskRepo.CreateTaskSession(ctx, &taskmodels.TaskSession{
			ID: sessionID, TaskID: taskID, State: taskmodels.TaskSessionStateRunning,
			StartedAt: now, UpdatedAt: now,
		}))
		require.NoError(t, taskRepo.CreateTaskRepository(ctx, &taskmodels.TaskRepository{
			ID: "task-repo-" + taskID, TaskID: taskID, RepositoryID: "repo-1",
			CheckoutBranch: branch, CreatedAt: now, UpdatedAt: now,
		}))
		pr := forkApprovalTestPR(workflowAttentionMonitorPRNumber(number), fmt.Sprintf("head-%d", number))
		pr.branch = branch
		workflowExecutor.setPR(pr)
		require.NoError(t, store.CreatePRWatch(ctx, &PRWatch{
			WorkspaceID: testWorkspaceID, SessionID: sessionID, TaskID: taskID,
			RepositoryID: "repo-1", Owner: "acme", Repo: "widget",
			PRNumber: pr.number, Branch: branch,
		}))
	}

	log, logErr := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, logErr)
	eventBus := bus.NewMemoryEventBus(log)
	executor := &workflowAttentionMonitorExecutor{workflow: workflowExecutor}
	coordinator := NewRateCoordinator(nil, nil)
	_, admission := coordinator.coordinate(defaultGitHubHost, AuthPrincipal{
		Kind: AuthPrincipalHuman, Login: "workflow-monitor",
	}, nil)
	client := &workflowAttentionMonitorClient{
		MockClient: NewMockClient(), admission: admission, inner: executor,
	}
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
			return loadWorkflowAttentionSummaryPRs(ctx, service, taskID)
		},
		Logger:       log,
		RetryBackoff: func(context.Context, int) error { return nil },
	})
	_, err = eventBus.Subscribe(events.GitHubTaskPRUpdated, func(ctx context.Context, event *bus.Event) error {
		return projector.HandleEvent(ctx, event)
	})
	require.NoError(t, err)

	var mu sync.Mutex
	var publications []summaryPublication
	_, err = eventBus.Subscribe(events.TaskStatusSummaryUpdated, func(_ context.Context, event *bus.Event) error {
		payload, ok := event.Data.(statussummary.SummaryUpdated)
		if !ok {
			return nil
		}
		mu.Lock()
		publications = append(publications, summaryPublication{
			TaskID: payload.TaskID, WorkspaceID: payload.WorkspaceID, Summary: payload.Summary,
		})
		mu.Unlock()
		return nil
	})
	require.NoError(t, err)

	poller := NewPoller(service, eventBus, log)
	return poller, service, store, executor, func() []summaryPublication {
		mu.Lock()
		defer mu.Unlock()
		return append([]summaryPublication(nil), publications...)
	}
}

func workflowAttentionMonitorTaskID(number int) string {
	return fmt.Sprintf("workflow-monitor-task-%02d", number)
}

func workflowAttentionMonitorPRNumber(number int) int {
	return 5000 + number
}

func loadWorkflowAttentionSummaryPRs(
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
		input := statussummary.PullRequestInput{
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
		}
		if pr.WorkflowAttention != nil {
			input.WorkflowAttentionState = string(pr.WorkflowAttention.State)
			input.WorkflowAttentionHeadSHA = pr.WorkflowAttention.HeadSHA
			input.WorkflowAttentionStale = pr.WorkflowAttention.Stale
		}
		inputs = append(inputs, input)
	}
	return inputs, nil
}

func summaryApprovalRequired(summary statussummary.TaskStatusSummary) bool {
	return summary.PullRequest != nil && summary.PullRequest.WorkflowApprovalRequired
}

func latestWorkflowAttentionSummaries(publications []summaryPublication) map[string]statussummary.TaskStatusSummary {
	latest := make(map[string]statussummary.TaskStatusSummary, len(publications))
	for _, publication := range publications {
		latest[publication.TaskID] = publication.Summary
	}
	return latest
}

// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.9
// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.1
// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.7
// @covers AC-INTEGRATIONS-GITHUB-PR-POLLING-002.5
// @covers AC-INTEGRATIONS-GITHUB-PR-POLLING-002.6
func TestPRMonitorWorkflowAttention_ApprovalVisibleWithinOneMinuteUnderSharedAdmission(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		poller, _, store, executor, publications := newWorkflowAttentionMonitorTest(t)
		startPRMonitorLoop(t, poller)
		synctest.Wait()

		require.Equal(t, int64(1), executor.statusCalls.Load(),
			"the shared status batch completes once on the first pass")
		initial := latestWorkflowAttentionSummaries(publications())
		require.Len(t, initial, workflowAttentionMonitorTaskCount)
		for taskID, summary := range initial {
			require.False(t, summaryApprovalRequired(summary),
				"%s must not claim approval before workflow evidence completes", taskID)
		}

		time.Sleep(5 * time.Second)
		synctest.Wait()

		final := latestWorkflowAttentionSummaries(publications())
		require.Len(t, final, workflowAttentionMonitorTaskCount)
		for number := 1; number <= workflowAttentionMonitorTaskCount; number++ {
			taskID := workflowAttentionMonitorTaskID(number)
			summary, ok := final[taskID]
			require.True(t, ok, "%s summary missing", taskID)
			require.True(t, summaryApprovalRequired(summary),
				"%s must show approval before the next inventory tick", taskID)
			require.Equal(t, workflowAttentionMonitorPRNumber(number), summary.PullRequest.WorkflowApprovalPRNumber)
		}
		require.Equal(t, int64(1), executor.statusCalls.Load(),
			"workflow resumes must not repeat the completed status phase")
		require.Empty(t, pendingTargetKeys(poller), "completed workflow work releases its pending entry")

		rows, err := store.ListTaskPRsByTaskIDs(context.Background(), []string{workflowAttentionMonitorTaskID(1)})
		require.NoError(t, err)
		require.Len(t, rows[workflowAttentionMonitorTaskID(1)], 1)
		require.Equal(t, WorkflowAttentionApprovalRequired, rows[workflowAttentionMonitorTaskID(1)][0].WorkflowAttention.State)
	})
}

// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.4
// @covers AC-INTEGRATIONS-GITHUB-PR-POLLING-002.7
func TestPRMonitorWorkflowAttention_PrunesObsoleteWorkflowWork(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		poller, _, store, _, publications := newWorkflowAttentionMonitorTest(t)
		startPRMonitorLoop(t, poller)
		synctest.Wait()
		require.NotEmpty(t, pendingTargetKeys(poller), "the first pass must retain deferred workflow work")

		removedTaskID := workflowAttentionMonitorTaskID(1)
		rotatedTaskID := workflowAttentionMonitorTaskID(2)
		watches, err := store.ListActivePRWatches(context.Background())
		require.NoError(t, err)
		for _, watch := range watches {
			if watch.TaskID == removedTaskID {
				require.NoError(t, store.DeletePRWatch(context.Background(), watch.ID))
			}
		}
		rows, err := store.ListTaskPRsByTaskIDs(context.Background(), []string{rotatedTaskID})
		require.NoError(t, err)
		require.Len(t, rows[rotatedTaskID], 1)
		rotated := rows[rotatedTaskID][0]
		rotated.HeadSHA = "rotated-head"
		require.NoError(t, store.UpdateTaskPR(context.Background(), rotated))

		time.Sleep(5 * time.Second)
		synctest.Wait()

		removedRows, err := store.ListTaskPRsByTaskIDs(context.Background(), []string{removedTaskID})
		require.NoError(t, err)
		for _, row := range removedRows[removedTaskID] {
			if row.WorkflowAttention == nil {
				continue
			}
			require.NotEqual(t, WorkflowAttentionApprovalRequired, row.WorkflowAttention.State,
				"a removed watch must not receive a late workflow write")
		}
		rotatedRows, err := store.ListTaskPRsByTaskIDs(context.Background(), []string{rotatedTaskID})
		require.NoError(t, err)
		for _, row := range rotatedRows[rotatedTaskID] {
			if row.WorkflowAttention == nil {
				continue
			}
			require.NotEqual(t, WorkflowAttentionApprovalRequired, row.WorkflowAttention.State,
				"a rotated head must not inherit the previous head's approval")
		}
		for _, publication := range publications() {
			if publication.TaskID != removedTaskID && publication.TaskID != rotatedTaskID {
				continue
			}
			require.False(t, summaryApprovalRequired(publication.Summary),
				"%s must not publish approval for obsolete work", publication.TaskID)
		}

		final := latestWorkflowAttentionSummaries(publications())
		approved := 0
		for taskID, summary := range final {
			if taskID == removedTaskID || taskID == rotatedTaskID {
				continue
			}
			if summaryApprovalRequired(summary) {
				approved++
			}
		}
		require.Equal(t, workflowAttentionMonitorTaskCount-2, approved)
		require.Empty(t, pendingTargetKeys(poller))
	})
}

type workflowDeferringExecutor struct{}

func (workflowDeferringExecutor) ExecuteGraphQL(
	context.Context, string, map[string]any, any,
) error {
	return &AdmissionDeferredError{Delay: time.Second, Reason: "test principal admission"}
}

type workflowFixtureGraphQLClient struct {
	*workflowPhaseCountingClient
	exec GraphQLExecutor
}

func (c *workflowFixtureGraphQLClient) ExecuteGraphQL(
	ctx context.Context, query string, variables map[string]any, out any,
) error {
	return c.exec.ExecuteGraphQL(ctx, query, variables, out)
}

// @covers AC-INTEGRATIONS-GITHUB-PR-POLLING-002.7
// @covers AC-INTEGRATIONS-GITHUB-RATE-002.3
func TestPRMonitorWorkflowAttention_IndependentPrincipalProgresses(t *testing.T) {
	poller, service, mockClient, store := setupPollerTest(t)
	ctx := context.Background()

	connections := map[string]*WorkspaceConnection{
		"workspace-a": {
			WorkspaceID: "workspace-a", Source: ConnectionSourcePAT, GitHubHost: defaultGitHubHost,
			Login: "principal-a", Status: ConnectionStatusActive, CredentialGeneration: 1,
		},
		"workspace-b": {
			WorkspaceID: "workspace-b", Source: ConnectionSourcePAT, GitHubHost: defaultGitHubHost,
			Login: "principal-b", Status: ConnectionStatusActive, CredentialGeneration: 1,
		},
	}
	service.resolver = NewCredentialResolver(testConnectionReader{workspaces: connections}, nil)
	blockedClient := &workflowFixtureGraphQLClient{
		workflowPhaseCountingClient: &workflowPhaseCountingClient{MockClient: mockClient},
		exec:                        workflowDeferringExecutor{},
	}
	executor := newWorkflowTestExecutor()
	pr := forkApprovalTestPR(21, "head-21")
	executor.setPR(pr)
	freeClient := &workflowFixtureGraphQLClient{
		workflowPhaseCountingClient: &workflowPhaseCountingClient{MockClient: mockClient},
		exec:                        executor,
	}
	service.resolver.SetAutomationProvider(workspaceAutomationCredentialProvider{clients: map[string]Client{
		"workspace-a": blockedClient,
		"workspace-b": freeClient,
	}})

	for _, workspaceID := range []string{"workspace-a", "workspace-b"} {
		taskID := "task-" + workspaceID
		seedTask(t, store, taskID, false)
		row := &TaskPR{
			WorkspaceID: workspaceID, TaskID: taskID, RepositoryID: "repo-1",
			Owner: "acme", Repo: "widget", PRNumber: 21,
			PRURL: "https://github.com/acme/widget/pull/21", HeadBranch: pr.branch, BaseBranch: "main",
			State: "open", HeadSHA: "head-21", CreatedAt: time.Now().UTC(),
		}
		require.NoError(t, store.CreateTaskPR(ctx, row))
		require.NoError(t, store.CreatePRWatch(ctx, &PRWatch{
			WorkspaceID: workspaceID, SessionID: "session-" + workspaceID, TaskID: taskID,
			RepositoryID: "repo-1", Owner: "acme", Repo: "widget", PRNumber: 21, Branch: pr.branch,
		}))
	}
	watches, err := store.ListActivePRWatches(ctx)
	require.NoError(t, err)
	byWorkspace := map[string][]*PRWatch{}
	for _, watch := range watches {
		byWorkspace[watch.WorkspaceID] = append(byWorkspace[watch.WorkspaceID], watch)
	}

	outcomes := poller.runWorkflowPhaseWakes(ctx, byWorkspace)
	require.Len(t, outcomes, 2)
	for _, outcome := range outcomes {
		if outcome.workspaceID == "workspace-a" {
			require.Equal(t, prWatchBatchDeferred, outcome.state)
			require.NotNil(t, outcome.deferred)
			require.NotEmpty(t, outcome.workflowUnfinished)
			continue
		}
		require.Equal(t, prWatchBatchCompleted, outcome.state)
		require.Empty(t, outcome.workflowUnfinished)
	}

	freeRows, err := store.ListTaskPRsByTaskIDs(ctx, []string{"task-workspace-b"})
	require.NoError(t, err)
	require.NotNil(t, freeRows["task-workspace-b"][0].WorkflowAttention)
	require.Equal(t, WorkflowAttentionApprovalRequired, freeRows["task-workspace-b"][0].WorkflowAttention.State,
		"a blocked principal must not delay another principal's workflow phase")
	blockedRows, err := store.ListTaskPRsByTaskIDs(ctx, []string{"task-workspace-a"})
	require.NoError(t, err)
	if blockedRows["task-workspace-a"][0].WorkflowAttention != nil {
		require.NotEqual(t, WorkflowAttentionApprovalRequired, blockedRows["task-workspace-a"][0].WorkflowAttention.State)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-PR-POLLING-002.7
func TestPRMonitorWorkflowAttention_CredentialRotationDropsPending(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		poller, _, store, _, publications := newWorkflowAttentionMonitorTest(t)
		startPRMonitorLoop(t, poller)
		synctest.Wait()
		require.NotEmpty(t, pendingTargetKeys(poller))

		seedTestWorkspaceConnection(t, store, testWorkspaceID, "rotated-principal", 2)
		time.Sleep(2 * time.Second)
		synctest.Wait()

		require.Empty(t, pendingTargetKeys(poller), "a rotated credential drops the old principal's boundary")
		for _, publication := range publications() {
			require.False(t, summaryApprovalRequired(publication.Summary),
				"%s must not publish approval read under the previous principal", publication.TaskID)
		}
	})
}

// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.9
func TestWorkflowPhase_CompletedStatusRefreshesWhileSiblingDefers(t *testing.T) {
	_, service, mockClient, store := setupPollerTest(t)
	workflowExec := newWorkflowTestExecutor()
	pr := forkApprovalTestPR(5001, "head-1")
	pr.branch = "feature-1"
	workflowExec.setPR(pr)
	executor := &workflowAttentionMonitorExecutor{workflow: workflowExec, branchDeferrals: 1}
	client := &workflowFixtureGraphQLClient{
		workflowPhaseCountingClient: &workflowPhaseCountingClient{MockClient: mockClient},
		exec:                        executor,
	}
	service.client = client
	configureTestWorkspaceAuth(t, service, client, testWorkspaceID)

	seedTask(t, store, "task-completed-status", false)
	seedTask(t, store, "task-deferred-search", false)
	numbered := &PRWatch{
		WorkspaceID: testWorkspaceID, SessionID: "session-completed-status",
		TaskID: "task-completed-status", RepositoryID: "repo-1",
		Owner: "acme", Repo: "widget", PRNumber: 5001, Branch: "feature-1",
	}
	searching := &PRWatch{
		WorkspaceID: testWorkspaceID, SessionID: "session-deferred-search",
		TaskID: "task-deferred-search", RepositoryID: "repo-1",
		Owner: "acme", Repo: "widget", Branch: "feature-search",
	}
	require.NoError(t, store.CreatePRWatch(context.Background(), numbered))
	require.NoError(t, store.CreatePRWatch(context.Background(), searching))

	outcome := service.syncWorkspaceWatchesBatchedForPoller(
		context.Background(), testWorkspaceID, []*PRWatch{numbered, searching},
	)
	require.NotNil(t, outcome.Deferred, "the searching status phase still defers after the numbered phase applies")

	rows, err := store.ListTaskPRsByTask(context.Background(), "task-completed-status")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].WorkflowAttention)
	require.Equal(t, WorkflowAttentionApprovalRequired, rows[0].WorkflowAttention.State,
		"a completed known-PR status must refresh even when an unrelated branch phase defers")
	step1, _ := workflowExec.counts()
	require.Equal(t, 1, step1)
}

// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.9
func TestPRMonitorWorkflowAttention_KeepsWorkflowBoundarySeparate(t *testing.T) {
	now := time.Now().UTC()
	statusWatch := &PRWatch{
		WorkspaceID: "workspace", SessionID: "session-status", TaskID: "task-status",
		Owner: "acme", Repo: "widget", PRNumber: 1, Branch: "feature-1",
	}
	workflowWatch := &PRWatch{
		WorkspaceID: "workspace", SessionID: "session-workflow", TaskID: "task-workflow",
		Owner: "acme", Repo: "widget", PRNumber: 2, Branch: "feature-2",
	}
	work := prMonitorPendingFromOutcome(prWatchWorkspaceBatchOutcome{
		workspaceID:        "workspace",
		watches:            []*PRWatch{statusWatch, workflowWatch},
		unfinished:         []*PRWatch{statusWatch},
		workflowUnfinished: []*PRWatch{workflowWatch},
		deferred:           &AdmissionDeferredError{Delay: 10 * time.Minute},
		workflowDeferred:   &AdmissionDeferredError{Delay: time.Second},
		state:              prWatchBatchDeferred,
	}, now)
	require.NotNil(t, work)
	statusTarget := work.targets[prWatchDiscoveryTargetKey(statusWatch)]
	workflowTarget := work.targets[prWatchDiscoveryTargetKey(workflowWatch)]
	require.NotNil(t, statusTarget)
	require.NotNil(t, workflowTarget)
	require.True(t, statusTarget.retryAt.Equal(now.Add(10*time.Minute)))
	require.True(t, workflowTarget.retryAt.Equal(now.Add(time.Second)),
		"workflow work must not inherit the searching phase's long boundary")
}
