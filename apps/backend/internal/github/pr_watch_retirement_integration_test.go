package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

var prWatchRetirementTaskTemplate = testutil.NewSQLiteTemplate(func(database *sqlx.DB) error {
	_, err := taskrepo.NewWithDB(database, database, nil)
	return err
})

type persistedTaskBranchProvider struct {
	repo       *taskrepo.Repository
	watchStore interface {
		ListActivePRWatches(context.Context) ([]*PRWatch, error)
	}
}

func (p persistedTaskBranchProvider) ListTasksNeedingPRWatch(ctx context.Context) ([]TaskBranchInfo, error) {
	sessions, err := p.repo.ListSessionsWithBranches(ctx)
	if err != nil {
		return nil, err
	}
	active, err := p.watchStore.ListActivePRWatches(ctx)
	if err != nil {
		return nil, err
	}
	watched := make(map[string]bool, len(active))
	for _, watch := range active {
		watched[retirementIntegrationTargetKey(watch.TaskID, watch.RepositoryID, watch.Branch)] = true
	}
	var targets []TaskBranchInfo
	for _, session := range sessions {
		repositories, err := p.repo.ListTaskRepositories(ctx, session.TaskID)
		if err != nil {
			return nil, err
		}
		for _, taskRepository := range repositories {
			repository, err := p.repo.GetRepository(ctx, taskRepository.RepositoryID)
			if err != nil {
				return nil, err
			}
			key := retirementIntegrationTargetKey(session.TaskID, repository.ID, session.Branch)
			if watched[key] {
				continue
			}
			targets = append(targets, TaskBranchInfo{
				WorkspaceID:  session.WorkspaceID,
				TaskID:       session.TaskID,
				SessionID:    session.SessionID,
				RepositoryID: repository.ID,
				Owner:        repository.ProviderOwner,
				Repo:         repository.ProviderName,
				Branch:       session.Branch,
			})
		}
	}
	return targets, nil
}

func (p persistedTaskBranchProvider) ResolveBranchForWatch(ctx context.Context, watch *PRWatch) string {
	sessions, err := p.repo.ListSessionsWithBranches(ctx)
	if err != nil {
		return ""
	}
	for _, session := range sessions {
		if session.TaskID == watch.TaskID && session.SessionID == watch.SessionID {
			return session.Branch
		}
	}
	return ""
}

func (p persistedTaskBranchProvider) ResolveBranchesForRepository(ctx context.Context, taskID, repositoryID string) []string {
	repositories, err := p.repo.ListTaskRepositories(ctx, taskID)
	if err != nil {
		return nil
	}
	for _, repository := range repositories {
		if repository.RepositoryID != repositoryID {
			continue
		}
		sessions, err := p.repo.ListSessionsWithBranches(ctx)
		if err != nil {
			return nil
		}
		var branches []string
		for _, session := range sessions {
			if session.TaskID == taskID {
				branches = append(branches, session.Branch)
			}
		}
		return branches
	}
	return nil
}

func retirementIntegrationTargetKey(taskID, repositoryID, branch string) string {
	return taskID + "|" + repositoryID + "|" + branch
}

// @covers AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.1
// @covers AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.2
// @covers AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.5
func TestPRWatchRetirement_TaskLifecycleIntegration(t *testing.T) {
	ctx := context.Background()
	database, _ := prWatchRetirementTaskTemplate.Open(t)
	taskRepo := taskrepo.NewWithInitializedDB(database, database, nil)
	store, err := NewStore(sqlx.NewDb(taskRepo.DB(), "sqlite3"), sqlx.NewDb(taskRepo.DB(), "sqlite3"))
	require.NoError(t, err)

	seedRetirementTaskWithBranch(t, taskRepo, "task-done", "session-done", "finished", v1.TaskStateCompleted)
	seedRetirementTaskWithBranch(t, taskRepo, "task-review", "session-review", "review", v1.TaskStateReview)

	firstClient := NewMockClient()
	firstService := newWorkspaceAuthenticatedTestService(t, firstClient, store, testWorkspaceID)
	firstService.SetTaskIssueStore(taskRepo)
	t.Cleanup(firstService.Stop)
	provider := persistedTaskBranchProvider{repo: taskRepo, watchStore: store}

	doneWatch, err := firstService.EnsurePRWatchForWorkspace(
		ctx, testWorkspaceID, "session-done", "task-done", "repo-1", "acme", "widget", "finished",
	)
	require.NoError(t, err)
	router := retirementIntegrationRouter(t, firstService)
	response := retirementIntegrationRequest(router, http.MethodDelete,
		"/api/v1/github/watches/pr/"+doneWatch.ID+"?workspace_id="+testWorkspaceID)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	firstPoller := NewPoller(firstService, nil, testLogger(t))
	firstPoller.SetTaskBranchProvider(provider)
	for range 3 {
		firstPoller.reconcileWatches(ctx)
	}
	active, err := store.ListActivePRWatches(ctx)
	require.NoError(t, err)
	require.Len(t, active, 1)
	require.Equal(t, "task-review", active[0].TaskID, "the Review sibling remains eligible")
	require.ErrorIs(t, func() error {
		_, err := firstService.EnsurePRWatchForWorkspace(
			ctx, testWorkspaceID, "session-done", "task-done", "repo-1", "acme", "widget", "finished",
		)
		return err
	}(), ErrPRWatchRetired)

	firstCalls := make(chan string, 8)
	firstClient.GateFindPRByBranch(firstCalls, nil)
	firstPoller.SetClock(func() time.Time { return time.Now().Add(2 * time.Minute) })
	firstPoller.checkPRWatches(ctx)
	require.Equal(t, []string{"review"}, drainRetirementIntegrationCalls(firstCalls),
		"only the active Review sibling reaches branch discovery")

	secondStore, err := NewStore(sqlx.NewDb(taskRepo.DB(), "sqlite3"), sqlx.NewDb(taskRepo.DB(), "sqlite3"))
	require.NoError(t, err, "reconstruct the GitHub store from persisted task database state")
	secondClient := NewMockClient()
	secondService := newWorkspaceAuthenticatedTestService(t, secondClient, secondStore, testWorkspaceID)
	secondService.SetTaskIssueStore(taskRepo)
	t.Cleanup(secondService.Stop)
	secondPoller := NewPoller(secondService, nil, testLogger(t))
	secondPoller.SetTaskBranchProvider(persistedTaskBranchProvider{repo: taskRepo, watchStore: secondStore})
	secondCalls := make(chan string, 8)
	secondClient.GateFindPRByBranch(secondCalls, nil)
	secondPoller.SetClock(func() time.Time { return time.Now().Add(2 * time.Minute) })
	secondPoller.checkPRWatches(ctx)
	require.Equal(t, []string{"review"}, drainRetirementIntegrationCalls(secondCalls),
		"service reconstruction preserves retirement while polling the active sibling")

	router = retirementIntegrationRouter(t, secondService)
	ordinary := retirementIntegrationRequest(router, http.MethodGet,
		"/api/v1/github/watches/pr?workspace_id="+testWorkspaceID)
	require.Equal(t, http.StatusOK, ordinary.Code, ordinary.Body.String())
	var activeInventory struct {
		Watches []*PRWatch `json:"watches"`
	}
	require.NoError(t, json.Unmarshal(ordinary.Body.Bytes(), &activeInventory))
	require.Len(t, activeInventory.Watches, 1)
	require.Equal(t, "task-review", activeInventory.Watches[0].TaskID)

	inclusive := retirementIntegrationRequest(router, http.MethodGet,
		"/api/v1/github/watches/pr?workspace_id="+testWorkspaceID+"&include_retired=true")
	require.Equal(t, http.StatusOK, inclusive.Code, inclusive.Body.String())
	var allInventory struct {
		Watches []*PRWatch `json:"watches"`
	}
	require.NoError(t, json.Unmarshal(inclusive.Body.Bytes(), &allInventory))
	require.Len(t, allInventory.Watches, 2)

	resume := retirementIntegrationRequest(router, http.MethodPost,
		"/api/v1/github/watches/pr/"+doneWatch.ID+"/resume?workspace_id="+testWorkspaceID)
	require.Equal(t, http.StatusOK, resume.Code, resume.Body.String())
	var resumedPayload struct {
		Watch PRWatch `json:"watch"`
	}
	require.NoError(t, json.Unmarshal(resume.Body.Bytes(), &resumedPayload))
	require.NotEmpty(t, resumedPayload.Watch.ID)
	require.NotEqual(t, doneWatch.ID, resumedPayload.Watch.ID, "resumption starts a new watch lifetime")
	require.Nil(t, resumedPayload.Watch.RetiredAt)

	secondPoller.checkPRWatches(ctx)
	postResumeCalls := drainRetirementIntegrationCalls(secondCalls)
	require.Contains(t, postResumeCalls, "finished", "the resumed Done target is admitted explicitly")
	retired, err := store.GetPRWatch(ctx, doneWatch.ID)
	require.NoError(t, err)
	require.Nil(t, retired, "resumption replaces the retired row with a new lifetime ID")
}

func seedRetirementTaskWithBranch(
	t *testing.T,
	repo *taskrepo.Repository,
	taskID, sessionID, branch string,
	state v1.TaskState,
) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	if taskID == "task-done" {
		require.NoError(t, repo.CreateWorkspace(ctx, &taskmodels.Workspace{
			ID: testWorkspaceID, Name: "Retirement", CreatedAt: now, UpdatedAt: now,
		}))
		require.NoError(t, repo.CreateWorkflow(ctx, &taskmodels.Workflow{
			ID: "workflow-retirement", WorkspaceID: testWorkspaceID, Name: "Retirement",
			CreatedAt: now, UpdatedAt: now,
		}))
		require.NoError(t, repo.CreateRepository(ctx, &taskmodels.Repository{
			ID: "repo-1", WorkspaceID: testWorkspaceID, Name: "widget", SourceType: "provider",
			Provider: "github", ProviderOwner: "acme", ProviderName: "widget", CreatedAt: now, UpdatedAt: now,
		}))
	}
	require.NoError(t, repo.CreateTask(ctx, &taskmodels.Task{
		ID: taskID, WorkspaceID: testWorkspaceID, WorkflowID: "workflow-retirement",
		WorkflowStepID: "step-review", Title: taskID, State: state, CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &taskmodels.TaskSession{
		ID: sessionID, TaskID: taskID, State: taskmodels.TaskSessionStateRunning, StartedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskRepository(ctx, &taskmodels.TaskRepository{
		ID: "task-repo-" + taskID, TaskID: taskID, RepositoryID: "repo-1",
		CheckoutBranch: branch, CreatedAt: now, UpdatedAt: now,
	}))
	environmentID := "environment-" + taskID
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &taskmodels.TaskEnvironment{
		ID: environmentID, TaskID: taskID, ExecutorType: "worktree", WorkspacePath: t.TempDir(),
		Status: taskmodels.TaskEnvironmentStatusReady, CreatedAt: now, UpdatedAt: now,
		Repos: []*taskmodels.TaskEnvironmentRepo{{
			ID: "environment-repo-" + taskID, TaskEnvironmentID: environmentID,
			RepositoryID: "repo-1", WorktreeID: "worktree-" + taskID,
			WorktreeBranch: branch, Status: "active", CreatedAt: now, UpdatedAt: now,
		}},
	}))
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	session.TaskEnvironmentID = environmentID
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
}

func retirementIntegrationRouter(t *testing.T, service *Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewController(service, testLogger(t)).RegisterHTTPRoutes(router)
	return router
}

func retirementIntegrationRequest(router *gin.Engine, method, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func drainRetirementIntegrationCalls(calls <-chan string) []string {
	var result []string
	for {
		select {
		case branch := <-calls:
			result = append(result, branch)
		default:
			return result
		}
	}
}
