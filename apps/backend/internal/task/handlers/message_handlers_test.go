package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type shellOutputMessageRepo struct {
	mockRepository
	messages map[string]*models.Message
	errors   map[string]error
}

func (r *shellOutputMessageRepo) GetMessage(_ context.Context, id string) (*models.Message, error) {
	if err := r.errors[id]; err != nil {
		return nil, err
	}
	message, ok := r.messages[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return message, nil
}

func TestHTTPShellOutputSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	exitCode := float64(4)
	populated := map[string]any{
		"kind": "shell_exec",
		"shell_exec": map[string]any{
			"command": "make test",
			"output": map[string]any{
				"exit_code": exitCode,
				"stdout":    "test output",
				"stderr":    "test error",
				"truncated": true,
			},
		},
	}
	empty := map[string]any{
		"kind":       "shell_exec",
		"shell_exec": map[string]any{"command": "make test"},
	}
	repo := &shellOutputMessageRepo{messages: map[string]*models.Message{
		"populated": {ID: "populated", TaskSessionID: "session-1", Type: models.MessageTypeToolExecute, UpdatedAt: now, Metadata: map[string]any{"status": "completed", "normalized": populated}},
		"empty":     {ID: "empty", TaskSessionID: "session-1", Type: models.MessageTypeToolExecute, UpdatedAt: now, Metadata: map[string]any{"status": "running", "normalized": empty}},
		"other":     {ID: "other", TaskSessionID: "session-2", Type: models.MessageTypeToolExecute, UpdatedAt: now, Metadata: map[string]any{"status": "completed", "normalized": populated}},
		"non-shell": {ID: "non-shell", TaskSessionID: "session-1", Type: models.MessageTypeToolRead, UpdatedAt: now, Metadata: map[string]any{"status": "completed", "normalized": map[string]any{"kind": "read_file", "read_file": map[string]any{"file_path": "README.md"}}}},
	}, errors: map[string]error{"broken": errors.New("database unavailable")}}
	router := shellOutputTestRouter(t, repo)

	tests := []struct {
		name       string
		path       string
		wantStatus int
		assertBody func(*testing.T, map[string]any)
	}{
		{
			name:       "populated",
			path:       "/api/v1/task-sessions/session-1/messages/populated/shell-output",
			wantStatus: http.StatusOK,
			assertBody: func(t *testing.T, body map[string]any) {
				require.Equal(t, "populated", body["message_id"])
				require.Equal(t, "completed", body["status"])
				require.Equal(t, now.Format(time.RFC3339), body["updated_at"])
				output := body["output"].(map[string]any)
				require.Equal(t, "test output", output["stdout"])
				require.Equal(t, "test error", output["stderr"])
				require.Equal(t, exitCode, output["exit_code"])
				require.Equal(t, true, output["truncated"])
			},
		},
		{
			name:       "empty running shell",
			path:       "/api/v1/task-sessions/session-1/messages/empty/shell-output",
			wantStatus: http.StatusOK,
			assertBody: func(t *testing.T, body map[string]any) {
				require.Equal(t, "running", body["status"])
				require.Empty(t, body["output"].(map[string]any))
			},
		},
		{name: "missing", path: "/api/v1/task-sessions/session-1/messages/missing/shell-output", wantStatus: http.StatusNotFound},
		{name: "cross session", path: "/api/v1/task-sessions/session-1/messages/other/shell-output", wantStatus: http.StatusNotFound},
		{name: "non shell", path: "/api/v1/task-sessions/session-1/messages/non-shell/shell-output", wantStatus: http.StatusNotFound},
		{name: "repository failure", path: "/api/v1/task-sessions/session-1/messages/broken/shell-output", wantStatus: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tt.path, nil))
			require.Equal(t, tt.wantStatus, response.Code)
			if tt.assertBody != nil {
				var body map[string]any
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
				tt.assertBody(t, body)
			}
		})
	}
}

func shellOutputTestRouter(t *testing.T, repo *shellOutputMessageRepo) *gin.Engine {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	svc := service.NewService(service.Repos{Messages: repo}, nil, log, service.RepositoryDiscoveryConfig{})
	router := gin.New()
	NewMessageHandlers(svc, nil, log).registerHTTP(router)
	return router
}

// sessionStateSequencer is a mock repository that returns a sequence of session states.
// Each call to GetTaskSession returns the next state in the sequence.
type sessionStateSequencer struct {
	mockRepository
	mu     sync.Mutex
	states []models.TaskSessionState
	errors []string
	call   int
}

func (s *sessionStateSequencer) GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.call
	if idx >= len(s.states) {
		idx = len(s.states) - 1
	}
	s.call++
	errMsg := ""
	if idx < len(s.errors) {
		errMsg = s.errors[idx]
	}
	return &models.TaskSession{
		ID:           id,
		State:        s.states[idx],
		ErrorMessage: errMsg,
	}, nil
}

func newTestMessageHandlers(t *testing.T, repo *sessionStateSequencer) *MessageHandlers {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	require.NoError(t, err)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	return NewMessageHandlers(svc, nil, log)
}

func TestWaitForSessionReady_ImmediatelyReady(t *testing.T) {
	repo := &sessionStateSequencer{
		states: []models.TaskSessionState{models.TaskSessionStateWaitingForInput},
	}
	h := newTestMessageHandlers(t, repo)

	err := h.waitForSessionReady(context.Background(), "session-1")
	assert.NoError(t, err)
}

func TestWaitForSessionReady_TransitionsToReady(t *testing.T) {
	repo := &sessionStateSequencer{
		states: []models.TaskSessionState{
			models.TaskSessionStateStarting,
			models.TaskSessionStateStarting,
			models.TaskSessionStateWaitingForInput,
		},
	}
	h := newTestMessageHandlers(t, repo)

	err := h.waitForSessionReady(context.Background(), "session-1")
	assert.NoError(t, err)
}

func TestWaitForSessionReady_Failed(t *testing.T) {
	repo := &sessionStateSequencer{
		states: []models.TaskSessionState{
			models.TaskSessionStateStarting,
			models.TaskSessionStateFailed,
		},
		errors: []string{"", "agent crashed"},
	}
	h := newTestMessageHandlers(t, repo)

	err := h.waitForSessionReady(context.Background(), "session-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent crashed")
}

func TestWaitForSessionReady_FailedEmptyMessage(t *testing.T) {
	repo := &sessionStateSequencer{
		states: []models.TaskSessionState{models.TaskSessionStateFailed},
	}
	h := newTestMessageHandlers(t, repo)

	err := h.waitForSessionReady(context.Background(), "session-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session failed during resume")
}

func TestWaitForSessionReady_Cancelled(t *testing.T) {
	repo := &sessionStateSequencer{
		states: []models.TaskSessionState{models.TaskSessionStateCancelled},
	}
	h := newTestMessageHandlers(t, repo)

	err := h.waitForSessionReady(context.Background(), "session-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected state")
}

func TestWaitForSessionReady_ContextCancelled(t *testing.T) {
	repo := &sessionStateSequencer{
		states: []models.TaskSessionState{
			models.TaskSessionStateStarting,
			models.TaskSessionStateStarting,
			models.TaskSessionStateStarting,
		},
	}
	h := newTestMessageHandlers(t, repo)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after a brief delay
	go func() {
		time.Sleep(1500 * time.Millisecond)
		cancel()
	}()

	err := h.waitForSessionReady(ctx, "session-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

type messageAddSwitchRepo struct {
	mockRepository
	tasks      map[string]*models.Task
	sessions   map[string]*models.TaskSession
	primaryID  string
	messages   []*models.Message
	turns      []*models.Turn
	getCalls   map[string]int
	failReload bool
}

func (r *messageAddSwitchRepo) GetTask(_ context.Context, id string) (*models.Task, error) {
	if task, ok := r.tasks[id]; ok {
		return task, nil
	}
	return nil, sql.ErrNoRows
}

func (r *messageAddSwitchRepo) GetTaskSession(_ context.Context, id string) (*models.TaskSession, error) {
	if r.getCalls == nil {
		r.getCalls = make(map[string]int)
	}
	r.getCalls[id]++
	if r.failReload && id == "s1" && r.getCalls[id] > 1 {
		return nil, errors.New("reload failed")
	}
	if session, ok := r.sessions[id]; ok {
		return session, nil
	}
	return nil, sql.ErrNoRows
}

func (r *messageAddSwitchRepo) GetPrimarySessionByTaskID(_ context.Context, taskID string) (*models.TaskSession, error) {
	session, ok := r.sessions[r.primaryID]
	if !ok || session.TaskID != taskID {
		return nil, sql.ErrNoRows
	}
	return session, nil
}

func (r *messageAddSwitchRepo) CreateMessage(_ context.Context, message *models.Message) error {
	r.messages = append(r.messages, message)
	return nil
}

func (r *messageAddSwitchRepo) GetActiveTurnBySessionID(_ context.Context, _ string) (*models.Turn, error) {
	return nil, sql.ErrNoRows
}

func (r *messageAddSwitchRepo) CreateTurn(_ context.Context, turn *models.Turn) error {
	r.turns = append(r.turns, turn)
	return nil
}

type switchingTurnStartOrchestrator struct {
	mu               sync.Mutex
	startOnce        sync.Once
	repo             *messageAddSwitchRepo
	forwardedSession string
	startedSession   string
	switchPrimary    bool
	started          chan struct{}
}

func (o *switchingTurnStartOrchestrator) PromptTask(
	_ context.Context,
	_, sessionID, _, _ string,
	_ bool,
	_ []v1.MessageAttachment,
	_ bool,
) (*orchestrator.PromptResult, error) {
	o.mu.Lock()
	o.forwardedSession = sessionID
	o.mu.Unlock()
	return &orchestrator.PromptResult{}, nil
}

func (o *switchingTurnStartOrchestrator) ResumeTaskSession(context.Context, string, string) error {
	return nil
}

func (o *switchingTurnStartOrchestrator) StartCreatedSession(
	_ context.Context,
	_ string,
	sessionID string,
	_ string,
	_ string,
	_ bool,
	_ bool,
	_ bool,
	_ []v1.MessageAttachment,
) error {
	o.mu.Lock()
	o.startedSession = sessionID
	o.mu.Unlock()
	o.startOnce.Do(func() {
		if o.started != nil {
			close(o.started)
		}
	})
	return nil
}

func (o *switchingTurnStartOrchestrator) ProcessOnTurnStart(context.Context, string, string) error {
	o.repo.sessions["s1"].State = models.TaskSessionStateCompleted
	if o.switchPrimary {
		o.repo.primaryID = "s2"
	}
	return nil
}

func (o *switchingTurnStartOrchestrator) StepRequiresCompletionSignal(context.Context, string) bool {
	return false
}

func (o *switchingTurnStartOrchestrator) getStartedSession() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.startedSession
}

func (o *switchingTurnStartOrchestrator) getForwardedSession() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.forwardedSession
}

func TestWSAddMessageUsesSessionSelectedByOnTurnStart(t *testing.T) {
	now := time.Now().UTC()
	repo := &messageAddSwitchRepo{
		tasks: map[string]*models.Task{
			"t1": {ID: "t1", State: v1.TaskStateReview, UpdatedAt: now},
		},
		sessions: map[string]*models.TaskSession{
			"s1": {ID: "s1", TaskID: "t1", State: models.TaskSessionStateWaitingForInput, AgentProfileID: "profile-old", UpdatedAt: now},
			"s2": {ID: "s2", TaskID: "t1", State: models.TaskSessionStateCreated, AgentProfileID: "profile-new", UpdatedAt: now},
		},
		primaryID: "s1",
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	started := make(chan struct{})
	orch := &switchingTurnStartOrchestrator{repo: repo, switchPrimary: true, started: started}
	h := NewMessageHandlers(svc, orch, log)

	req, err := ws.NewRequest("req-1", ws.ActionMessageAdd, map[string]interface{}{
		"task_id":    "t1",
		"session_id": "s1",
		"content":    "continue here",
	})
	require.NoError(t, err)

	resp, err := h.wsAddMessage(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	require.Len(t, repo.messages, 1)
	assert.Equal(t, "s2", repo.messages[0].TaskSessionID)

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("created session was not started")
	}
	assert.Equal(t, "s2", orch.getStartedSession())
	assert.Empty(t, orch.getForwardedSession())
}

func TestWSAddMessageFailsWhenOnTurnStartCompletesSessionWithoutReplacement(t *testing.T) {
	now := time.Now().UTC()
	repo := &messageAddSwitchRepo{
		tasks: map[string]*models.Task{
			"t1": {ID: "t1", State: v1.TaskStateReview, UpdatedAt: now},
		},
		sessions: map[string]*models.TaskSession{
			"s1": {ID: "s1", TaskID: "t1", State: models.TaskSessionStateWaitingForInput, AgentProfileID: "profile-old", UpdatedAt: now},
			"s2": {ID: "s2", TaskID: "t1", State: models.TaskSessionStateCreated, AgentProfileID: "profile-new", UpdatedAt: now},
		},
		primaryID: "s1",
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	orch := &switchingTurnStartOrchestrator{repo: repo}
	h := NewMessageHandlers(svc, orch, log)

	req, err := ws.NewRequest("req-1", ws.ActionMessageAdd, map[string]interface{}{
		"task_id":    "t1",
		"session_id": "s1",
		"content":    "continue here",
	})
	require.NoError(t, err)

	resp, err := h.wsAddMessage(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, resp.Type)
	assert.Empty(t, repo.messages)
	assert.Empty(t, orch.getStartedSession())
	assert.Empty(t, orch.getForwardedSession())
}

func TestWSAddMessageFailsWhenSessionReloadAfterOnTurnStartFails(t *testing.T) {
	now := time.Now().UTC()
	repo := &messageAddSwitchRepo{
		tasks: map[string]*models.Task{
			"t1": {ID: "t1", State: v1.TaskStateReview, UpdatedAt: now},
		},
		sessions: map[string]*models.TaskSession{
			"s1": {ID: "s1", TaskID: "t1", State: models.TaskSessionStateWaitingForInput, AgentProfileID: "profile-old", UpdatedAt: now},
			"s2": {ID: "s2", TaskID: "t1", State: models.TaskSessionStateCreated, AgentProfileID: "profile-new", UpdatedAt: now},
		},
		primaryID:  "s1",
		failReload: true,
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	orch := &switchingTurnStartOrchestrator{repo: repo, switchPrimary: true}
	h := NewMessageHandlers(svc, orch, log)

	req, err := ws.NewRequest("req-1", ws.ActionMessageAdd, map[string]interface{}{
		"task_id":    "t1",
		"session_id": "s1",
		"content":    "continue here",
	})
	require.NoError(t, err)

	resp, err := h.wsAddMessage(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, resp.Type)
	assert.Empty(t, repo.messages)
	assert.Empty(t, orch.getStartedSession())
	assert.Empty(t, orch.getForwardedSession())
}
