package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/gorilla/websocket"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type failingSessionMetadataRepo struct {
	*mockRepository
	err error
}

func (r *failingSessionMetadataRepo) SetSessionMetadataKey(
	context.Context,
	string,
	string,
	interface{},
) error {
	return r.err
}

func TestSessionConfigurationPersistenceFailureDoesNotReportSaved(t *testing.T) {
	log := processConfigurationTestLogger(t)
	session := &models.TaskSession{ID: "session-config-failure", TaskID: "task-1", State: models.TaskSessionStateIdle}
	repo := &mockRepository{sessions: map[string]*models.TaskSession{session.ID: session}}
	svc := processConfigurationService(repo, &failingSessionMetadataRepo{
		mockRepository: repo,
		err:            errors.New("database write failed"),
	}, log)
	handler := &ProcessHandlers{service: svc, lifecycleMgr: newLifecycleManager(t, log), logger: log}
	router := newProcessStopRouter(t, svc, handler.lifecycleMgr, log)
	request := httptest.NewRequest(http.MethodPost,
		"/api/v1/task-sessions/session-config-failure/set-model",
		strings.NewReader(`{"model_id":"model-next"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", recorder.Code, recorder.Body.String())
	}
	var response sessionConfigurationHTTPResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.ConfigurationResult.Status != "persistence_failed" {
		t.Fatalf("status = %q, want persistence_failed", response.ConfigurationResult.Status)
	}
	if response.ConfigurationResult.Confirmed || response.ConfigurationResult.Effective != "" {
		t.Fatalf("deferred save failure claims a live value: %+v", response.ConfigurationResult)
	}
	if !strings.Contains(response.Error, "persist session runtime config") {
		t.Fatalf("error = %q, want save failure", response.Error)
	}
}

func TestLiveSessionConfigurationPersistenceFailurePreservesAppliedValue(t *testing.T) {
	client := startConfigurationAgentctlStream(t, func(request ws.Message, connection *websocket.Conn) {
		response, err := ws.NewResponse(request.ID, request.Action, map[string]any{
			"requested": "model-requested", "effective": "model-effective", "confirmed": true,
		})
		if err != nil {
			t.Errorf("create setter response: %v", err)
			return
		}
		if err := connection.WriteJSON(response); err != nil {
			t.Errorf("write setter response: %v", err)
		}
	})

	log := processConfigurationTestLogger(t)
	session := &models.TaskSession{ID: "session-live-config-failure", TaskID: "task-1", State: models.TaskSessionStateIdle}
	repo := &mockRepository{sessions: map[string]*models.TaskSession{session.ID: session}}
	svc := processConfigurationService(repo, &failingSessionMetadataRepo{
		mockRepository: repo,
		err:            errors.New("database write failed"),
	}, log)
	mgr := newLifecycleManager(t, log)
	execution := (&lifecycle.ExecutorInstance{InstanceID: "exec-live-config", Client: client}).ToAgentExecution(
		&lifecycle.ExecutorCreateRequest{SessionID: session.ID, TaskID: session.TaskID},
	)
	execution.ACPSessionID = "acp-session"
	setHandlerExecutionInitialized(t, execution)
	addExecution(t, mgr, execution)
	router := newProcessStopRouter(t, svc, mgr, log)
	request := httptest.NewRequest(http.MethodPost,
		"/api/v1/task-sessions/session-live-config-failure/set-model",
		strings.NewReader(`{"model_id":"model-requested"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", recorder.Code, recorder.Body.String())
	}
	var response sessionConfigurationHTTPResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	result := response.ConfigurationResult
	if result.Status != "persistence_failed" || !result.Confirmed || result.Effective != "model-effective" || result.ExecutionID != execution.ID {
		t.Fatalf("configuration_result = %+v, want persistence failure with confirmed live value", result)
	}
	if !strings.Contains(response.Error, "persist session runtime config") {
		t.Fatalf("error = %q, want save failure", response.Error)
	}
}

func TestSessionConfigurationTransportTimeoutIsUnconfirmed(t *testing.T) {
	client := startConfigurationAgentctlStream(t, func(ws.Message, *websocket.Conn) {})
	log := processConfigurationTestLogger(t)
	session := &models.TaskSession{ID: "session-config-timeout", TaskID: "task-1", State: models.TaskSessionStateIdle}
	repo := &mockRepository{sessions: map[string]*models.TaskSession{session.ID: session}}
	svc := processConfigurationService(repo, repo, log)
	mgr := newLifecycleManager(t, log)
	execution := (&lifecycle.ExecutorInstance{InstanceID: "exec-timeout-config", Client: client}).ToAgentExecution(
		&lifecycle.ExecutorCreateRequest{SessionID: session.ID, TaskID: session.TaskID},
	)
	execution.ACPSessionID = "acp-session"
	setHandlerExecutionInitialized(t, execution)
	addExecution(t, mgr, execution)
	router := newProcessStopRouter(t, svc, mgr, log)
	request := httptest.NewRequest(http.MethodPost,
		"/api/v1/task-sessions/session-config-timeout/set-model",
		strings.NewReader(`{"model_id":"model-requested"}`))
	request.Header.Set("Content-Type", "application/json")
	ctx, cancel := context.WithTimeout(request.Context(), 50*time.Millisecond)
	defer cancel()
	request = request.WithContext(ctx)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", recorder.Code, recorder.Body.String())
	}
	var response sessionConfigurationHTTPResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.ConfigurationResult.Status != "unconfirmed" || response.ConfigurationResult.Confirmed {
		t.Fatalf("configuration_result = %+v, want unconfirmed transport outcome", response.ConfigurationResult)
	}
	if !strings.Contains(response.Error, "context deadline exceeded") {
		t.Fatalf("error = %q, want readable timeout", response.Error)
	}
}

func TestSessionConfigurationStreamDisconnectIsUnconfirmed(t *testing.T) {
	client := startConfigurationAgentctlStream(t, func(_ ws.Message, connection *websocket.Conn) {
		_ = connection.Close()
	})
	log := processConfigurationTestLogger(t)
	session := &models.TaskSession{ID: "session-config-disconnect", TaskID: "task-1", State: models.TaskSessionStateIdle}
	repo := &mockRepository{sessions: map[string]*models.TaskSession{session.ID: session}}
	svc := processConfigurationService(repo, repo, log)
	mgr := newLifecycleManager(t, log)
	execution := (&lifecycle.ExecutorInstance{InstanceID: "exec-disconnect-config", Client: client}).ToAgentExecution(
		&lifecycle.ExecutorCreateRequest{SessionID: session.ID, TaskID: session.TaskID},
	)
	execution.ACPSessionID = "acp-session"
	setHandlerExecutionInitialized(t, execution)
	addExecution(t, mgr, execution)
	router := newProcessStopRouter(t, svc, mgr, log)
	request := httptest.NewRequest(http.MethodPost,
		"/api/v1/task-sessions/session-config-disconnect/set-model",
		strings.NewReader(`{"model_id":"model-requested"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	var response sessionConfigurationHTTPResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if recorder.Code != http.StatusInternalServerError || response.ConfigurationResult.Status != "unconfirmed" || response.ConfigurationResult.Confirmed {
		t.Fatalf("status/result = %d/%+v, want 500/unconfirmed", recorder.Code, response.ConfigurationResult)
	}
	if !strings.Contains(response.Error, "agent stream not connected") {
		t.Fatalf("error = %q, want readable disconnect", response.Error)
	}
}

func TestSessionConfigurationExplicitRejectionKeepsReadableReason(t *testing.T) {
	client := startConfigurationAgentctlStream(t, func(request ws.Message, connection *websocket.Conn) {
		response, err := ws.NewError(request.ID, request.Action, ws.ErrorCodeValidation, "model is not available", nil)
		if err != nil {
			t.Errorf("create setter rejection: %v", err)
			return
		}
		if err := connection.WriteJSON(response); err != nil {
			t.Errorf("write setter rejection: %v", err)
		}
	})
	log := processConfigurationTestLogger(t)
	session := &models.TaskSession{ID: "session-config-rejected", TaskID: "task-1", State: models.TaskSessionStateIdle}
	repo := &mockRepository{sessions: map[string]*models.TaskSession{session.ID: session}}
	svc := processConfigurationService(repo, repo, log)
	mgr := newLifecycleManager(t, log)
	execution := (&lifecycle.ExecutorInstance{InstanceID: "exec-rejected-config", Client: client}).ToAgentExecution(
		&lifecycle.ExecutorCreateRequest{SessionID: session.ID, TaskID: session.TaskID},
	)
	execution.ACPSessionID = "acp-session"
	setHandlerExecutionInitialized(t, execution)
	addExecution(t, mgr, execution)
	router := newProcessStopRouter(t, svc, mgr, log)
	request := httptest.NewRequest(http.MethodPost,
		"/api/v1/task-sessions/session-config-rejected/set-model",
		strings.NewReader(`{"model_id":"model-requested"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	var response sessionConfigurationHTTPResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if recorder.Code != http.StatusInternalServerError || response.ConfigurationResult.Status != "rejected" {
		t.Fatalf("status/result = %d/%+v, want 500/rejected", recorder.Code, response.ConfigurationResult)
	}
	if !strings.Contains(response.Error, "model is not available") {
		t.Fatalf("error = %q, want explicit rejection reason", response.Error)
	}
}

func TestSessionConfigurationConfigOptionRejectionIsRejected(t *testing.T) {
	err := errors.New("set config option failed: set session config option failed: requested value is unsupported")
	for _, configID := range []string{"reasoning_effort", "model", "mode"} {
		t.Run(configID, func(t *testing.T) {
			if got := sessionConfigurationSetterErrorStatus("config option", err); got != sessionConfigurationStatusRejected {
				t.Fatalf("status = %q, want rejected for explicit provider response", got)
			}
		})
	}
}

func TestSessionConfigurationRetryRequiresExpectedBaseModel(t *testing.T) {
	setterCalls := make(chan struct{}, 1)
	client := startConfigurationAgentctlStream(t, func(request ws.Message, connection *websocket.Conn) {
		setterCalls <- struct{}{}
		response, err := ws.NewResponse(request.ID, request.Action, map[string]any{
			"requested": "high", "effective": "high", "confirmed": true,
		})
		if err != nil {
			t.Errorf("create setter response: %v", err)
			return
		}
		if err := connection.WriteJSON(response); err != nil {
			t.Errorf("write setter response: %v", err)
		}
	})

	log := processConfigurationTestLogger(t)
	session := &models.TaskSession{ID: "session-config-retry-model", TaskID: "task-1", State: models.TaskSessionStateIdle}
	repo := &mockRepository{sessions: map[string]*models.TaskSession{session.ID: session}}
	svc := processConfigurationService(repo, repo, log)
	mgr := newLifecycleManager(t, log)
	execution := (&lifecycle.ExecutorInstance{InstanceID: "exec-retry-model", Client: client}).ToAgentExecution(
		&lifecycle.ExecutorCreateRequest{SessionID: session.ID, TaskID: session.TaskID},
	)
	execution.ACPSessionID = "acp-session"
	execution.SetModelState(&lifecycle.CachedModelState{CurrentModelID: "model-c"})
	setHandlerExecutionInitialized(t, execution)
	addExecution(t, mgr, execution)
	router := newProcessStopRouter(t, svc, mgr, log)
	request := httptest.NewRequest(http.MethodPost,
		"/api/v1/task-sessions/session-config-retry-model/set-config-option",
		strings.NewReader(`{"config_id":"thinking","value":"high","expected_model_id":"model-b"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", recorder.Code, recorder.Body.String())
	}
	var response sessionConfigurationHTTPResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.ConfigurationResult.Status != "unconfirmed" || response.ConfigurationResult.Confirmed {
		t.Fatalf("configuration_result = %+v, want unconfirmed precondition failure", response.ConfigurationResult)
	}
	select {
	case <-setterCalls:
		t.Fatal("provider option mutation ran after the base model changed")
	default:
	}
}

func TestSessionConfigurationRetryRejectsWhenExpectedModelCannotBeVerified(t *testing.T) {
	log := processConfigurationTestLogger(t)
	session := &models.TaskSession{
		ID: "session-config-retry-stopped", TaskID: "task-1", State: models.TaskSessionStateIdle,
		Metadata: map[string]interface{}{
			models.SessionMetaKeyRuntimeConfigOverrides: models.SessionRuntimeConfig{Model: "model-c"},
		},
	}
	repo := &mockRepository{sessions: map[string]*models.TaskSession{session.ID: session}}
	svc := processConfigurationService(repo, repo, log)
	mgr := newLifecycleManager(t, log)
	router := newProcessStopRouter(t, svc, mgr, log)
	request := httptest.NewRequest(http.MethodPost,
		"/api/v1/task-sessions/session-config-retry-stopped/set-config-option",
		strings.NewReader(`{"config_id":"thinking","value":"high","expected_model_id":"model-b"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", recorder.Code, recorder.Body.String())
	}
	var response sessionConfigurationHTTPResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.ConfigurationResult.Status != "unconfirmed" || response.ConfigurationResult.Confirmed {
		t.Fatalf("configuration_result = %+v, want unconfirmed precondition failure", response.ConfigurationResult)
	}
	if !strings.Contains(response.Error, lifecycle.ErrSessionConfigurationModelChanged.Error()) {
		t.Fatalf("error = %q, want readable inactive-session precondition", response.Error)
	}
	overrides, _ := models.LoadSessionRuntimeConfigOverrides(session.Metadata)
	if overrides.ConfigOptions["thinking"] == "high" {
		t.Fatal("saved option after refusing to verify the expected model")
	}
}

type sessionConfigurationHTTPResponse struct {
	Error               string `json:"error"`
	ConfigurationResult struct {
		Status      string `json:"status"`
		Requested   string `json:"requested"`
		Effective   string `json:"effective"`
		Confirmed   bool   `json:"confirmed"`
		ExecutionID string `json:"execution_id"`
	} `json:"configuration_result"`
}

func processConfigurationTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	return log
}

func processConfigurationService(
	repo *mockRepository,
	sessions repository.SessionRepository,
	log *logger.Logger,
) *service.Service {
	return service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: sessions, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
}

func startConfigurationAgentctlStream(
	t *testing.T,
	handle func(ws.Message, *websocket.Conn),
) *agentctlclient.Client {
	t.Helper()
	connected := make(chan struct{})
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = connection.Close() }()
		close(connected)
		for {
			_, encoded, readErr := connection.ReadMessage()
			if readErr != nil {
				return
			}
			var request ws.Message
			if err := json.Unmarshal(encoded, &request); err != nil {
				t.Errorf("decode agentctl request: %v", err)
				return
			}
			handle(request, connection)
		}
	}))
	t.Cleanup(server.Close)
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse agentctl test URL: %v", err)
	}
	host, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatalf("split agentctl test address: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse agentctl test port: %v", err)
	}
	client := agentctlclient.NewClient(host, port, processConfigurationTestLogger(t))
	streamCtx, cancel := context.WithCancel(context.Background())
	if err := client.StreamUpdates(streamCtx, nil, nil, nil); err != nil {
		cancel()
		t.Fatalf("open agentctl update stream: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		client.Close()
	})
	select {
	case <-connected:
	case <-time.After(2 * time.Second):
		t.Fatal("agentctl update stream did not connect")
	}
	return client
}

func setHandlerExecutionInitialized(t *testing.T, execution *lifecycle.AgentExecution) {
	t.Helper()
	field := reflect.ValueOf(execution).Elem().FieldByName("sessionInitialized")
	if !field.IsValid() {
		t.Fatal("sessionInitialized field not found on lifecycle execution")
	}
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().SetBool(true)
}
