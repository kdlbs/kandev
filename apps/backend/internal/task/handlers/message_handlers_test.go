package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
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

type registrationRetryTaskRepository struct {
	mockRepository
	task *models.Task
	err  error
}

func (r *registrationRetryTaskRepository) GetTask(context.Context, string) (*models.Task, error) {
	return r.task, r.err
}

type registrationRetryNotifier struct {
	taskID         string
	registrationID string
	operationID    string
	actorID        string
	action         string
	err            error
	calls          int
}

func (*registrationRetryNotifier) NotifyAcceptedMessage(string, string) {}

func (n *registrationRetryNotifier) ActTaskChangeRegistration(
	_ context.Context, taskID, registrationID, operationID, actorID, action string,
) error {
	n.calls++
	n.taskID, n.registrationID, n.operationID, n.actorID, n.action = taskID, registrationID, operationID, actorID, action
	return n.err
}

func TestHTTPRetryTaskChangeRegistration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name        string
		task        *models.Task
		taskErr     error
		notifier    bool
		notifierErr error
		wantStatus  int
		wantCalls   int
	}{
		{name: "accepted", task: &models.Task{ID: "task-1"}, notifier: true, wantStatus: http.StatusAccepted, wantCalls: 1},
		{name: "missing task", taskErr: taskrepo.ErrTaskNotFound, notifier: true, wantStatus: http.StatusNotFound},
		{name: "missing registration", task: &models.Task{ID: "task-1"}, notifier: true, notifierErr: sql.ErrNoRows, wantStatus: http.StatusNotFound, wantCalls: 1},
		{name: "registration conflict", task: &models.Task{ID: "task-1"}, notifier: true, notifierErr: taskrepo.ErrTaskChangeRegistrationNotRetryable, wantStatus: http.StatusConflict, wantCalls: 1},
		{name: "registration store failure", task: &models.Task{ID: "task-1"}, notifier: true, notifierErr: errors.New("database unavailable"), wantStatus: http.StatusInternalServerError, wantCalls: 1},
		{name: "worker unavailable", task: &models.Task{ID: "task-1"}, wantStatus: http.StatusServiceUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
			require.NoError(t, err)
			repo := &registrationRetryTaskRepository{task: test.task, err: test.taskErr}
			svc := service.NewService(service.Repos{Tasks: repo, TaskRepos: repo}, nil, log, service.RepositoryDiscoveryConfig{})
			handlers := NewMessageHandlers(svc, nil, log)
			var notifier *registrationRetryNotifier
			if test.notifier {
				notifier = &registrationRetryNotifier{err: test.notifierErr}
				handlers.SetTaskChangeRegistrationNotifier(notifier)
			}
			router := gin.New()
			handlers.registerHTTP(router)
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost,
				"/api/v1/tasks/task-1/change-request-registrations/selection-1/retry",
				strings.NewReader(`{"action":"retry","operation_id":"action-1"}`))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(response, request)
			require.Equal(t, test.wantStatus, response.Code)
			if notifier != nil {
				require.Equal(t, test.wantCalls, notifier.calls)
				if test.wantCalls > 0 {
					require.Equal(t, "task-1", notifier.taskID)
					require.Equal(t, "selection-1", notifier.registrationID)
					require.Equal(t, "action-1", notifier.operationID)
					require.NotEmpty(t, notifier.actorID)
					require.Equal(t, "retry", notifier.action)
				}
			}
			if test.wantStatus == http.StatusAccepted {
				var body map[string]any
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
				require.Equal(t, true, body["accepted"])
				require.Equal(t, "pending", body["state"])
			}
		})
	}
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

func (s *sessionStateSequencer) ClaimPromptableTaskSessionIfActive(_ context.Context, id string) (models.PromptableTaskSessionClaim, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.call
	if idx >= len(s.states) {
		idx = len(s.states) - 1
	}
	if idx < 0 || !isPromptableTaskSessionState(s.states[idx]) {
		return models.PromptableTaskSessionClaim{Status: models.PromptableTaskSessionBusy}, nil
	}
	previousState := s.states[idx]
	s.states[idx] = models.TaskSessionStateRunning
	return models.PromptableTaskSessionClaim{Status: models.PromptableTaskSessionClaimed, PreviousState: previousState}, nil
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
