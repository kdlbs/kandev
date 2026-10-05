package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/httpmw"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/task/statussummary"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type sidebarCancellationRepo struct {
	httpTaskRepo
	page                 *models.SidebarTaskPageResult
	queryCalls           int
	batchCalls           int
	primaryInfoCalls     int
	pendingActionCalls   int
	repoLinksCalls       int
	sessionCountCalls    int
	folderCalls          int
	taskEnvironmentCalls int
	gitSnapshotCalls     int
	onQuery              func(context.Context) (*models.SidebarTaskPageResult, error)
	onBatchSessions      func(context.Context) (map[string][]*models.TaskSession, error)
	onPendingActions     func(context.Context) (map[string]models.TaskPendingAction, error)
	onRepoLinks          func(context.Context) (map[string][]*models.TaskRepository, error)
}

func (r *sidebarCancellationRepo) QuerySidebarTaskPage(
	ctx context.Context,
	_ string,
	_ models.SidebarTaskViewQuery,
	_ models.SidebarTaskViewPreferences,
) (*models.SidebarTaskPageResult, error) {
	r.queryCalls++
	if r.onQuery != nil {
		return r.onQuery(ctx)
	}
	return r.page, nil
}

func (r *sidebarCancellationRepo) BatchGetSessionsByTaskIDs(
	ctx context.Context,
	taskIDs []string,
) (map[string][]*models.TaskSession, error) {
	r.batchCalls++
	if r.onBatchSessions != nil {
		return r.onBatchSessions(ctx)
	}
	return r.httpTaskRepo.BatchGetSessionsByTaskIDs(ctx, taskIDs)
}

func (r *sidebarCancellationRepo) GetPrimarySessionInfoByTaskIDs(
	ctx context.Context,
	taskIDs []string,
) (map[string]*models.TaskSession, error) {
	r.primaryInfoCalls++
	return r.httpTaskRepo.GetPrimarySessionInfoByTaskIDs(ctx, taskIDs)
}

func (r *sidebarCancellationRepo) GetPendingActionsBySessionIDs(
	ctx context.Context,
	sessionIDs []string,
) (map[string]models.TaskPendingAction, error) {
	r.pendingActionCalls++
	if r.onPendingActions != nil {
		return r.onPendingActions(ctx)
	}
	return r.httpTaskRepo.GetPendingActionsBySessionIDs(ctx, sessionIDs)
}

func (r *sidebarCancellationRepo) ListTaskRepositoriesByTaskIDs(
	ctx context.Context,
	taskIDs []string,
) (map[string][]*models.TaskRepository, error) {
	r.repoLinksCalls++
	if r.onRepoLinks != nil {
		return r.onRepoLinks(ctx)
	}
	return r.httpTaskRepo.ListTaskRepositoriesByTaskIDs(ctx, taskIDs)
}

func (r *sidebarCancellationRepo) GetSessionCountsByTaskIDs(
	ctx context.Context,
	taskIDs []string,
) (map[string]int, error) {
	r.sessionCountCalls++
	return r.httpTaskRepo.GetSessionCountsByTaskIDs(ctx, taskIDs)
}

func (r *sidebarCancellationRepo) GetTaskEnvironmentByTaskID(
	ctx context.Context,
	taskID string,
) (*models.TaskEnvironment, error) {
	r.taskEnvironmentCalls++
	return r.httpTaskRepo.GetTaskEnvironmentByTaskID(ctx, taskID)
}

func (r *sidebarCancellationRepo) GetLatestGitStatusSnapshotsByTaskEnvironmentIDs(
	ctx context.Context,
	environmentIDs []string,
) ([]*models.GitSnapshot, error) {
	r.gitSnapshotCalls++
	return r.httpTaskRepo.GetLatestGitStatusSnapshotsByTaskEnvironmentIDs(ctx, environmentIDs)
}

func (r *sidebarCancellationRepo) ListTaskWorkspaceFoldersByTaskIDs(
	context.Context,
	[]string,
) (map[string][]*models.TaskWorkspaceFolder, error) {
	r.folderCalls++
	return map[string][]*models.TaskWorkspaceFolder{}, nil
}

func (*sidebarCancellationRepo) ListTaskWorkspaceFolders(context.Context, string) ([]*models.TaskWorkspaceFolder, error) {
	return nil, nil
}

func (*sidebarCancellationRepo) CreateWorkspaceSourceBatch(context.Context, *models.WorkspaceSourceBatch) error {
	return nil
}

func (*sidebarCancellationRepo) CompensateWorkspaceSourceBatch(context.Context, *models.WorkspaceSourceBatch) error {
	return nil
}

type sidebarCancellationBlockers struct {
	service.BlockerRepository
	calls  int
	cancel context.CancelFunc
}

func (r *sidebarCancellationBlockers) ListBlockersForTasks(context.Context, []string) (map[string][]string, error) {
	r.calls++
	r.cancel()
	return nil, fmt.Errorf("list blockers: %w", context.Canceled)
}

type sidebarStatusSummaryReader struct {
	repository.TaskStatusSummaryRepository
	loadCalls  int
	writeCalls int
	rows       map[string]*statussummary.TaskStatusSummary
}

func (r *sidebarStatusSummaryReader) LoadTaskStatusSummaries(
	_ context.Context,
	taskIDs []string,
) (map[string]*statussummary.TaskStatusSummary, error) {
	r.loadCalls++
	rows := make(map[string]*statussummary.TaskStatusSummary, len(taskIDs))
	for _, taskID := range taskIDs {
		if summary := r.rows[taskID]; summary != nil {
			copy := *summary
			rows[taskID] = &copy
		}
	}
	return rows, nil
}

func (r *sidebarStatusSummaryReader) CompareAndUpdateTaskStatusSummary(
	_ context.Context,
	stored *statussummary.StoredTaskStatusSummary,
) (bool, error) {
	r.writeCalls++
	if r.rows == nil {
		r.rows = make(map[string]*statussummary.TaskStatusSummary)
	}
	copy := stored.Summary
	r.rows[stored.TaskID] = &copy
	return true, nil
}

func (r *sidebarStatusSummaryReader) DeleteTaskStatusSummary(_ context.Context, taskID string) error {
	delete(r.rows, taskID)
	return nil
}

type sidebarCancellationActivityReader struct {
	calls  int
	cancel context.CancelFunc
}

type cancelDuringSidebarBodyRead struct {
	cancel context.CancelFunc
}

func (r cancelDuringSidebarBodyRead) Read([]byte) (int, error) {
	r.cancel()
	return 0, errors.New("request body read interrupted")
}

func (r *sidebarCancellationActivityReader) LoadTaskLastActivity(
	_ context.Context,
	taskIDs []string,
) (map[string]time.Time, error) {
	r.calls++
	if r.calls == 1 {
		r.cancel()
		return nil, fmt.Errorf("load task activity: %w", context.Canceled)
	}
	return map[string]time.Time{}, nil
}

type sidebarQueuedPromptCounter struct{ calls int }

func (c *sidebarQueuedPromptCounter) CountPendingByTaskIDs(context.Context, []string) (map[string]int, error) {
	c.calls++
	return map[string]int{}, nil
}

func newSidebarCancellationHandlers(
	t *testing.T,
	repo *sidebarCancellationRepo,
	statusSummaries repository.TaskStatusSummaryRepository,
	log *logger.Logger,
	activity ...repository.TaskActivityRepository,
) *TaskHandlers {
	t.Helper()
	var taskActivity repository.TaskActivityRepository
	if len(activity) > 0 {
		taskActivity = activity[0]
	}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo, WorkspaceFolders: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo, ResourceCleanups: repo, StatusSummaries: statusSummaries, TaskActivity: taskActivity,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	svc.SetWorktreeCleanup(authzDeleteCleanup{})
	return &TaskHandlers{service: svc, logger: log}
}

func TestTaskListActivityCancellationStopsSummaryReconciliationAndAllowsSuccessor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, observed := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	repo := &sidebarCancellationRepo{httpTaskRepo: httpTaskRepo{listTotal: 1}}
	statusSummaries := &sidebarStatusSummaryReader{}
	queuedCounter := &sidebarQueuedPromptCounter{}
	ctx, cancel := context.WithCancel(context.Background())
	activity := &sidebarCancellationActivityReader{cancel: cancel}
	h := newSidebarCancellationHandlers(t, repo, statusSummaries, log, activity)
	h.service.SetQueuedPromptCounter(queuedCounter)
	launchQueueCalls := 0
	h.service.SetTaskStatusSummaryLaunchQueueReader(func(context.Context, []*models.Task) map[string]*statussummary.LaunchQueueSummary {
		launchQueueCalls++
		return map[string]*statussummary.LaunchQueueSummary{}
	})

	first, firstResponse := taskRequestAs(t, "", http.MethodGet, "/api/v1/workspaces/ws-b/tasks", "ws-b")
	first.Request = first.Request.WithContext(ctx)
	h.httpListTasksByWorkspace(first)

	require.Equal(t, 499, firstResponse.Code)
	require.Empty(t, firstResponse.Body.String())
	require.Equal(t, 1, activity.calls)
	require.Equal(t, 1, statusSummaries.loadCalls, "the persisted summary read is wired and precedes activity reconciliation")
	require.Zero(t, statusSummaries.writeCalls, "canceled summary repair must not write")
	require.Zero(t, launchQueueCalls, "reconciliation must stop immediately after canceled activity read")
	require.Zero(t, repo.taskEnvironmentCalls)
	require.Zero(t, repo.gitSnapshotCalls)
	require.Zero(t, queuedCounter.calls)
	require.Zero(t, observed.FilterLevelExact(zapcore.WarnLevel).Len())
	require.Zero(t, observed.FilterLevelExact(zapcore.ErrorLevel).Len())

	second, secondResponse := taskRequestAs(t, "", http.MethodGet, "/api/v1/workspaces/ws-b/tasks", "ws-b")
	h.httpListTasksByWorkspace(second)
	require.Equal(t, http.StatusOK, secondResponse.Code)
	require.Equal(t, 2, activity.calls)
	require.Equal(t, 2, statusSummaries.loadCalls)
	require.Equal(t, 1, statusSummaries.writeCalls, "an active successor must rebuild its missing summary")
}

func TestSidebarQueryCancellationReturns499AndAllowsSuccessfulSuccessor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, observed := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	repo := &sidebarCancellationRepo{
		page: &models.SidebarTaskPageResult{
			QueryKey: "sidebar-key", Page: 1, PageSize: 20, TotalTasks: 1,
			TotalVisibleTasks: 1,
			Entries:           []models.SidebarTaskPageEntry{{Kind: "task", TaskID: "task-b"}},
			Tasks:             []*models.Task{{ID: "task-b", WorkspaceID: "ws-b", Title: "Mine", State: v1.TaskStateTODO}},
		},
	}
	h := newSidebarCancellationHandlers(t, repo, nil, log)
	router := gin.New()
	router.Use(httpmw.RequestLogger(log, "task"))
	router.POST("/api/v1/workspaces/:id/sidebar/query", h.httpQuerySidebarTasks)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/ws-b/sidebar/query", strings.NewReader(`{"locale":"en"}`)).WithContext(ctx)
	first := httptest.NewRecorder()
	firstQuery := true
	repo.onQuery = func(context.Context) (*models.SidebarTaskPageResult, error) {
		if firstQuery {
			firstQuery = false
			cancel()
			return nil, fmt.Errorf("sidebar query: %w", context.Canceled)
		}
		return repo.page, nil
	}
	router.ServeHTTP(first, req)

	require.Equal(t, 499, first.Code)
	require.Empty(t, first.Body.String(), "an abandoned request has no JSON failure body")
	require.Equal(t, 1, repo.queryCalls)
	require.Zero(t, repo.batchCalls, "a canceled query must not start enrichment")
	require.Empty(t, repo.updated, "a canceled query must not mutate task state")
	require.Zero(t, observed.FilterLevelExact(zapcore.WarnLevel).Len())
	require.Zero(t, observed.FilterLevelExact(zapcore.ErrorLevel).Len())

	second := httptest.NewRecorder()
	secondReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/ws-b/sidebar/query", strings.NewReader(`{"locale":"en"}`))
	router.ServeHTTP(second, secondReq)
	require.Equal(t, http.StatusOK, second.Code)
	require.Contains(t, second.Body.String(), `"query_key":"sidebar-key"`)
	require.Contains(t, second.Body.String(), `"title":"Mine"`)
	require.Equal(t, v1.TaskStateTODO, repo.page.Tasks[0].State)
}

func TestSidebarQueryBodyCancellationWinsOverMalformedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, observed := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	repo := &sidebarCancellationRepo{}
	h := newSidebarCancellationHandlers(t, repo, nil, log)
	router := gin.New()
	router.POST("/api/v1/workspaces/:id/sidebar/query", h.httpQuerySidebarTasks)
	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/ws-b/sidebar/query", nil).WithContext(ctx)
	request.Body = io.NopCloser(cancelDuringSidebarBodyRead{cancel: cancel})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, 499, response.Code)
	require.Empty(t, response.Body.String())
	require.Zero(t, repo.queryCalls)
	require.Zero(t, observed.FilterLevelExact(zapcore.WarnLevel).Len())
	require.Zero(t, observed.FilterLevelExact(zapcore.ErrorLevel).Len())
}

func TestSidebarQueryCancellationStillLogsConcurrentRepositoryFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, observed := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	repo := &sidebarCancellationRepo{}
	h := newSidebarCancellationHandlers(t, repo, nil, log)
	router := gin.New()
	router.POST("/api/v1/workspaces/:id/sidebar/query", h.httpQuerySidebarTasks)
	ctx, cancel := context.WithCancel(context.Background())
	repo.onQuery = func(context.Context) (*models.SidebarTaskPageResult, error) {
		cancel()
		return nil, errors.New("database busy")
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/ws-b/sidebar/query", strings.NewReader(`{"locale":"en"}`)).WithContext(ctx)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, 499, response.Code)
	require.Empty(t, response.Body.String())
	require.Equal(t, 1, observed.FilterLevelExact(zapcore.ErrorLevel).Len())
}

func TestSidebarEnrichmentCancellationStopsOptionalReads(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, observed := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	repo := &sidebarCancellationRepo{
		page: &models.SidebarTaskPageResult{
			QueryKey: "sidebar-key", Page: 1, PageSize: 20,
			Entries: []models.SidebarTaskPageEntry{{Kind: "task", TaskID: "task-b"}},
			Tasks:   []*models.Task{{ID: "task-b", WorkspaceID: "ws-b", Title: "Mine", State: v1.TaskStateTODO}},
		},
	}
	repo.onBatchSessions = func(context.Context) (map[string][]*models.TaskSession, error) {
		return map[string][]*models.TaskSession{"task-b": {{
			ID: "session-b", TaskID: "task-b", State: models.TaskSessionStateRunning,
		}}}, nil
	}
	statusSummaries := &sidebarStatusSummaryReader{}
	queuedCounter := &sidebarQueuedPromptCounter{}
	h := newSidebarCancellationHandlers(t, repo, statusSummaries, log)
	h.service.SetQueuedPromptCounter(queuedCounter)
	router := gin.New()
	router.Use(httpmw.RequestLogger(log, "task"))
	router.POST("/api/v1/workspaces/:id/sidebar/query", h.httpQuerySidebarTasks)
	ctx, cancel := context.WithCancel(context.Background())
	repo.onPendingActions = func(context.Context) (map[string]models.TaskPendingAction, error) {
		cancel()
		return nil, fmt.Errorf("pending actions: %w", context.Canceled)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/ws-b/sidebar/query", strings.NewReader(`{"locale":"en"}`)).WithContext(ctx)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)

	require.Equal(t, 499, response.Code)
	require.Empty(t, response.Body.String())
	require.Equal(t, 1, repo.pendingActionCalls)
	require.Zero(t, statusSummaries.loadCalls, "status summary read must not run after cancellation")
	require.Zero(t, queuedCounter.calls, "queued-prompt count must not run after cancellation")
	require.Empty(t, repo.updated, "read cancellation must not mutate task state")
	require.Zero(t, observed.FilterLevelExact(zapcore.WarnLevel).Len())
	require.Zero(t, observed.FilterLevelExact(zapcore.ErrorLevel).Len())
}

func TestSidebarDependencyCancellationStopsLaterProjectionReads(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, observed := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	repo := &sidebarCancellationRepo{
		page: &models.SidebarTaskPageResult{
			QueryKey: "sidebar-key", Page: 1, PageSize: 20,
			Entries: []models.SidebarTaskPageEntry{{Kind: "task", TaskID: "task-b"}},
			Tasks:   []*models.Task{{ID: "task-b", WorkspaceID: "ws-b", Title: "Mine", State: v1.TaskStateTODO}},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	blockers := &sidebarCancellationBlockers{cancel: cancel}
	h := newSidebarCancellationHandlers(t, repo, nil, log)
	h.service.SetBlockerRepository(blockers)
	router := gin.New()
	router.Use(httpmw.RequestLogger(log, "task"))
	router.POST("/api/v1/workspaces/:id/sidebar/query", h.httpQuerySidebarTasks)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/ws-b/sidebar/query", strings.NewReader(`{"locale":"en"}`)).WithContext(ctx)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, 499, response.Code)
	require.Equal(t, 1, blockers.calls)
	require.Zero(t, repo.repoLinksCalls, "runner projection must not start after dependency cancellation")
	require.Zero(t, observed.FilterLevelExact(zapcore.WarnLevel).Len())
	require.Zero(t, observed.FilterLevelExact(zapcore.ErrorLevel).Len())
}

func TestSidebarRunnerCancellationStopsRemainingSignalReads(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, observed := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	repo := &sidebarCancellationRepo{
		page: &models.SidebarTaskPageResult{
			QueryKey: "sidebar-key", Page: 1, PageSize: 20,
			Entries: []models.SidebarTaskPageEntry{{Kind: "task", TaskID: "task-b"}},
			Tasks:   []*models.Task{{ID: "task-b", WorkspaceID: "ws-b", Title: "Mine", State: v1.TaskStateTODO}},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	repo.onRepoLinks = func(context.Context) (map[string][]*models.TaskRepository, error) {
		cancel()
		return map[string][]*models.TaskRepository{}, nil
	}
	h := newSidebarCancellationHandlers(t, repo, nil, log)
	router := gin.New()
	router.Use(httpmw.RequestLogger(log, "task"))
	router.POST("/api/v1/workspaces/:id/sidebar/query", h.httpQuerySidebarTasks)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/ws-b/sidebar/query", strings.NewReader(`{"locale":"en"}`)).WithContext(ctx)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, 499, response.Code)
	require.Equal(t, 1, repo.repoLinksCalls)
	require.Zero(t, repo.sessionCountCalls, "runner batch must stop after the canceled signal read")
	require.Zero(t, repo.folderCalls, "later runner signals must not start after cancellation")
	require.Zero(t, observed.FilterLevelExact(zapcore.WarnLevel).Len())
	require.Zero(t, observed.FilterLevelExact(zapcore.ErrorLevel).Len())
}

func TestSidebarNestedCancellationWithActiveRequestRemainsServerFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, observed := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	repo := &sidebarCancellationRepo{}
	repo.onQuery = func(context.Context) (*models.SidebarTaskPageResult, error) {
		return nil, fmt.Errorf("database adapter: %w", context.Canceled)
	}
	h := newSidebarCancellationHandlers(t, repo, nil, log)
	router := gin.New()
	router.Use(httpmw.RequestLogger(log, "task"))
	router.POST("/api/v1/workspaces/:id/sidebar/query", h.httpQuerySidebarTasks)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/ws-b/sidebar/query", strings.NewReader(`{"locale":"en"}`))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusInternalServerError, response.Code)
	require.Equal(t, 1, observed.FilterLevelExact(zapcore.ErrorLevel).Len())
}

func TestSidebarDeadlineRemainsServerFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := newTestLogger(t)
	repo := &sidebarCancellationRepo{}
	repo.onQuery = func(ctx context.Context) (*models.SidebarTaskPageResult, error) {
		return nil, ctx.Err()
	}
	h := newSidebarCancellationHandlers(t, repo, nil, log)
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/ws-b/sidebar/query", strings.NewReader(`{"locale":"en"}`)).WithContext(ctx)
	response := httptest.NewRecorder()
	router := gin.New()
	router.POST("/api/v1/workspaces/:id/sidebar/query", h.httpQuerySidebarTasks)
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusInternalServerError, response.Code)
}
