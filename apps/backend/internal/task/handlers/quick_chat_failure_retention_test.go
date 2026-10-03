package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type quickChatRetentionRepo struct {
	mockRepository
	taskRepos      []*models.TaskRepository
	task           *models.Task
	deletedTaskID  string
	sessionsByTask map[string][]*models.TaskSession
	primaryByTask  map[string]*models.TaskSession
	lookupErr      error
}

func (r *quickChatRetentionRepo) GetWorkspace(_ context.Context, id string) (*models.Workspace, error) {
	defaultAgent := "profile-1"
	defaultExecutor := models.ExecutorIDLocal
	return &models.Workspace{
		ID:                    id,
		DefaultAgentProfileID: &defaultAgent,
		DefaultExecutorID:     &defaultExecutor,
	}, nil
}

func (r *quickChatRetentionRepo) CreateTask(_ context.Context, task *models.Task) error {
	r.task = task
	return nil
}

func (r *quickChatRetentionRepo) GetTask(_ context.Context, id string) (*models.Task, error) {
	if r.task != nil && r.task.ID == id && r.deletedTaskID != id {
		return r.task, nil
	}
	return nil, nil
}

func (r *quickChatRetentionRepo) DeleteTask(_ context.Context, id string) error {
	r.deletedTaskID = id
	return nil
}

func (r *quickChatRetentionRepo) CreateTaskRepository(_ context.Context, taskRepo *models.TaskRepository) error {
	r.taskRepos = append(r.taskRepos, taskRepo)
	return nil
}

func (r *quickChatRetentionRepo) ListTaskRepositories(_ context.Context, _ string) ([]*models.TaskRepository, error) {
	return r.taskRepos, nil
}

func (r *quickChatRetentionRepo) GetPrimarySessionByTaskID(ctx context.Context, taskID string) (*models.TaskSession, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.lookupErr != nil {
		return nil, r.lookupErr
	}
	if r.primaryByTask != nil {
		if session := r.primaryByTask[taskID]; session != nil {
			return session, nil
		}
	}
	return nil, errors.New("no primary session")
}

func (r *quickChatRetentionRepo) ListTaskSessions(ctx context.Context, taskID string) ([]*models.TaskSession, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.lookupErr != nil {
		return nil, r.lookupErr
	}
	if r.sessionsByTask != nil {
		return r.sessionsByTask[taskID], nil
	}
	return nil, nil
}

func (r *quickChatRetentionRepo) ListTasksByWorkspace(_ context.Context, workspaceID, _, _, _ string, _, _ int, _ string, _, _, _, _ bool) ([]*models.Task, int, error) {
	if r.task != nil && r.task.WorkspaceID == workspaceID && r.deletedTaskID != r.task.ID {
		return []*models.Task{r.task}, 1, nil
	}
	return nil, 0, nil
}

func (r *quickChatRetentionRepo) GetPrimarySessionInfoByTaskIDs(_ context.Context, taskIDs []string) (map[string]*models.TaskSession, error) {
	result := make(map[string]*models.TaskSession)
	for _, id := range taskIDs {
		if r.primaryByTask != nil && r.primaryByTask[id] != nil {
			result[id] = r.primaryByTask[id]
		}
	}
	return result, nil
}

type retentionOrchestrator struct {
	captureOrchestrator
	launchFn func(ctx context.Context, req *orchestrator.LaunchSessionRequest) (*orchestrator.LaunchSessionResponse, error)
}

func (m *retentionOrchestrator) LaunchSession(ctx context.Context, req *orchestrator.LaunchSessionRequest) (*orchestrator.LaunchSessionResponse, error) {
	if m.launchFn != nil {
		return m.launchFn(ctx, req)
	}
	return m.captureOrchestrator.LaunchSession(ctx, req)
}

func TestQuickChatFailureRetention_PostAllocationFailure_RetainsIdentityAndReturns500WithIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := newTestLogger(t)
	repo := &quickChatRetentionRepo{
		sessionsByTask: make(map[string][]*models.TaskSession),
		primaryByTask:  make(map[string]*models.TaskSession),
	}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})

	orch := &retentionOrchestrator{
		launchFn: func(_ context.Context, req *orchestrator.LaunchSessionRequest) (*orchestrator.LaunchSessionResponse, error) {
			session := &models.TaskSession{
				ID:             "sess-retained-1",
				TaskID:         req.TaskID,
				AgentProfileID: req.AgentProfileID,
				IsPrimary:      true,
				State:          models.TaskSessionStateFailed,
				StartedAt:      time.Now().UTC(),
			}
			repo.sessionsByTask[req.TaskID] = []*models.TaskSession{session}
			repo.primaryByTask[req.TaskID] = session
			return nil, errors.New("agent bootstrap failed")
		},
	}

	h := &TaskHandlers{service: svc, orchestrator: orch, logger: log}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/workspaces/ws-1/quick-chat", strings.NewReader(`{
		"agent_profile_id":"profile-1"
	}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "ws-1"}}

	h.httpStartQuickChat(c)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "failed to start session", resp["error"])
	assert.NotEmpty(t, resp["task_id"])
	assert.Equal(t, "sess-retained-1", resp["session_id"])
	assert.Empty(t, repo.deletedTaskID, "ephemeral task should not be deleted on post-allocation failure")

	// Ensure the retained session is restorable in ListQuickChatSessions.
	items, err := svc.ListQuickChatSessions(context.Background(), "ws-1")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "sess-retained-1", items[0].SessionID)
	assert.Equal(t, repo.task.ID, items[0].TaskID)
}

func TestQuickChatFailureRetention_PreSessionFailure_DeletesTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := newTestLogger(t)
	repo := &quickChatRetentionRepo{}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})

	orch := &retentionOrchestrator{
		launchFn: func(_ context.Context, _ *orchestrator.LaunchSessionRequest) (*orchestrator.LaunchSessionResponse, error) {
			return nil, errors.New("cannot allocate session")
		},
	}

	h := &TaskHandlers{service: svc, orchestrator: orch, logger: log}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/workspaces/ws-1/quick-chat", strings.NewReader(`{
		"agent_profile_id":"profile-1"
	}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "ws-1"}}

	h.httpStartQuickChat(c)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "failed to start session", resp["error"])
	assert.Nil(t, resp["task_id"])
	assert.Nil(t, resp["session_id"])
	assert.NotEmpty(t, repo.deletedTaskID, "task should be cleaned up on pre-session failure")
}

func TestQuickChatFailureRetention_CanceledRequest_RetainsIdentity(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gin.SetMode(gin.TestMode)
	log := newTestLogger(t)
	repo := &quickChatRetentionRepo{
		sessionsByTask: make(map[string][]*models.TaskSession),
		primaryByTask:  make(map[string]*models.TaskSession),
	}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})

	orch := &retentionOrchestrator{
		launchFn: func(_ context.Context, req *orchestrator.LaunchSessionRequest) (*orchestrator.LaunchSessionResponse, error) {
			session := &models.TaskSession{
				ID:             "sess-retained-1",
				TaskID:         req.TaskID,
				AgentProfileID: req.AgentProfileID,
				IsPrimary:      true,
				State:          models.TaskSessionStateFailed,
				StartedAt:      time.Now().UTC(),
			}
			repo.sessionsByTask[req.TaskID] = []*models.TaskSession{session}
			repo.primaryByTask[req.TaskID] = session
			cancel()
			return nil, context.Canceled
		},
	}

	h := &TaskHandlers{service: svc, orchestrator: orch, logger: log}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/workspaces/ws-1/quick-chat", strings.NewReader(`{
		"agent_profile_id":"profile-1"
	}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "ws-1"}}

	c.Request = c.Request.WithContext(ctx)
	h.httpStartQuickChat(c)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "failed to start session", resp["error"])
	assert.NotEmpty(t, resp["task_id"])
	assert.Equal(t, "sess-retained-1", resp["session_id"])
	assert.Empty(t, repo.deletedTaskID, "ephemeral task should not be deleted on post-allocation failure")

	// Ensure the retained session is restorable in ListQuickChatSessions.
	items, err := svc.ListQuickChatSessions(context.Background(), "ws-1")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "sess-retained-1", items[0].SessionID)
	assert.Equal(t, repo.task.ID, items[0].TaskID)
}

func TestQuickChatFailureRetention_LookupUnavailableDoesNotProveAbsence(t *testing.T) {
	log := newTestLogger(t)
	repo := &quickChatRetentionRepo{lookupErr: errors.New("storage unavailable")}
	svc := service.NewService(service.Repos{Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo, Sessions: repo, GitSnapshots: repo,
		RepoEntities: repo, Executors: repo, Environments: repo, TaskEnvironments: repo, Reviews: repo}, nil, log, service.RepositoryDiscoveryConfig{})
	orch := &retentionOrchestrator{launchFn: func(context.Context, *orchestrator.LaunchSessionRequest) (*orchestrator.LaunchSessionResponse, error) {
		return nil, errors.New("launch failed")
	}}
	h := &TaskHandlers{service: svc, orchestrator: orch, logger: log}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/workspaces/ws-1/quick-chat", strings.NewReader(`{"agent_profile_id":"profile-1"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "ws-1"}}
	h.httpStartQuickChat(c)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Empty(t, repo.deletedTaskID)
	require.NotContains(t, rec.Body.String(), "storage unavailable")
}
