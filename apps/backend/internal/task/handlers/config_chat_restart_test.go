package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
)

func TestHTTPRestartConfigChatRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handlers := &TaskHandlers{logger: newTestLogger(t)}
	handlers.registerHTTP(router)

	registered := make(map[string]bool)
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	assert.True(t, registered["POST /api/v1/workspaces/:id/config-chat/restart"], "restart route missing")
}

type restartHTTPRepo struct {
	mockRepository
	mu                                  sync.Mutex
	tasks                               map[string]*models.Task
	profile                             *settingsmodels.AgentProfile
	executor                            *models.Executor
	executorProfile                     *models.ExecutorProfile
	extraSessions                       []*models.TaskSession
	createErr, deleteErr, compatibleErr error
	calls                               []string
	listHook                            func()
}

func (r *restartHTTPRepo) GetWorkspace(_ context.Context, id string) (*models.Workspace, error) {
	if id != "ws-1" {
		return nil, repoerrors.ErrWorkspaceNotFound
	}
	return &models.Workspace{ID: id, OwnerID: "owner"}, nil
}

func (r *restartHTTPRepo) GetTask(_ context.Context, id string) (*models.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task := r.tasks[id]
	if task == nil {
		return nil, repoerrors.ErrTaskNotFound
	}
	copy := *task
	copy.Metadata = maps.Clone(task.Metadata)
	return &copy, nil
}

func (r *restartHTTPRepo) CreateTask(ctx context.Context, task *models.Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, "create")
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.createErr != nil {
		return r.createErr
	}
	r.tasks[task.ID] = task
	return nil
}

func (r *restartHTTPRepo) ListTaskSessions(ctx context.Context, taskID string) ([]*models.TaskSession, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []*models.TaskSession
	for _, session := range r.sessions {
		if session.TaskID == taskID {
			copy := *session
			result = append(result, &copy)
		}
	}
	return append(result, r.extraSessions...), nil
}

func (r *restartHTTPRepo) ListTasksByWorkspace(_ context.Context, _, _, _, _ string, _, _ int, _ string, _, _, _, _ bool) ([]*models.Task, int, error) {
	if r.listHook != nil {
		r.listHook()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var tasks []*models.Task
	for _, task := range r.tasks {
		copy := *task
		tasks = append(tasks, &copy)
	}
	return tasks, len(tasks), nil
}

func (r *restartHTTPRepo) GetPrimarySessionInfoByTaskIDs(_ context.Context, ids []string) (map[string]*models.TaskSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make(map[string]*models.TaskSession)
	for _, id := range ids {
		for _, session := range r.sessions {
			if session.TaskID == id && session.IsPrimary {
				copy := *session
				result[id] = &copy
			}
		}
	}
	return result, nil
}

func (r *restartHTTPRepo) GetAgentProfile(context.Context, string) (*settingsmodels.AgentProfile, error) {
	return r.profile, nil
}

func (r *restartHTTPRepo) GetExecutor(context.Context, string) (*models.Executor, error) {
	return r.executor, nil
}

func (r *restartHTTPRepo) GetExecutorProfile(context.Context, string) (*models.ExecutorProfile, error) {
	return r.executorProfile, nil
}

func (r *restartHTTPRepo) ValidateAgentProfileForExecutor(context.Context, *settingsmodels.AgentProfile, *models.Executor, *models.ExecutorProfile) error {
	return r.compatibleErr
}

func (r *restartHTTPRepo) DeleteTaskTree(ctx context.Context, id string, cascade bool) (*service.CascadeOutcome, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, "delete")
	if r.deleteErr != nil {
		return nil, r.deleteErr
	}
	delete(r.tasks, id)
	for key, session := range r.sessions {
		if session.TaskID == id {
			delete(r.sessions, key)
		}
	}
	return &service.CascadeOutcome{}, nil
}

type restartHTTPOrchestrator struct {
	repo                          *restartHTTPRepo
	stopErr, prepareErr, startErr error
	stopEntered, stopRelease      chan struct{}
	requests                      []*orchestrator.LaunchSessionRequest
	identity                      authn.Identity
	preparePartial                bool
	prepareCancel                 context.CancelFunc
}

func (o *restartHTTPOrchestrator) EnsureSession(context.Context, string, ...orchestrator.EnsureSessionOptions) (*orchestrator.EnsureSessionResponse, error) {
	return nil, nil
}

func (o *restartHTTPOrchestrator) RetireConfigChatSession(ctx context.Context, _, _ string, deleteFn func(context.Context) error) error {
	o.repo.mu.Lock()
	o.repo.calls = append(o.repo.calls, "stop")
	o.repo.mu.Unlock()
	o.identity, _ = authn.IdentityFromContext(ctx)
	if o.stopEntered != nil {
		close(o.stopEntered)
		<-o.stopRelease
	}
	if o.stopErr != nil {
		return o.stopErr
	}
	return deleteFn(ctx)
}

func (o *restartHTTPOrchestrator) LaunchSession(ctx context.Context, req *orchestrator.LaunchSessionRequest) (*orchestrator.LaunchSessionResponse, error) {
	o.requests = append(o.requests, req)
	if req.Intent == orchestrator.IntentPrepare {
		if o.prepareCancel != nil {
			o.prepareCancel()
		}
		if o.prepareErr != nil && !o.preparePartial {
			return nil, o.prepareErr
		}
		o.repo.mu.Lock()
		o.repo.sessions["new-session"] = &models.TaskSession{ID: "new-session", TaskID: req.TaskID, IsPrimary: true, AgentProfileID: req.AgentProfileID}
		o.repo.mu.Unlock()
		if o.prepareErr != nil {
			return nil, o.prepareErr
		}
		return &orchestrator.LaunchSessionResponse{TaskID: req.TaskID, SessionID: "new-session", AgentProfileID: req.AgentProfileID}, nil
	}
	return &orchestrator.LaunchSessionResponse{TaskID: req.TaskID, SessionID: req.SessionID}, o.startErr
}

func newRestartHTTPFixture(t *testing.T) (*gin.Engine, *service.Service, *restartHTTPRepo, *restartHTTPOrchestrator) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	now := time.Now().UTC()
	repo := &restartHTTPRepo{
		tasks:           map[string]*models.Task{"old-task": {ID: "old-task", WorkspaceID: "ws-1", Title: "Old conversation", IsEphemeral: true, UpdatedAt: now, Metadata: map[string]interface{}{"config_mode": true}}},
		profile:         &settingsmodels.AgentProfile{ID: "profile-1", Enabled: true},
		executor:        &models.Executor{ID: "exec-1", Status: models.ExecutorStatusActive},
		executorProfile: &models.ExecutorProfile{ID: "exec-profile", ExecutorID: "exec-1"},
	}
	repo.sessions = map[string]*models.TaskSession{"old-session": {ID: "old-session", TaskID: "old-task", IsPrimary: true, AgentProfileID: "profile-1", ExecutorID: "exec-1", ExecutorProfileID: "exec-profile", State: models.TaskSessionStateRunning}}
	log := newTestLogger(t)
	svc := service.NewService(service.Repos{Workspaces: repo, Tasks: repo, TaskRepos: repo, Workflows: repo, Messages: repo, Turns: repo, Sessions: repo, GitSnapshots: repo, RepoEntities: repo, Executors: repo, Environments: repo, TaskEnvironments: repo, Reviews: repo, AgentProfiles: repo, AgentProfileExecutorValidator: repo}, nil, log, service.RepositoryDiscoveryConfig{})
	svc.SetWorktreeCleanup(authzDeleteCleanup{})
	svc.SetTaskLifecycleCoordinator(repo)
	o := &restartHTTPOrchestrator{repo: repo}
	router := gin.New()
	NewTaskHandlers(svc, o, nil, nil, log).registerHTTP(router)
	return router, svc, repo, o
}

func restartHTTPTicket(t *testing.T, svc *service.Service) string {
	t.Helper()
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "http-test-user", Synthetic: true})
	preview, err := svc.TaskDeletePreflight(ctx, []string{"old-task"}, false, false)
	require.NoError(t, err)
	return preview.ConfirmationID
}

func restartHTTPRequest(router *gin.Engine, ticket string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/ws-1/config-chat/restart", strings.NewReader(`{"task_id":"old-task","session_id":"old-session"}`))
	request.Header.Set(taskDeleteConfirmationHeader, ticket)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, withTaskDeleteTestIdentity(request))
	return recorder
}

func TestHTTPRestartConfigChatReplacesAndStartsBlank(t *testing.T) {
	router, svc, repo, o := newRestartHTTPFixture(t)
	response := restartHTTPRequest(router, restartHTTPTicket(t, svc))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var result map[string]string
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	assert.NotEqual(t, "old-task", result["task_id"])
	assert.Equal(t, "new-session", result["session_id"])
	assert.Equal(t, []string{"stop", "delete", "create"}, repo.calls)
	require.Len(t, o.requests, 2)
	assert.True(t, o.requests[0].DeferredStart)
	assert.Equal(t, "exec-profile", o.requests[0].ExecutorProfileID)
	assert.Equal(t, "exec-1", o.requests[0].ExecutorID)
	assert.Equal(t, "profile-1", o.requests[1].AgentProfileID)
	assert.Equal(t, orchestrator.IntentStartCreated, o.requests[1].Intent)
	assert.Equal(t, orchestrator.LaunchActivationSourceUserAction, o.requests[1].ActivationSource)
	assert.Empty(t, o.requests[1].Prompt)
	assert.True(t, o.requests[1].NoInitialPrompt)
	assert.True(t, o.requests[1].SkipMessageRecord)
	assert.Equal(t, "http-test-user", o.identity.UserID)
}

func TestHTTPRestartConfigChatRequiresCurrentConfirmation(t *testing.T) {
	router, _, repo, _ := newRestartHTTPFixture(t)
	response := restartHTTPRequest(router, "")
	assert.Equal(t, http.StatusPreconditionRequired, response.Code, response.Body.String())
	assert.Empty(t, repo.calls)
}

func TestHTTPRestartConfigChatPreservesImplicitLocalExecutor(t *testing.T) {
	router, svc, repo, o := newRestartHTTPFixture(t)
	repo.sessions["old-session"].ExecutorID = ""
	repo.sessions["old-session"].ExecutorProfileID = ""
	repo.executor = &models.Executor{ID: models.ExecutorIDLocal, Type: models.ExecutorTypeLocal, Status: models.ExecutorStatusActive}
	response := restartHTTPRequest(router, restartHTTPTicket(t, svc))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Len(t, o.requests, 2)
	assert.Equal(t, models.ExecutorIDLocal, o.requests[0].ExecutorID)
	assert.Empty(t, o.requests[0].ExecutorProfileID)
}

func TestHTTPRestartConfigChatValidatesBeforeStopping(t *testing.T) {
	cases := map[string]func(*restartHTTPRepo){
		"regular task":             func(r *restartHTTPRepo) { r.tasks["old-task"].IsEphemeral = false },
		"ordinary chat":            func(r *restartHTTPRepo) { r.tasks["old-task"].Metadata["config_mode"] = false },
		"workflow":                 func(r *restartHTTPRepo) { r.tasks["old-task"].WorkflowID = "workflow" },
		"archived":                 func(r *restartHTTPRepo) { now := time.Now(); r.tasks["old-task"].ArchivedAt = &now },
		"wrong workspace":          func(r *restartHTTPRepo) { r.tasks["old-task"].WorkspaceID = "another" },
		"wrong pair":               func(r *restartHTTPRepo) { r.sessions["old-session"].TaskID = "another" },
		"not primary":              func(r *restartHTTPRepo) { r.sessions["old-session"].IsPrimary = false },
		"additional session":       func(r *restartHTTPRepo) { r.extraSessions = []*models.TaskSession{{ID: "extra", TaskID: "old-task"}} },
		"disabled agent":           func(r *restartHTTPRepo) { r.profile.Enabled = false },
		"foreign agent":            func(r *restartHTTPRepo) { r.profile.WorkspaceID = "another" },
		"disabled executor":        func(r *restartHTTPRepo) { r.executor.Status = models.ExecutorStatusDisabled },
		"missing executor profile": func(r *restartHTTPRepo) { r.executorProfile = nil },
		"incompatible":             func(r *restartHTTPRepo) { r.compatibleErr = errors.New("unsupported") },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			router, svc, repo, _ := newRestartHTTPFixture(t)
			ticket := restartHTTPTicket(t, svc)
			mutate(repo)
			response := restartHTTPRequest(router, ticket)
			assert.GreaterOrEqual(t, response.Code, 400, response.Body.String())
			assert.Empty(t, repo.calls)
		})
	}
}

func TestHTTPRestartConfigChatPartialFailures(t *testing.T) {
	for _, stage := range []string{"stop", "delete", "create", "prepare", "start"} {
		t.Run(stage, func(t *testing.T) {
			router, svc, repo, o := newRestartHTTPFixture(t)
			switch stage {
			case "stop":
				o.stopErr = errors.New(stage)
			case "delete":
				repo.deleteErr = errors.New(stage)
			case "create":
				repo.createErr = errors.New(stage)
			case "prepare":
				o.prepareErr = errors.New(stage)
			case "start":
				o.startErr = errors.New(stage)
			}
			response := restartHTTPRequest(router, restartHTTPTicket(t, svc))
			require.GreaterOrEqual(t, response.Code, 500, response.Body.String())
			var body struct {
				Stage       string `json:"stage"`
				OldDeleted  bool   `json:"old_deleted"`
				Replacement *struct {
					TaskID    string `json:"task_id"`
					SessionID string `json:"session_id"`
				} `json:"replacement"`
			}
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
			wantStage := stage
			if stage == "prepare" {
				wantStage = "create"
			}
			assert.Equal(t, wantStage, body.Stage)
			assert.Equal(t, stage != "stop" && stage != "delete", body.OldDeleted)
			if stage == "start" {
				require.NotNil(t, body.Replacement)
				assert.Equal(t, "new-session", body.Replacement.SessionID)
				assert.NotNil(t, repo.tasks[body.Replacement.TaskID])
			}
			if stage == "prepare" {
				assert.Empty(t, repo.tasks, "failed preparation must use canonical rollback")
			}
		})
	}
}

func TestHTTPRestartConfigChatKeepsPartiallyPreparedReplacementReachable(t *testing.T) {
	router, svc, repo, o := newRestartHTTPFixture(t)
	o.prepareErr = errors.New("workspace preparation failed after session creation")
	o.preparePartial = true
	response := restartHTTPRequest(router, restartHTTPTicket(t, svc))
	require.Equal(t, http.StatusInternalServerError, response.Code, response.Body.String())
	var failure configChatRestartFailure
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &failure))
	assert.True(t, failure.OldDeleted)
	assert.Equal(t, configChatRestartStageCreate, failure.Stage)
	require.NotNil(t, failure.Replacement)
	assert.Equal(t, "new-session", failure.Replacement.SessionID)
	assert.NotNil(t, repo.tasks[failure.Replacement.TaskID])
	assert.Equal(t, []string{"stop", "delete", "create"}, repo.calls)
	require.Len(t, o.requests, 1)
}

func TestHTTPRestartConfigChatAdmissionAndReconciliation(t *testing.T) {
	router, svc, repo, o := newRestartHTTPFixture(t)
	o.stopEntered = make(chan struct{})
	o.stopRelease = make(chan struct{})
	ticket := restartHTTPTicket(t, svc)
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- restartHTTPRequest(router, ticket) }()
	<-o.stopEntered
	duplicate := restartHTTPRequest(router, ticket)
	assert.Equal(t, http.StatusConflict, duplicate.Code, duplicate.Body.String())
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/ws-1/quick-chats", nil))
	assert.Contains(t, listRec.Body.String(), `"config_chat_restart_pending":true`)
	assert.Contains(t, listRec.Body.String(), `"config_chat_retiring_session_id":"old-session"`)
	startRec := httptest.NewRecorder()
	router.ServeHTTP(startRec, httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/ws-1/config-chat", strings.NewReader(`{"agent_profile_id":"profile-1"}`)))
	assert.Equal(t, http.StatusConflict, startRec.Code)
	close(o.stopRelease)
	require.Equal(t, http.StatusOK, (<-done).Code)
	assert.Equal(t, []string{"stop", "delete", "create"}, repo.calls)
}

func TestHTTPRestartConfigChatStalePreview(t *testing.T) {
	router, svc, repo, _ := newRestartHTTPFixture(t)
	ticket := restartHTTPTicket(t, svc)
	repo.tasks["old-task"].UpdatedAt = repo.tasks["old-task"].UpdatedAt.Add(time.Second)
	response := restartHTTPRequest(router, ticket)
	assert.Equal(t, http.StatusConflict, response.Code, response.Body.String())
	assert.Empty(t, repo.calls)
}

func TestHTTPRestartConfigChatPreservesIdentityAfterDisconnect(t *testing.T) {
	router, svc, _, o := newRestartHTTPFixture(t)
	ticket := restartHTTPTicket(t, svc)
	ctx, cancel := context.WithCancel(authn.WithIdentity(context.Background(), authn.Identity{UserID: "http-test-user", Synthetic: true}))
	cancel()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/ws-1/config-chat/restart", strings.NewReader(`{"task_id":"old-task","session_id":"old-session"}`)).WithContext(ctx)
	request.Header.Set(taskDeleteConfirmationHeader, ticket)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, "http-test-user", o.identity.UserID)
}

func TestHTTPRestartConfigChatForeignCallerHasNoEffects(t *testing.T) {
	router, svc, repo, _ := newRestartHTTPFixture(t)
	ticket := restartHTTPTicket(t, svc)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/ws-1/config-chat/restart", strings.NewReader(`{"task_id":"old-task","session_id":"old-session"}`))
	request = request.WithContext(authn.WithIdentity(request.Context(), authn.Identity{UserID: "foreign", Role: authn.RoleMember}))
	request.Header.Set(taskDeleteConfirmationHeader, ticket)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	assert.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
	assert.Empty(t, repo.calls)
}

func TestHTTPRestartConfigChatListAcrossTransition(t *testing.T) {
	router, svc, repo, _ := newRestartHTTPFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	repo.listHook = func() { once.Do(func() { close(entered); <-release }) }
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/ws-1/quick-chats", nil))
		done <- response
	}()
	<-entered
	replacement := restartHTTPRequest(router, restartHTTPTicket(t, svc))
	close(release)
	require.Equal(t, http.StatusOK, replacement.Code, replacement.Body.String())
	response := <-done
	assert.Contains(t, response.Body.String(), `"session_id":"new-session"`)
	assert.Contains(t, response.Body.String(), `"config_chat_restart_pending":false`)
	assert.NotContains(t, response.Body.String(), "old-session")
}
