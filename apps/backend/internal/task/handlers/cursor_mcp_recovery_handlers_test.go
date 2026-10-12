package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/service"
	terminalmodels "github.com/kandev/kandev/internal/terminal/models"
	terminalservice "github.com/kandev/kandev/internal/terminal/service"
)

type fakeCursorMCPRecoveryManager struct {
	mu          sync.Mutex
	serverIDs   []string
	command     string
	authSpec    runtime.CursorMCPAuthenticationSpec
	authErr     error
	retryResult runtime.CursorMCPRetryResult
	retryErr    error
	authCalls   int
	retryCalls  int
}

func (m *fakeCursorMCPRecoveryManager) CursorMCPAuthenticationSpec(
	_ context.Context,
	_, serverID string,
) (runtime.CursorMCPAuthenticationSpec, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.authCalls++
	m.serverIDs = append(m.serverIDs, serverID)
	m.command = m.authSpec.InitialCommand
	return m.authSpec, m.authErr
}

func (m *fakeCursorMCPRecoveryManager) RetryCursorMCPConnection(
	_ context.Context,
	_, serverID string,
) (runtime.CursorMCPRetryResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.retryCalls++
	m.serverIDs = append(m.serverIDs, serverID)
	return m.retryResult, m.retryErr
}

type fakeCursorMCPRecoveryTerminals struct {
	mu        sync.Mutex
	items     []terminalservice.ListItem
	created   int
	resumed   []string
	renamed   []string
	destroyed []string
	commands  []string
	pending   map[string]bool
	live      map[string]bool
}

func (s *fakeCursorMCPRecoveryTerminals) CreateWithOneShotInitialCommand(
	_ context.Context,
	taskID, envID, initialCommand string,
) (*terminalmodels.Terminal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.created++
	id := "shell-created-" + strconv.Itoa(s.created)
	s.commands = append(s.commands, initialCommand)
	s.items = append(s.items, terminalservice.ListItem{
		ID: id, Kind: terminalservice.KindOrdinary, EnvironmentID: envID, DisplayName: "Terminal 1",
		State: "open", InitialCommand: initialCommand,
	})
	return &terminalmodels.Terminal{ID: id, TaskID: taskID, EnvironmentID: envID, InitialCommand: initialCommand}, nil
}

func (s *fakeCursorMCPRecoveryTerminals) List(
	_ context.Context,
	_ string,
	_ bool,
) ([]terminalservice.ListItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := append([]terminalservice.ListItem(nil), s.items...)
	for index := range items {
		if s.live[items[index].ID] {
			items[index].PTYStatus = terminalservice.PTYStatusRunning
		}
	}
	return items, nil
}

func (s *fakeCursorMCPRecoveryTerminals) MarkCursorMCPAuthenticationAttemptPending(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending == nil {
		s.pending = make(map[string]bool)
	}
	s.pending[id] = true
}

func (s *fakeCursorMCPRecoveryTerminals) CursorMCPAuthenticationAttemptPending(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pending[id]
}

func (s *fakeCursorMCPRecoveryTerminals) ClaimShellLaunchInfo(context.Context, string, string, string) (terminalservice.ShellLaunchInfo, error) {
	return terminalservice.ShellLaunchInfo{}, nil
}

func (s *fakeCursorMCPRecoveryTerminals) MarkCursorMCPAuthenticationAttemptFailed(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pending, id)
}

func (s *fakeCursorMCPRecoveryTerminals) Rename(_ context.Context, _, id string, name *string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.renamed = append(s.renamed, id)
	for i := range s.items {
		if s.items[i].ID == id && name != nil {
			s.items[i].DisplayName = *name
		}
	}
	return nil
}

func (s *fakeCursorMCPRecoveryTerminals) Resume(_ context.Context, _, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resumed = append(s.resumed, id)
	for i := range s.items {
		if s.items[i].ID == id {
			s.items[i].State = "open"
		}
	}
	return nil
}

func (s *fakeCursorMCPRecoveryTerminals) Destroy(_ context.Context, _, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.destroyed = append(s.destroyed, id)
	return nil
}

func newCursorMCPRecoveryRouter(
	t *testing.T,
	manager *fakeCursorMCPRecoveryManager,
	terminals *fakeCursorMCPRecoveryTerminals,
) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log := newTestLogger(t)
	svc := service.NewService(service.Repos{}, nil, log, service.RepositoryDiscoveryConfig{})
	if manager.authSpec.TaskID == "" {
		manager.authSpec = runtime.CursorMCPAuthenticationSpec{
			TaskID: "task-1", TaskEnvironmentID: "env-1", WorkspacePath: "/private/workspace",
			InitialCommand: "cursor-agent mcp login server-exact", Label: "Sign in to Issue Tracker", ServerID: "server-exact",
		}
	}
	if manager.retryResult.ProviderID == "" {
		manager.retryResult = runtime.CursorMCPRetryResult{
			ProviderID: "cursor", ServerID: "server-exact", Status: "ready", ToolCount: 3,
		}
	}
	router := gin.New()
	handlers := RegisterProcessRoutes(router, svc, nil, log)
	handlers.cursorMCPRecovery = manager
	handlers.terminalSvc = terminals
	return router
}

func performMCPRecoveryRequest(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	return rec
}

func TestCursorMCPAuthenticateReusesSignInTerminalAndKeepsCommandPrivate(t *testing.T) {
	manager := &fakeCursorMCPRecoveryManager{}
	terminals := &fakeCursorMCPRecoveryTerminals{}
	router := newCursorMCPRecoveryRouter(t, manager, terminals)

	first := performMCPRecoveryRequest(router, http.MethodPost, "/api/v1/task-sessions/session-1/mcp/authenticate", `{"server_id":"server-exact"}`)
	second := performMCPRecoveryRequest(router, http.MethodPost, "/api/v1/task-sessions/session-1/mcp/authenticate", `{"server_id":"server-exact"}`)
	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("authenticate statuses = %d, %d; bodies %s / %s", first.Code, second.Code, first.Body.String(), second.Body.String())
	}
	var firstBody map[string]any
	if err := json.Unmarshal(first.Body.Bytes(), &firstBody); err != nil {
		t.Fatal(err)
	}
	if firstBody["terminal_id"] != "shell-created-1" || firstBody["task_environment_id"] != "env-1" || firstBody["label"] != "Sign in to Issue Tracker" || firstBody["reused"] != false {
		t.Fatalf("unexpected auth response: %#v", firstBody)
	}
	if strings.Contains(first.Body.String(), "cursor-agent") || strings.Contains(first.Body.String(), "/private/workspace") {
		t.Fatalf("response leaked trusted command or workspace path: %s", first.Body.String())
	}
	if terminals.created != 1 || len(terminals.renamed) != 1 || manager.authCalls != 2 {
		t.Fatalf("created=%d renamed=%v manager calls=%d", terminals.created, terminals.renamed, manager.authCalls)
	}
	if !firstBodyBool(t, second.Body.Bytes(), "reused") {
		t.Fatalf("second response did not mark existing sign-in terminal reused: %s", second.Body.String())
	}
	if len(terminals.commands) != 1 || terminals.commands[0] != "cursor-agent mcp login server-exact" {
		t.Fatalf("terminal commands = %#v", terminals.commands)
	}
}

func firstBodyBool(t *testing.T, body []byte, key string) bool {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	result, ok := value[key].(bool)
	if !ok {
		t.Fatalf("%s is not boolean in %s", key, body)
	}
	return result
}

func TestCursorMCPAuthenticateResumesParkedOrdinaryTerminal(t *testing.T) {
	manager := &fakeCursorMCPRecoveryManager{}
	terminals := &fakeCursorMCPRecoveryTerminals{pending: map[string]bool{"shell-existing": true}, items: []terminalservice.ListItem{{
		ID: "shell-existing", Kind: terminalservice.KindOrdinary, EnvironmentID: "env-1",
		DisplayName: "Sign in to Issue Tracker", InitialCommand: "cursor-agent mcp login server-exact", State: "parked",
	}}}
	router := newCursorMCPRecoveryRouter(t, manager, terminals)
	rec := performMCPRecoveryRequest(router, http.MethodPost, "/api/v1/task-sessions/session-1/mcp/authenticate", `{"server_id":"server-exact"}`)
	if rec.Code != http.StatusOK || !firstBodyBool(t, rec.Body.Bytes(), "reused") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if terminals.created != 0 || len(terminals.resumed) != 1 || terminals.resumed[0] != "shell-existing" {
		t.Fatalf("created=%d resumed=%v", terminals.created, terminals.resumed)
	}
}

func TestCursorMCPAuthenticateStartsNewTerminalAfterPriorLoginExits(t *testing.T) {
	manager := &fakeCursorMCPRecoveryManager{}
	terminals := &fakeCursorMCPRecoveryTerminals{}
	router := newCursorMCPRecoveryRouter(t, manager, terminals)
	path := "/api/v1/task-sessions/session-1/mcp/authenticate"
	first := performMCPRecoveryRequest(router, http.MethodPost, path, `{"server_id":"server-exact"}`)
	if first.Code != http.StatusOK {
		t.Fatalf("first status=%d body=%s", first.Code, first.Body.String())
	}
	terminals.mu.Lock()
	terminals.pending["shell-created-1"] = false
	terminals.mu.Unlock()
	second := performMCPRecoveryRequest(router, http.MethodPost, path, `{"server_id":"server-exact"}`)
	if second.Code != http.StatusOK || terminals.created != 2 || len(terminals.renamed) != 2 {
		t.Fatalf("second status=%d created=%d renamed=%v body=%s", second.Code, terminals.created, terminals.renamed, second.Body.String())
	}
	var response cursorMCPAuthenticationResponse
	if err := json.Unmarshal(second.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.TerminalID != "shell-created-2" || response.Reused {
		t.Fatalf("second response = %+v", response)
	}
}

func TestCursorMCPAuthenticateReusesLiveTerminalAfterWebSocketDisconnect(t *testing.T) {
	manager := &fakeCursorMCPRecoveryManager{}
	terminals := &fakeCursorMCPRecoveryTerminals{
		live: map[string]bool{"shell-existing": true},
		items: []terminalservice.ListItem{{
			ID: "shell-existing", Kind: terminalservice.KindOrdinary, EnvironmentID: "env-1",
			DisplayName: "Sign in to Issue Tracker", InitialCommand: "cursor-agent mcp login server-exact", State: "open",
		}},
	}
	router := newCursorMCPRecoveryRouter(t, manager, terminals)
	rec := performMCPRecoveryRequest(router, http.MethodPost, "/api/v1/task-sessions/session-1/mcp/authenticate", `{"server_id":"server-exact"}`)
	if rec.Code != http.StatusOK || terminals.created != 0 || !firstBodyBool(t, rec.Body.Bytes(), "reused") {
		t.Fatalf("status=%d created=%d body=%s", rec.Code, terminals.created, rec.Body.String())
	}
}

func TestCursorMCPRecoveryValidatesExactServerIDAndMapsTypedErrors(t *testing.T) {
	for _, tc := range []struct {
		name       string
		serverID   string
		managerErr error
		wantStatus int
		wantCode   string
	}{
		{name: "empty", serverID: "", wantStatus: http.StatusBadRequest, wantCode: "invalid_server_id"},
		{name: "oversized", serverID: strings.Repeat("x", 513), wantStatus: http.StatusBadRequest, wantCode: "invalid_server_id"},
		{name: "busy", serverID: "Case-Sensitive", managerErr: runtime.ErrCursorMCPRecoverySessionBusy, wantStatus: http.StatusConflict, wantCode: "session_busy"},
		{name: "unavailable", serverID: "Case-Sensitive", managerErr: runtime.ErrCursorMCPRecoveryUnavailable, wantStatus: http.StatusConflict, wantCode: "server_unavailable"},
		{name: "unsupported shell", serverID: "Case-Sensitive", managerErr: runtime.ErrCursorMCPAuthenticationUnsupported, wantStatus: http.StatusNotImplemented, wantCode: "unsupported_shell"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager := &fakeCursorMCPRecoveryManager{retryErr: tc.managerErr}
			terminals := &fakeCursorMCPRecoveryTerminals{}
			router := newCursorMCPRecoveryRouter(t, manager, terminals)
			payload, _ := json.Marshal(map[string]string{"server_id": tc.serverID})
			rec := performMCPRecoveryRequest(router, http.MethodPost, "/api/v1/task-sessions/session-1/mcp/retry", string(payload))
			if rec.Code != tc.wantStatus || !strings.Contains(rec.Body.String(), tc.wantCode) {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if tc.wantStatus == http.StatusBadRequest && manager.retryCalls != 0 {
				t.Fatalf("invalid server ID reached manager: calls=%d", manager.retryCalls)
			}
			if tc.managerErr != nil && strings.Contains(rec.Body.String(), tc.managerErr.Error()) {
				t.Fatalf("response leaked manager error: %s", rec.Body.String())
			}
			if tc.managerErr != nil {
				var body map[string]interface{}
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatalf("decode recovery error: %v", err)
				}
				if body["error_code"] != body["error"] {
					t.Fatalf("recovery error envelope = %#v, want matching error and error_code", body)
				}
			}
			if tc.serverID != "" && len(tc.serverID) <= 512 && len(manager.serverIDs) != 1 {
				t.Fatalf("manager server IDs = %#v", manager.serverIDs)
			}
		})
	}
}

func TestCursorMCPRetryReturnsRetainedDiagnostic(t *testing.T) {
	diagnostic := &mcpconfig.NativeMCPDiagnostic{Operation: "enable", Stage: "wait", Kind: "wait_failed", Message: "native helper failed"}
	result := runtime.CursorMCPRetryResult{
		ProviderID: "cursor", ServerID: "server-exact", Status: "connection_failed",
		ReasonCode: "connection_failed", Diagnostic: diagnostic,
	}
	manager := &fakeCursorMCPRecoveryManager{retryResult: result}
	router := newCursorMCPRecoveryRouter(t, manager, &fakeCursorMCPRecoveryTerminals{})
	rec := performMCPRecoveryRequest(router, http.MethodPost, "/api/v1/task-sessions/session-1/mcp/retry", `{"server_id":"server-exact"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"mcp_diagnostic"`) {
		t.Fatalf("status=%d body=%s, want retained diagnostic", rec.Code, rec.Body.String())
	}
}

func TestCursorMCPRetryReturnsOnlySanitizedReadiness(t *testing.T) {
	manager := &fakeCursorMCPRecoveryManager{retryResult: runtime.CursorMCPRetryResult{
		ProviderID: "cursor", ServerID: "server-exact", Status: "ready", ToolCount: 4,
	}}
	router := newCursorMCPRecoveryRouter(t, manager, &fakeCursorMCPRecoveryTerminals{})
	rec := performMCPRecoveryRequest(router, http.MethodPost, "/api/v1/task-sessions/session-1/mcp/retry", `{"server_id":"server-exact"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	for _, forbidden := range []string{"stdout", "stderr", "command", "workspace"} {
		if strings.Contains(rec.Body.String(), forbidden) {
			t.Fatalf("readiness response contains %q: %s", forbidden, rec.Body.String())
		}
	}
	if !strings.Contains(rec.Body.String(), `"tool_count":4`) || !strings.Contains(rec.Body.String(), `"status":"ready"`) {
		t.Fatalf("unexpected readiness response: %s", rec.Body.String())
	}
}

func TestCursorMCPRetryAcceptsUnsupportedSameSessionReload(t *testing.T) {
	manager := &fakeCursorMCPRecoveryManager{retryResult: runtime.CursorMCPRetryResult{
		ProviderID: "cursor", ServerID: "server-exact", Status: "unavailable",
		ReasonCode: "session_reload_unsupported",
	}}
	router := newCursorMCPRecoveryRouter(t, manager, &fakeCursorMCPRecoveryTerminals{})
	rec := performMCPRecoveryRequest(router, http.MethodPost, "/api/v1/task-sessions/session-1/mcp/retry", `{"server_id":"server-exact"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"reason_code":"session_reload_unsupported"`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCursorMCPRecoveryDeniesForeignSessionBeforeManagerCall(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := &fakeCursorMCPRecoveryManager{}
	log := newTestLogger(t)
	repo := &foreignSessionRepo{}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	router := gin.New()
	h := RegisterProcessRoutes(router, svc, nil, log)
	h.cursorMCPRecovery = manager
	for _, path := range []string{
		"/api/v1/task-sessions/sess-b/mcp/authenticate",
		"/api/v1/task-sessions/sess-b/mcp/retry",
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"server_id":"server-exact"}`))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(authn.WithIdentity(req.Context(), authn.Identity{UserID: "user-a", Role: authn.RoleMember}))
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s status=%d body=%s", path, rec.Code, rec.Body.String())
		}
	}
	if manager.authCalls != 0 || manager.retryCalls != 0 {
		t.Fatalf("foreign request reached manager: auth=%d retry=%d", manager.authCalls, manager.retryCalls)
	}
}

func TestCursorMCPRecoveryUnavailableWhenDependenciesAreMissing(t *testing.T) {
	log := newTestLogger(t)
	router := gin.New()
	serviceForTest := service.NewService(service.Repos{}, nil, log, service.RepositoryDiscoveryConfig{})
	h := RegisterProcessRoutes(router, serviceForTest, nil, log)
	h.cursorMCPRecovery = nil
	rec := performMCPRecoveryRequest(router, http.MethodPost, "/api/v1/task-sessions/session-1/mcp/retry", `{"server_id":"server-exact"}`)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "mcp_recovery_unavailable") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
