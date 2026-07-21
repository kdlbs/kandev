package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	usermodels "github.com/kandev/kandev/internal/user/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// captureOrchestrator records every LaunchSession request so tests can assert
// on the fields the handler set. prepErr, when non-nil, is returned from
// LaunchSession to short-circuit the two-phase create flow before its async
// start goroutine spawns (keeping assertions race-free).
type captureOrchestrator struct {
	mu                 sync.Mutex
	requests           []*orchestrator.LaunchSessionRequest
	prepErr            error
	startCreatedCalled chan struct{}
}

func (m *captureOrchestrator) LaunchSession(_ context.Context, req *orchestrator.LaunchSessionRequest) (*orchestrator.LaunchSessionResponse, error) {
	m.mu.Lock()
	m.requests = append(m.requests, req)
	m.mu.Unlock()
	if req.Intent == orchestrator.IntentStartCreated && m.startCreatedCalled != nil {
		select {
		case m.startCreatedCalled <- struct{}{}:
		default:
		}
	}
	if m.prepErr != nil {
		return nil, m.prepErr
	}
	return &orchestrator.LaunchSessionResponse{SessionID: "sess-1"}, nil
}

func (m *captureOrchestrator) EnsureSession(_ context.Context, _ string, _ ...orchestrator.EnsureSessionOptions) (*orchestrator.EnsureSessionResponse, error) {
	return nil, nil
}

func TestQuickChatRequestBuildRepositoriesAcceptsPluralInput(t *testing.T) {
	var body httpStartQuickChatRequest
	require.NoError(t, json.Unmarshal([]byte(`{
		"repositories": [
			{"repository_id":"repo-1","base_branch":"main"},
			{"repository_id":"repo-2","base_branch":"develop"}
		]
	}`), &body))

	repos := body.buildRepositories()

	require.Len(t, repos, 2)
	assert.Equal(t, service.TaskRepositoryInput{RepositoryID: "repo-1", BaseBranch: "main"}, repos[0])
	assert.Equal(t, service.TaskRepositoryInput{RepositoryID: "repo-2", BaseBranch: "develop"}, repos[1])
}

func TestQuickChatResolveParamsForcesWorktreeForRepositoryContext(t *testing.T) {
	defaultExecutor := models.ExecutorIDLocal
	body := httpStartQuickChatRequest{
		AgentProfileID: "profile-1",
		RepositoryID:   "repo-1",
		BaseBranch:     "main",
	}

	params := body.resolveParams(&models.Workspace{DefaultExecutorID: &defaultExecutor})

	assert.Equal(t, models.ExecutorIDWorktree, params.executorID)
}

type quickChatHandlerRepo struct {
	mockRepository
	taskRepos []*models.TaskRepository
}

func (r *quickChatHandlerRepo) GetWorkspace(_ context.Context, id string) (*models.Workspace, error) {
	defaultAgent := "profile-1"
	defaultExecutor := models.ExecutorIDLocal
	return &models.Workspace{
		ID:                    id,
		DefaultAgentProfileID: &defaultAgent,
		DefaultExecutorID:     &defaultExecutor,
	}, nil
}

func (r *quickChatHandlerRepo) GetRepository(_ context.Context, id string) (*models.Repository, error) {
	return &models.Repository{ID: id, WorkspaceID: "ws-1", Name: id, DefaultBranch: "main"}, nil
}

func (r *quickChatHandlerRepo) CreateTaskRepository(_ context.Context, taskRepo *models.TaskRepository) error {
	r.taskRepos = append(r.taskRepos, taskRepo)
	return nil
}

func (r *quickChatHandlerRepo) ListTaskRepositories(_ context.Context, _ string) ([]*models.TaskRepository, error) {
	return r.taskRepos, nil
}

func newQuickChatHandlerForTest(t *testing.T) (*TaskHandlers, *captureOrchestrator) {
	t.Helper()
	log := newTestLogger(t)
	repo := &quickChatHandlerRepo{}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	orch := &captureOrchestrator{}
	return &TaskHandlers{service: svc, orchestrator: orch, logger: log}, orch
}

func TestHTTPStartQuickChatRejectsInvalidRepositoryShapes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name string
		body string
	}{
		{
			name: "mixed legacy and plural repositories",
			body: `{"repository_id":"repo-legacy","repositories":[{"repository_id":"repo-1","base_branch":"main"}]}`,
		},
		{
			name: "same repository twice",
			body: `{"repositories":[{"repository_id":"repo-1","base_branch":"main"},{"repository_id":"repo-1","base_branch":"develop"}]}`,
		},
		{
			name: "empty repository id",
			body: `{"repositories":[{"repository_id":"","base_branch":"main"}]}`,
		},
		{
			name: "empty base branch",
			body: `{"repositories":[{"repository_id":"repo-1","base_branch":""}]}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, orch := newQuickChatHandlerForTest(t)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/workspaces/ws-1/quick-chat", strings.NewReader(tc.body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Params = gin.Params{{Key: "id", Value: "ws-1"}}

			h.httpStartQuickChat(c)

			assert.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", rec.Body.String())
			assert.Empty(t, orch.requests)
		})
	}
}

// TestStartAgentForNewTask_SetsDeferredStart pins the call-site half of the
// passthrough start_agent prompt-delivery fix: the synchronous prepare must
// carry DeferredStart=true so launchPrepare does not eagerly upgrade a
// passthrough profile into a promptless PTY launch and pre-empt the
// prompt-bearing IntentStartCreated that follows. Returning an error from the
// prepare call keeps the async start goroutine from spawning, so the assertion
// reads orch.requests without racing it.
func TestStartAgentForNewTask_SetsDeferredStart(t *testing.T) {
	orch := &captureOrchestrator{prepErr: errors.New("prepare failed")}
	h := &TaskHandlers{orchestrator: orch, logger: newTestLogger(t)}

	resp := &createTaskResponse{}
	body := httpCreateTaskRequest{StartAgent: true, AgentProfileID: "profile-1"}
	h.startAgentForNewTask(context.Background(), resp, "task-1", "do the thing", body, "step-1")

	orch.mu.Lock()
	defer orch.mu.Unlock()
	require.Len(t, orch.requests, 1, "prepare error must short-circuit before the async start goroutine")
	prep := orch.requests[0]
	assert.Equal(t, orchestrator.IntentPrepare, prep.Intent)
	assert.True(t, prep.DeferredStart,
		"sync prepare must defer the start so the passthrough PTY is launched with the prompt by the follow-up IntentStartCreated")
}

// configChatRepo returns a non-nil workspace so resolveConfigChatDefaults does
// not nil-deref; everything else inherits mockRepository's no-op stubs.
type configChatRepo struct {
	mockRepository
}

func (r *configChatRepo) GetWorkspace(_ context.Context, id string) (*models.Workspace, error) {
	return &models.Workspace{ID: id}, nil
}

// TestHttpStartConfigChat_SetsDeferredStart pins the second call site of the
// passthrough prompt-delivery fix. Unlike start_agent (always deferred), config
// chat defers the start only when a prompt is present — with no prompt there is
// no follow-up start, so the passthrough upgrade must stay on to give the
// terminal a PTY. Both branches are load-bearing, so both are pinned. The prepare
// LaunchSession is invoked synchronously before the async launchConfigChatAgent
// goroutine spawns, so requests[0] is the prepare and is read under the mock's
// mutex — race-free without asserting the (timing-dependent) total count.
func TestHttpStartConfigChat_SetsDeferredStart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := newTestLogger(t)

	cases := []struct {
		name              string
		prompt            string
		wantDeferredStart bool
	}{
		{name: "prompt present defers the start", prompt: "configure my workflow", wantDeferredStart: true},
		{name: "no prompt keeps the eager PTY upgrade", prompt: "", wantDeferredStart: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &configChatRepo{}
			svc := service.NewService(service.Repos{
				Workspaces: repo, Tasks: repo, TaskRepos: repo,
				Workflows: repo, Messages: repo, Turns: repo,
				Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
				Executors: repo, Environments: repo, TaskEnvironments: repo,
				Reviews: repo,
			}, nil, log, service.RepositoryDiscoveryConfig{})
			orch := &captureOrchestrator{}
			h := &TaskHandlers{service: svc, orchestrator: orch, logger: log}

			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			body := `{"agent_profile_id":"cfg-profile","prompt":` + strconv.Quote(tc.prompt) + `}`
			c.Request = httptest.NewRequest(http.MethodPost, "/workspaces/ws-1/config-chat", strings.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Params = gin.Params{{Key: "id", Value: "ws-1"}}

			h.httpStartConfigChat(c)

			orch.mu.Lock()
			defer orch.mu.Unlock()
			require.GreaterOrEqual(t, len(orch.requests), 1, "the synchronous prepare must have been issued")
			prep := orch.requests[0]
			assert.Equal(t, orchestrator.IntentPrepare, prep.Intent, "first call is the synchronous prepare")
			assert.Equal(t, tc.wantDeferredStart, prep.DeferredStart,
				"config-chat prepare must defer the start iff a prompt will be delivered by the follow-up IntentStartCreated")
		})
	}
}

type captureCreateTaskRepo struct {
	mockRepository
	captured       *models.Task
	updateStateErr error
}

type captureTaskCreateLastUsedRecorder struct {
	calls int
	got   usermodels.TaskCreateLastUsed
}

func (m *captureTaskCreateLastUsedRecorder) RecordTaskCreateLastUsed(_ context.Context, patch usermodels.TaskCreateLastUsed) error {
	m.calls++
	m.got = patch
	return nil
}

func (m *captureCreateTaskRepo) GetWorkspaceTaskPrefix(_ context.Context, _ string) (string, string, error) {
	return "KAN", "wf-office", nil
}

func (m *captureCreateTaskRepo) GetRepository(_ context.Context, id string) (*models.Repository, error) {
	if id == "repo-2" {
		return &models.Repository{ID: id, WorkspaceID: "ws-1", DefaultBranch: "main"}, nil
	}
	return nil, errors.New("repository not found")
}

func (m *captureCreateTaskRepo) CreateTask(_ context.Context, task *models.Task) error {
	m.captured = task
	return nil
}

func (m *captureCreateTaskRepo) GetTask(_ context.Context, id string) (*models.Task, error) {
	if m.captured == nil || m.captured.ID != id {
		return nil, errors.New("task not found")
	}
	return m.captured, nil
}

func (m *captureCreateTaskRepo) UpdateTaskState(_ context.Context, id string, state v1.TaskState) error {
	if m.captured == nil || m.captured.ID != id {
		return errors.New("task not found")
	}
	if m.updateStateErr != nil {
		return m.updateStateErr
	}
	m.captured.State = state
	return nil
}

// TestHTTPCreateTask_ProjectIDReachesOfficePath guards the wiring that broke
// the office "New Task" dialog: when the request body sets project_id (and
// omits workflow_id), the handler must forward it to the service so
// isOfficeRequest() returns true and the workflow is auto-resolved. Without
// this, requests fall through to the kanban validator with
// "workflow_id is required for non-ephemeral tasks".
func TestHTTPCreateTask_ProjectIDReachesOfficePath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := newTestLogger(t)

	repo := &captureCreateTaskRepo{}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	h := &TaskHandlers{service: svc, logger: log}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(`{
		"workspace_id": "ws-1",
		"title": "Analyse integrations",
		"project_id": "proj-1",
		"priority": "medium"
	}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.httpCreateTask(c)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	require.NotNil(t, repo.captured, "service.CreateTask was not called")
	assert.Equal(t, "proj-1", repo.captured.ProjectID)
	assert.Equal(t, "wf-office", repo.captured.WorkflowID, "office workflow should be auto-resolved")
}

func TestHTTPCreateTaskRecordsFinalLastUsedSelections(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := newTestLogger(t)

	repo := &captureCreateTaskRepo{}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	recorder := &captureTaskCreateLastUsedRecorder{}
	h := &TaskHandlers{service: svc, taskCreateLastUsedRecorder: recorder, logger: log}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(`{
		"workspace_id": "ws-1",
		"workflow_id": "wf-1",
		"workflow_step_id": "step-1",
		"title": "Use current selections",
		"repositories": [{
			"repository_id": "repo-2",
			"base_branch": "main",
			"checkout_branch": "feature/current"
		}],
		"agent_profile_id": "agent-2",
		"executor_profile_id": "exec-profile-2"
	}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.httpCreateTask(c)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	require.Equal(t, 1, recorder.calls)
	assert.Equal(t, usermodels.TaskCreateLastUsed{
		RepositoryID:      "repo-2",
		Branch:            "feature/current",
		AgentProfileID:    "agent-2",
		ExecutorProfileID: "exec-profile-2",
	}, recorder.got)
}

func TestHTTPCreateTaskRecordsRepositoryWithoutProfileIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := newTestLogger(t)

	repo := &captureCreateTaskRepo{}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	recorder := &captureTaskCreateLastUsedRecorder{}
	h := &TaskHandlers{service: svc, taskCreateLastUsedRecorder: recorder, logger: log}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(`{
		"workspace_id": "ws-1",
		"workflow_id": "wf-1",
		"workflow_step_id": "step-1",
		"title": "Passive API task",
		"repositories": [{
			"repository_id": "repo-2",
			"base_branch": "main"
		}]
	}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.httpCreateTask(c)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	require.Equal(t, 1, recorder.calls)
	assert.Equal(t, usermodels.TaskCreateLastUsed{
		RepositoryID: "repo-2",
		Branch:       "main",
	}, recorder.got)
}

func TestConvertCreateTaskRepositoriesForwardsPRNumber(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	repos, ok := convertCreateTaskRepositories(c, []httpTaskRepositoryInput{{
		GitHubURL:      "https://github.com/kdlbs/kandev",
		BaseBranch:     "main",
		CheckoutBranch: "feature/fork-pr",
		PRNumber:       1567,
	}})

	require.True(t, ok)
	require.Len(t, repos, 1)
	assert.Equal(t, dto.TaskRepositoryInput{
		BaseBranch:     "main",
		CheckoutBranch: "feature/fork-pr",
		PRNumber:       1567,
		GitHubURL:      "https://github.com/kdlbs/kandev",
	}, repos[0])
}

func TestBuildTaskCreateLastUsedPatchRecordsFirstWorkspaceRepository(t *testing.T) {
	patch := buildTaskCreateLastUsedPatch(httpCreateTaskRequest{
		AgentProfileID:    "agent-2",
		ExecutorProfileID: "exec-profile-2",
	}, []dto.TaskRepositoryInput{
		{RepositoryID: "repo-without-branch"},
		{RepositoryID: "repo-with-branch", CheckoutBranch: "feature/current"},
	})

	assert.Equal(t, usermodels.TaskCreateLastUsed{
		RepositoryID:      "repo-without-branch",
		AgentProfileID:    "agent-2",
		ExecutorProfileID: "exec-profile-2",
	}, patch)
}

func TestBuildTaskCreateLastUsedPatchUsesFreshBranchRequestBase(t *testing.T) {
	patch := buildTaskCreateLastUsedPatch(httpCreateTaskRequest{
		Repositories: []httpTaskRepositoryInput{{
			RepositoryID:   "repo-2",
			BaseBranch:     "main",
			CheckoutBranch: "task/use-current-selections",
			FreshBranch:    true,
		}},
	}, []dto.TaskRepositoryInput{{
		RepositoryID: "repo-2",
		BaseBranch:   "task/use-current-selections",
	}})

	assert.Equal(t, usermodels.TaskCreateLastUsed{
		RepositoryID: "repo-2",
		Branch:       "main",
	}, patch)
}

func TestBuildTaskCreateLastUsedPatchRecordsRepositoryWithoutBranch(t *testing.T) {
	patch := buildTaskCreateLastUsedPatch(httpCreateTaskRequest{
		AgentProfileID: "agent-2",
	}, []dto.TaskRepositoryInput{
		{RepositoryID: "repo-without-branch"},
	})

	assert.Equal(t, usermodels.TaskCreateLastUsed{
		RepositoryID:   "repo-without-branch",
		AgentProfileID: "agent-2",
	}, patch)
}

func TestBuildTaskCreateLastUsedPatchSkipsBranchWithoutWorkspaceRepository(t *testing.T) {
	patch := buildTaskCreateLastUsedPatch(httpCreateTaskRequest{
		AgentProfileID: "agent-2",
	}, []dto.TaskRepositoryInput{
		{LocalPath: "/tmp/repo", CheckoutBranch: "feature/local"},
		{GitHubURL: "https://github.com/kdlbs/example", BaseBranch: "main"},
	})

	assert.Equal(t, usermodels.TaskCreateLastUsed{
		AgentProfileID: "agent-2",
	}, patch)
}

func TestWSCreateTaskRecordsFinalLastUsedSelections(t *testing.T) {
	log := newTestLogger(t)

	repo := &captureCreateTaskRepo{}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	recorder := &captureTaskCreateLastUsedRecorder{}
	h := &TaskHandlers{service: svc, taskCreateLastUsedRecorder: recorder, logger: log}

	msg, err := ws.NewRequest("msg-1", ws.ActionTaskCreate, map[string]any{
		"workspace_id":        "ws-1",
		"workflow_id":         "wf-1",
		"workflow_step_id":    "step-1",
		"title":               "Use current selections",
		"agent_profile_id":    "agent-2",
		"executor_profile_id": "exec-profile-2",
		"repositories": []map[string]any{{
			"repository_id":   "repo-2",
			"checkout_branch": "feature/current",
		}},
	})
	require.NoError(t, err)

	resp, err := h.wsCreateTask(context.Background(), msg)

	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	require.Equal(t, 1, recorder.calls)
	assert.Equal(t, usermodels.TaskCreateLastUsed{
		RepositoryID:      "repo-2",
		Branch:            "feature/current",
		AgentProfileID:    "agent-2",
		ExecutorProfileID: "exec-profile-2",
	}, recorder.got)
}

func TestWSCreateTaskRecordsFreshBranchRequestBase(t *testing.T) {
	log := newTestLogger(t)

	repo := &captureCreateTaskRepo{}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	recorder := &captureTaskCreateLastUsedRecorder{}
	h := &TaskHandlers{service: svc, taskCreateLastUsedRecorder: recorder, logger: log}

	msg, err := ws.NewRequest("msg-1", ws.ActionTaskCreate, map[string]any{
		"workspace_id":     "ws-1",
		"workflow_id":      "wf-1",
		"workflow_step_id": "step-1",
		"title":            "Use fresh branch",
		"repositories": []map[string]any{{
			"repository_id":   "repo-2",
			"base_branch":     "main",
			"checkout_branch": "task/use-current-selections",
			"fresh_branch":    true,
		}},
	})
	require.NoError(t, err)

	resp, err := h.wsCreateTask(context.Background(), msg)

	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	require.Equal(t, 1, recorder.calls)
	assert.Equal(t, usermodels.TaskCreateLastUsed{
		RepositoryID: "repo-2",
		Branch:       "main",
	}, recorder.got)
}

func TestHTTPCreateTask_StartAgentReturnsSchedulingTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := newTestLogger(t)

	repo := &captureCreateTaskRepo{}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	startCreatedCalled := make(chan struct{}, 1)
	orch := &captureOrchestrator{startCreatedCalled: startCreatedCalled}
	h := &TaskHandlers{service: svc, orchestrator: orch, logger: log}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(`{
		"workspace_id": "ws-1",
		"workflow_id": "wf-1",
		"workflow_step_id": "step-1",
		"title": "Boot an agent",
		"description": "Do the thing",
		"priority": "medium",
		"agent_profile_id": "profile-1",
		"start_agent": true
	}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.httpCreateTask(c)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	var response createTaskResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	assert.Equal(t, v1.TaskStateScheduling, repo.captured.State)
	assert.Equal(t, v1.TaskStateScheduling, response.State)
	assert.Equal(t, "sess-1", response.TaskSessionID)
	requireStartCreatedLaunch(t, startCreatedCalled)
}

func TestHTTPCreateTask_StartAgentKeepsCreatedStateWhenSchedulingUpdateFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := newTestLogger(t)

	repo := &captureCreateTaskRepo{updateStateErr: errors.New("database locked")}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	startCreatedCalled := make(chan struct{}, 1)
	orch := &captureOrchestrator{startCreatedCalled: startCreatedCalled}
	h := &TaskHandlers{service: svc, orchestrator: orch, logger: log}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(`{
		"workspace_id": "ws-1",
		"workflow_id": "wf-1",
		"workflow_step_id": "step-1",
		"title": "Boot an agent",
		"description": "Do the thing",
		"priority": "medium",
		"agent_profile_id": "profile-1",
		"start_agent": true
	}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.httpCreateTask(c)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	var response createTaskResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	assert.Equal(t, v1.TaskStateCreated, repo.captured.State)
	assert.Equal(t, v1.TaskStateCreated, response.State)
	assert.Equal(t, "sess-1", response.TaskSessionID)
	requireStartCreatedLaunch(t, startCreatedCalled)
}

func requireStartCreatedLaunch(t *testing.T, started <-chan struct{}) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(1 * time.Second):
		t.Fatal("async IntentStartCreated launch did not complete")
	}
}

func TestValidateAttachments_DeliveryMode(t *testing.T) {
	base := v1.MessageAttachment{
		Type:     "resource",
		MimeType: "text/plain",
		Data:     "dGVzdA==",
	}

	tests := []struct {
		name         string
		deliveryMode string
		wantErr      string
	}{
		{name: "empty uses default", deliveryMode: ""},
		{name: "prompt is valid", deliveryMode: "prompt"},
		{name: "path is valid", deliveryMode: "path"},
		{name: "inline is rejected", deliveryMode: "inline", wantErr: "delivery_mode must be prompt or path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attachment := base
			attachment.DeliveryMode = tt.deliveryMode

			err := validateAttachments([]v1.MessageAttachment{attachment})
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func newTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:      "debug",
		Format:     "console",
		OutputPath: "stdout",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	return log
}

// subtaskCountRepo lets the subtask-count handler test drive
// ListChildren to specific values / errors without standing up a real
// SQLite repo.
type subtaskCountRepo struct {
	mockRepository
	children []*models.Task
	err      error
}

func (r *subtaskCountRepo) ListChildren(_ context.Context, _ string) ([]*models.Task, error) {
	return r.children, r.err
}

func (r *subtaskCountRepo) CountToolCallMessagesBySession(
	_ context.Context, _ []string,
) (map[string]int, error) {
	return nil, nil
}

func TestHTTPTaskSubtaskCount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := newTestLogger(t)

	t.Run("returns count for task with subtasks", func(t *testing.T) {
		repo := &subtaskCountRepo{children: []*models.Task{{ID: "c1"}, {ID: "c2"}, {ID: "c3"}}}
		h := &TaskHandlers{repo: repo, logger: log}
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/tasks/root/subtask-count", nil)
		c.Params = gin.Params{{Key: "id", Value: "root"}}

		h.httpTaskSubtaskCount(c)

		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		assert.JSONEq(t, `{"count":3}`, rec.Body.String())
	})

	t.Run("returns zero for task with no subtasks", func(t *testing.T) {
		repo := &subtaskCountRepo{children: nil}
		h := &TaskHandlers{repo: repo, logger: log}
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/tasks/root/subtask-count", nil)
		c.Params = gin.Params{{Key: "id", Value: "root"}}

		h.httpTaskSubtaskCount(c)

		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		assert.JSONEq(t, `{"count":0}`, rec.Body.String())
	})

	t.Run("returns 500 with a generic error on repo failure", func(t *testing.T) {
		repo := &subtaskCountRepo{err: errors.New("sql: connection refused: postgres://user@host/db")}
		h := &TaskHandlers{repo: repo, logger: log}
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/tasks/root/subtask-count", nil)
		c.Params = gin.Params{{Key: "id", Value: "root"}}

		h.httpTaskSubtaskCount(c)

		require.Equal(t, http.StatusInternalServerError, rec.Code)
		// Must NOT leak the raw error (would expose DSN / driver details).
		assert.NotContains(t, rec.Body.String(), "postgres://")
		assert.Contains(t, rec.Body.String(), "failed to count subtasks")
	})
}

func TestHandleSelectedMoveError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := newTestLogger(t)

	tests := []struct {
		name             string
		err              error
		want             int
		wantBodyContains string
	}{
		{
			name: "not found",
			err:  errors.New("task not found: task-1"),
			want: http.StatusNotFound,
		},
		{
			name: "move conflict",
			err:  errors.New("task task-1 cannot be moved: task has an active session (running)"),
			want: http.StatusConflict,
		},
		{
			name: "bad request validation",
			err:  errors.New("invalid workflow id"),
			want: http.StatusBadRequest,
		},
		{
			name:             "internal",
			err:              errors.New("failed to count target workflow step tasks: database is locked"),
			want:             http.StatusInternalServerError,
			wantBodyContains: "task move failed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)

			handleSelectedMoveError(c, log, tc.err)

			assert.Equal(t, tc.want, rec.Code)
			if tc.wantBodyContains != "" {
				assert.Contains(t, rec.Body.String(), tc.wantBodyContains)
			}
		})
	}
}

type moveTaskConflictRepo struct {
	mockRepository
	task      *models.Task
	sessions  []*models.TaskSession
	workflows map[string]*models.Workflow
}

func (m *moveTaskConflictRepo) GetTask(ctx context.Context, id string) (*models.Task, error) {
	return m.task, nil
}

func (m *moveTaskConflictRepo) UpdateTask(ctx context.Context, task *models.Task) error {
	m.task = task
	return nil
}

func (m *moveTaskConflictRepo) GetWorkflow(ctx context.Context, id string) (*models.Workflow, error) {
	if m.workflows != nil {
		if workflow, ok := m.workflows[id]; ok {
			return workflow, nil
		}
	}
	return &models.Workflow{ID: id, WorkspaceID: m.task.WorkspaceID}, nil
}

func (m *moveTaskConflictRepo) ListTaskSessions(ctx context.Context, taskID string) ([]*models.TaskSession, error) {
	return m.sessions, nil
}

func (m *moveTaskConflictRepo) GetPrimarySessionByTaskID(ctx context.Context, taskID string) (*models.TaskSession, error) {
	for _, session := range m.sessions {
		if session.TaskID == taskID && session.IsPrimary {
			return session, nil
		}
	}
	return nil, nil
}

func TestHTTPMoveTaskMapsMoveConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := newTestLogger(t)
	archivedAt := time.Now().UTC()

	tests := []struct {
		name     string
		task     *models.Task
		sessions []*models.TaskSession
	}{
		{
			name: "archived task",
			task: &models.Task{
				ID:             "task-archived",
				WorkspaceID:    "workspace-1",
				WorkflowID:     "wf-source",
				WorkflowStepID: "step-source",
				ArchivedAt:     &archivedAt,
			},
		},
		{
			name: "active non-primary session",
			task: &models.Task{
				ID:             "task-running",
				WorkspaceID:    "workspace-1",
				WorkflowID:     "wf-source",
				WorkflowStepID: "step-source",
			},
			sessions: []*models.TaskSession{{
				ID:     "session-running",
				TaskID: "task-running",
				State:  models.TaskSessionStateRunning,
			}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &moveTaskConflictRepo{task: tc.task, sessions: tc.sessions}
			svc := service.NewService(service.Repos{
				Workspaces: repo, Tasks: repo, TaskRepos: repo,
				Workflows: repo, Messages: repo, Turns: repo,
				Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
				Executors: repo, Environments: repo, TaskEnvironments: repo,
				Reviews: repo,
			}, nil, log, service.RepositoryDiscoveryConfig{})
			h := &TaskHandlers{service: svc, logger: log}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Params = gin.Params{{Key: "id", Value: tc.task.ID}}
			c.Request = httptest.NewRequest(http.MethodPost, "/tasks/"+tc.task.ID+"/move", strings.NewReader(`{
				"workflow_id": "wf-target",
				"workflow_step_id": "step-target",
				"position": 0
			}`))
			c.Request.Header.Set("Content-Type", "application/json")

			h.httpMoveTask(c)

			assert.Equal(t, http.StatusConflict, rec.Code)
		})
	}
}

func TestHTTPMoveTaskAllowsRunningPrimarySession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := newTestLogger(t)
	task := &models.Task{
		ID:             "task-running-primary",
		WorkspaceID:    "workspace-1",
		WorkflowID:     "wf-source",
		WorkflowStepID: "step-source",
	}
	repo := &moveTaskConflictRepo{
		task: task,
		sessions: []*models.TaskSession{{
			ID:        "session-running-primary",
			TaskID:    task.ID,
			State:     models.TaskSessionStateRunning,
			IsPrimary: true,
		}},
	}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	h := &TaskHandlers{service: svc, logger: log}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "id", Value: task.ID}}
	c.Request = httptest.NewRequest(http.MethodPost, "/tasks/"+task.ID+"/move", strings.NewReader(`{
		"workflow_id": "wf-target",
		"workflow_step_id": "step-target",
		"position": 0
	}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.httpMoveTask(c)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.Equal(t, "wf-target", repo.task.WorkflowID)
	assert.Equal(t, "step-target", repo.task.WorkflowStepID)
}

func TestResolveFreshBranchName(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		taskTitle string
		assert    func(t *testing.T, got string)
	}{
		{
			name:      "uses raw name when provided",
			raw:       "feature/x",
			taskTitle: "ignored",
			assert: func(t *testing.T, got string) {
				if got != "feature/x" {
					t.Fatalf("expected feature/x, got %q", got)
				}
			},
		},
		{
			name:      "trims whitespace from raw name",
			raw:       "  feature/y  ",
			taskTitle: "ignored",
			assert: func(t *testing.T, got string) {
				if got != "feature/y" {
					t.Fatalf("expected feature/y, got %q", got)
				}
			},
		},
		{
			name:      "derives from title when raw is empty",
			raw:       "",
			taskTitle: "Fix login bug",
			assert: func(t *testing.T, got string) {
				if !strings.HasPrefix(got, "fix-login-bug_") {
					t.Fatalf("expected fix-login-bug_ prefix, got %q", got)
				}
			},
		},
		{
			name:      "title with only special chars falls back to suffix only",
			raw:       "",
			taskTitle: "!!!",
			assert: func(t *testing.T, got string) {
				// SemanticWorktreeName returns just the suffix (3 chars from
				// the alphabet) when the sanitized title is empty.
				if len(got) != 3 {
					t.Fatalf("expected 3-char suffix, got %q (len %d)", got, len(got))
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.assert(t, resolveFreshBranchName(tc.raw, tc.taskTitle))
		})
	}
}

type freshBranchIdentityRepository struct {
	mockRepository
	task         *models.Task
	taskRepos    []*models.TaskRepository
	repositories map[string]*models.Repository
	listTaskErr  error
	deletedTask  bool
}

func (r *freshBranchIdentityRepository) GetTask(_ context.Context, _ string) (*models.Task, error) {
	return r.task, nil
}

func (r *freshBranchIdentityRepository) DeleteTask(_ context.Context, _ string) error {
	r.deletedTask = true
	return nil
}

func (r *freshBranchIdentityRepository) ListTaskRepositories(_ context.Context, _ string) ([]*models.TaskRepository, error) {
	return r.taskRepos, r.listTaskErr
}

func (r *freshBranchIdentityRepository) GetRepository(_ context.Context, id string) (*models.Repository, error) {
	repo, ok := r.repositories[id]
	if !ok {
		return nil, errors.New("repository not found")
	}
	return repo, nil
}

func (r *freshBranchIdentityRepository) DeleteTaskRepositoriesByTask(_ context.Context, _ string) error {
	r.taskRepos = nil
	return nil
}

func (r *freshBranchIdentityRepository) CreateTaskRepository(_ context.Context, taskRepo *models.TaskRepository) error {
	r.taskRepos = append(r.taskRepos, taskRepo)
	return nil
}

func TestCommitFreshBranchUsesPersistedTaskRepositoryIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	realRepoPath := initHandlerGitRepository(t, filepath.Join(t.TempDir(), "real"))
	decoyRepoPath := initHandlerGitRepository(t, filepath.Join(t.TempDir(), "decoy"))
	repo := &freshBranchIdentityRepository{
		task: &models.Task{ID: "task-1", WorkspaceID: "ws-1"},
		taskRepos: []*models.TaskRepository{{
			ID: "task-repo-1", TaskID: "task-1", RepositoryID: "real-repo", BaseBranch: "main", Position: 0,
		}},
		repositories: map[string]*models.Repository{
			"real-repo": {ID: "real-repo", WorkspaceID: "ws-1", Name: "real", SourceType: "local", LocalPath: realRepoPath},
		},
	}
	log := newTestLogger(t)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo, Workflows: repo,
		Messages: repo, Turns: repo, Sessions: repo, GitSnapshots: repo,
		RepoEntities: repo, Executors: repo, Environments: repo,
		TaskEnvironments: repo, Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{Roots: []string{filepath.Dir(decoyRepoPath)}})
	handler := &TaskHandlers{service: svc, logger: log}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/tasks", nil)
	inputs := []httpTaskRepositoryInput{{
		RepositoryID: "real-repo", LocalPath: decoyRepoPath, BaseBranch: "main",
		FreshBranch: true, NewBranchName: "feature/identity",
	}}
	repos := []dto.TaskRepositoryInput{{RepositoryID: "real-repo", BaseBranch: "main", LocalPath: decoyRepoPath}}

	if ok := handler.commitFreshBranch(c, "task-1", "Identity", "ws-1", inputs, repos); !ok {
		t.Fatalf("commitFreshBranch failed: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if branch := handlerGitCurrentBranch(t, realRepoPath); branch != "feature/identity" {
		t.Fatalf("persisted repository branch = %q, want feature/identity", branch)
	}
	if branch := handlerGitCurrentBranch(t, decoyRepoPath); branch != "main" {
		t.Fatalf("request path branch = %q, want unchanged main", branch)
	}
	if len(repo.taskRepos) != 1 || repo.taskRepos[0].BaseBranch != "feature/identity" {
		t.Fatalf("persisted task repositories = %+v, want rewritten feature/identity branch", repo.taskRepos)
	}
}

func TestCommitFreshBranchRollsBackTaskWhenPersistedRepositoriesCannotBeLoaded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &freshBranchIdentityRepository{
		task:        &models.Task{ID: "task-1", WorkspaceID: "ws-1"},
		listTaskErr: errors.New("database unavailable"),
	}
	log := newTestLogger(t)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo, Workflows: repo,
		Messages: repo, Turns: repo, Sessions: repo, GitSnapshots: repo,
		RepoEntities: repo, Executors: repo, Environments: repo,
		TaskEnvironments: repo, Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	handler := &TaskHandlers{service: svc, logger: log}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/tasks", nil)
	inputs := []httpTaskRepositoryInput{{RepositoryID: "repo-1", FreshBranch: true}}
	repos := []dto.TaskRepositoryInput{{RepositoryID: "repo-1"}}

	if ok := handler.commitFreshBranch(c, "task-1", "Identity", "ws-1", inputs, repos); ok {
		t.Fatal("commitFreshBranch succeeded despite task repository load error")
	}
	if !repo.deletedTask {
		t.Fatal("created task was not rolled back")
	}
}

func initHandlerGitRepository(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"commit", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = path
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	return path
}

func handlerGitCurrentBranch(t *testing.T, path string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("git current branch: %v", err)
	}
	return strings.TrimSpace(string(output))
}

func TestAssociatePRFromRepoInputs(t *testing.T) {
	log := newTestLogger(t)

	t.Run("calls callback when repo input contains PR URL", func(t *testing.T) {
		var mu sync.Mutex
		var called bool
		var gotTaskID, gotSessionID, gotPRURL, gotBranch string

		h := &TaskHandlers{logger: log}
		h.SetOnTaskCreatedWithPR(func(ctx context.Context, taskID, sessionID, prURL, branch string) {
			mu.Lock()
			defer mu.Unlock()
			called = true
			gotTaskID = taskID
			gotSessionID = sessionID
			gotPRURL = prURL
			gotBranch = branch
		})

		// The callback runs in a goroutine, so we need a channel to sync
		done := make(chan struct{})
		h.onTaskCreatedWithPR = func(ctx context.Context, taskID, sessionID, prURL, branch string) {
			defer close(done)
			mu.Lock()
			defer mu.Unlock()
			called = true
			gotTaskID = taskID
			gotSessionID = sessionID
			gotPRURL = prURL
			gotBranch = branch
		}

		h.associatePRFromRepoInputs("task-1", "session-1", []httpTaskRepositoryInput{
			{
				GitHubURL:      "https://github.com/owner/repo/pull/123",
				CheckoutBranch: "feature-branch",
			},
		})

		<-done

		mu.Lock()
		defer mu.Unlock()
		require.True(t, called)
		assert.Equal(t, "task-1", gotTaskID)
		assert.Equal(t, "session-1", gotSessionID)
		assert.Equal(t, "https://github.com/owner/repo/pull/123", gotPRURL)
		assert.Equal(t, "feature-branch", gotBranch)
	})

	t.Run("does not call callback for plain repo URLs", func(t *testing.T) {
		called := false
		h := &TaskHandlers{logger: log}
		h.SetOnTaskCreatedWithPR(func(ctx context.Context, taskID, sessionID, prURL, branch string) {
			called = true
		})

		h.associatePRFromRepoInputs("task-1", "", []httpTaskRepositoryInput{
			{
				GitHubURL:      "https://github.com/owner/repo",
				CheckoutBranch: "main",
			},
		})

		assert.False(t, called)
	})

	t.Run("does not call callback when no onTaskCreatedWithPR set", func(t *testing.T) {
		h := &TaskHandlers{logger: log}
		// Should not panic
		h.associatePRFromRepoInputs("task-1", "", []httpTaskRepositoryInput{
			{
				GitHubURL:      "https://github.com/owner/repo/pull/123",
				CheckoutBranch: "feature-branch",
			},
		})
	})

	t.Run("passes empty session ID when no session created", func(t *testing.T) {
		done := make(chan struct{})
		var gotSessionID string

		h := &TaskHandlers{logger: log}
		h.onTaskCreatedWithPR = func(ctx context.Context, taskID, sessionID, prURL, branch string) {
			defer close(done)
			gotSessionID = sessionID
		}

		h.associatePRFromRepoInputs("task-1", "", []httpTaskRepositoryInput{
			{
				GitHubURL:      "https://github.com/owner/repo/pull/456",
				CheckoutBranch: "fix-branch",
			},
		})

		<-done
		assert.Equal(t, "", gotSessionID)
	})

	t.Run("only processes first PR URL when multiple repos have PRs", func(t *testing.T) {
		var count int
		var mu sync.Mutex
		done := make(chan struct{})

		h := &TaskHandlers{logger: log}
		h.onTaskCreatedWithPR = func(ctx context.Context, taskID, sessionID, prURL, branch string) {
			defer close(done)
			mu.Lock()
			defer mu.Unlock()
			count++
		}

		h.associatePRFromRepoInputs("task-1", "", []httpTaskRepositoryInput{
			{
				GitHubURL:      "https://github.com/owner/repo/pull/1",
				CheckoutBranch: "branch-1",
			},
			{
				GitHubURL:      "https://github.com/owner/repo/pull/2",
				CheckoutBranch: "branch-2",
			},
		})

		<-done
		mu.Lock()
		defer mu.Unlock()
		assert.Equal(t, 1, count)
	})
}
