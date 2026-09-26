package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
)

// newForeignProcessHandlers wires ProcessHandlers over the shared
// foreign-session fixture, whose sole session "sess-b" belongs to "user-b".
func newForeignProcessHandlers(t *testing.T, mgr *lifecycle.Manager) *ProcessHandlers {
	t.Helper()
	log := newTestLogger(t)
	repo := &foreignSessionRepo{}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	return &ProcessHandlers{service: svc, lifecycleMgr: mgr, logger: log}
}

// processRequestAs builds a request for sess-b carrying userID's identity.
func processRequestAs(t *testing.T, userID, method, target, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request = c.Request.WithContext(authn.WithIdentity(
		c.Request.Context(), authn.Identity{UserID: userID, Role: authn.RoleMember}))
	c.Params = gin.Params{{Key: "id", Value: "sess-b"}, {Key: "processId", Value: "proc-1"}}
	return c, rec
}

// agentctlProcessServer serves a single process owned by procSessionID and
// reports whether the list endpoint was reached.
func agentctlProcessServer(t *testing.T, procSessionID string) (*httptest.Server, *atomic.Bool) {
	t.Helper()
	listed := &atomic.Bool{}
	proc := agentctlclient.ProcessInfo{
		ID:        "proc-1",
		SessionID: procSessionID,
		Kind:      agentctlclient.ProcessKind("dev"),
		Status:    agentctlclient.ProcessStatus("running"),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/processes":
			listed.Store(true)
			data, _ := json.Marshal([]agentctlclient.ProcessInfo{proc})
			_, _ = w.Write(data)
		case strings.HasPrefix(r.URL.Path, "/api/v1/processes/"):
			data, _ := json.Marshal(proc)
			_, _ = w.Write(data)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server, listed
}

// managerWithSessionExecution registers a running execution for sessionID that
// talks to the given agentctl server.
func managerWithSessionExecution(t *testing.T, serverURL, sessionID string) *lifecycle.Manager {
	t.Helper()
	log := newTestLogger(t)
	mgr := newLifecycleManager(t, log)
	exec := (&lifecycle.ExecutorInstance{
		InstanceID: "exec-1",
		TaskID:     "task-b",
		SessionID:  sessionID,
		Client:     newAgentctlClient(t, serverURL, log),
	}).ToAgentExecution(&lifecycle.ExecutorCreateRequest{
		InstanceID:     "exec-1",
		TaskID:         "task-b",
		SessionID:      sessionID,
		AgentProfileID: "profile-1",
		WorkspacePath:  "/tmp/workspace",
	})
	addExecution(t, mgr, exec)
	return mgr
}

// TestHTTPListProcessesDeniesForeignSession closes the read half of the
// session-keyed process routes. The owner request is the witness: it reaches
// agentctl and returns the process, so the 404 above is the scoping denial and
// not some unrelated failure.
func TestHTTPListProcessesDeniesForeignSession(t *testing.T) {
	server, listed := agentctlProcessServer(t, "sess-b")
	h := newForeignProcessHandlers(t, managerWithSessionExecution(t, server.URL, "sess-b"))

	c, rec := processRequestAs(t, "user-a", http.MethodGet, "/api/v1/task-sessions/sess-b/processes", "")
	h.httpListProcesses(c)
	require.Equal(t, http.StatusNotFound, rec.Code, "body: %s", rec.Body.String())
	require.NotContains(t, rec.Body.String(), "proc-1", "a foreign session's processes must not leak")
	require.False(t, listed.Load(), "the denial must land before agentctl is contacted")

	ownerCtx, ownerRec := processRequestAs(t, "user-b", http.MethodGet, "/api/v1/task-sessions/sess-b/processes", "")
	h.httpListProcesses(ownerCtx)
	require.Equal(t, http.StatusOK, ownerRec.Code)
	require.Contains(t, ownerRec.Body.String(), "proc-1")
	require.True(t, listed.Load())
}

// TestHTTPGetProcessDeniesForeignSession is the same shape for the single-process
// read, whose payload can carry captured process output.
func TestHTTPGetProcessDeniesForeignSession(t *testing.T) {
	server, _ := agentctlProcessServer(t, "sess-b")
	h := newForeignProcessHandlers(t, managerWithSessionExecution(t, server.URL, "sess-b"))

	c, rec := processRequestAs(t, "user-a", http.MethodGet,
		"/api/v1/task-sessions/sess-b/processes/proc-1", "")
	h.httpGetProcess(c)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.NotContains(t, rec.Body.String(), "running", "the foreign process must not be serialized")

	ownerCtx, ownerRec := processRequestAs(t, "user-b", http.MethodGet,
		"/api/v1/task-sessions/sess-b/processes/proc-1", "")
	h.httpGetProcess(ownerCtx)
	require.Equal(t, http.StatusOK, ownerRec.Code)
	require.Contains(t, ownerRec.Body.String(), "proc-1")
}

// TestHTTPGetProcessRejectsProcessFromAnotherSession covers the ownership
// re-check after the lookup: agentctl is asked by process ID alone, so the
// handler must compare the returned SessionID against the route's.
func TestHTTPGetProcessRejectsProcessFromAnotherSession(t *testing.T) {
	server, _ := agentctlProcessServer(t, "sess-elsewhere")
	h := newForeignProcessHandlers(t, managerWithSessionExecution(t, server.URL, "sess-b"))

	c, rec := processRequestAs(t, "user-b", http.MethodGet,
		"/api/v1/task-sessions/sess-b/processes/proc-1", "")
	h.httpGetProcess(c)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.JSONEq(t, `{"error":"process not found"}`, rec.Body.String())
}

// TestHTTPStartProcessDeniesForeignSession covers the write half. Starting a
// process runs a shell command inside the session's workspace, so this is the
// highest-value denial in the file. The owner witness fails later, on the
// repository the fixture deliberately does not provide.
func TestHTTPStartProcessDeniesForeignSession(t *testing.T) {
	h := newForeignProcessHandlers(t, newLifecycleManager(t, newTestLogger(t)))

	c, rec := processRequestAs(t, "user-a", http.MethodPost,
		"/api/v1/task-sessions/sess-b/processes/start", `{"kind":"dev"}`)
	h.httpStartProcess(c)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.JSONEq(t, `{"error":"task session not found"}`, rec.Body.String())

	ownerCtx, ownerRec := processRequestAs(t, "user-b", http.MethodPost,
		"/api/v1/task-sessions/sess-b/processes/start", `{"kind":"dev"}`)
	h.httpStartProcess(ownerCtx)
	require.NotEqual(t, http.StatusNotFound, ownerRec.Code,
		"the owner must get past the session check — otherwise the 404 above proves nothing")
	require.Equal(t, http.StatusBadRequest, ownerRec.Code)
	require.JSONEq(t, `{"error":"session has no repository"}`, ownerRec.Body.String())
}

// TestProcessRoutesRejectMissingIdentifiers covers the cheap argument checks
// that run before any lookup.
func TestProcessRoutesRejectMissingIdentifiers(t *testing.T) {
	h := newForeignProcessHandlers(t, nil)

	for name, tc := range map[string]struct {
		handler  func(*gin.Context)
		params   gin.Params
		wantBody string
	}{
		"start without session": {
			handler:  h.httpStartProcess,
			wantBody: `{"error":"session_id is required"}`,
		},
		"list without session": {
			handler:  h.httpListProcesses,
			wantBody: `{"error":"session_id is required"}`,
		},
		"get without session": {
			handler:  h.httpGetProcess,
			params:   gin.Params{{Key: "processId", Value: "proc-1"}},
			wantBody: `{"error":"session_id and process_id are required"}`,
		},
		"get without process": {
			handler:  h.httpGetProcess,
			params:   gin.Params{{Key: "id", Value: "sess-b"}},
			wantBody: `{"error":"session_id and process_id are required"}`,
		},
		"stop without process": {
			handler:  h.httpStopProcessByID,
			params:   gin.Params{{Key: "id", Value: "sess-b"}},
			wantBody: `{"error":"session_id and process_id are required"}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{}`))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Params = tc.params

			tc.handler(c)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.JSONEq(t, tc.wantBody, rec.Body.String())
		})
	}
}

func TestHTTPStartProcessRejectsInvalidBody(t *testing.T) {
	h := newForeignProcessHandlers(t, nil)
	c, rec := processRequestAs(t, "user-b", http.MethodPost,
		"/api/v1/task-sessions/sess-b/processes/start", `{`)

	h.httpStartProcess(c)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.JSONEq(t, `{"error":"invalid request body"}`, rec.Body.String())
}

// TestHTTPListProcessesReportsEmptyWhenAgentNotStarted pins the graceful
// degradation: an absent execution is "no processes yet", not a 500, because
// the UI polls this endpoint while the agent is still launching.
func TestHTTPListProcessesReportsEmptyWhenAgentNotStarted(t *testing.T) {
	h := newForeignProcessHandlers(t, newLifecycleManager(t, newTestLogger(t)))
	c, rec := processRequestAs(t, "user-b", http.MethodGet, "/api/v1/task-sessions/sess-b/processes", "")

	h.httpListProcesses(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `[]`, rec.Body.String())
}

// TestHTTPStopProcessIsIdempotentWithoutAnExecution documents the 204 fast
// path: nothing is running, so there is nothing to stop and no error to report.
// Driven through the real router because a 204 is only flushed to the recorder
// once gin finishes the request.
func TestHTTPStopProcessIsIdempotentWithoutAnExecution(t *testing.T) {
	log := newTestLogger(t)
	repo := &foreignSessionRepo{}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	router := newProcessStopRouter(t, svc, newLifecycleManager(t, log), log)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/task-sessions/sess-b/processes/proc-1/stop", nil)
	req = req.WithContext(authn.WithIdentity(req.Context(),
		authn.Identity{UserID: "user-b", Role: authn.RoleMember}))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Empty(t, rec.Body.String())
}

func TestResolveWorkingDirForStartPrefersTheExecutionWorkspace(t *testing.T) {
	server, _ := agentctlProcessServer(t, "sess-b")
	h := newForeignProcessHandlers(t, managerWithSessionExecution(t, server.URL, "sess-b"))
	c, rec := processRequestAs(t, "user-b", http.MethodPost, "/x", "")

	dir, ok := h.resolveWorkingDirForStart(c, "sess-b", "/fallback")

	require.True(t, ok)
	require.Equal(t, "/tmp/workspace", dir, "a live execution's workspace wins over the legacy fallback")
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestResolveWorkingDirForStartFallsBackAndThenFails(t *testing.T) {
	h := newForeignProcessHandlers(t, newLifecycleManager(t, newTestLogger(t)))

	c, _ := processRequestAs(t, "user-b", http.MethodPost, "/x", "")
	dir, ok := h.resolveWorkingDirForStart(c, "sess-none", "  /repo/path  ")
	require.True(t, ok)
	require.Equal(t, "/repo/path", dir, "the fallback is trimmed")

	emptyCtx, emptyRec := processRequestAs(t, "user-b", http.MethodPost, "/x", "")
	_, ok = h.resolveWorkingDirForStart(emptyCtx, "sess-none", "   ")
	require.False(t, ok)
	require.Equal(t, http.StatusServiceUnavailable, emptyRec.Code)
	require.JSONEq(t, `{"error":"workspace path unavailable for session"}`, emptyRec.Body.String())
}

func TestPrepareCommandWithPortsLeavesPlainCommandsAlone(t *testing.T) {
	h := newForeignProcessHandlers(t, nil)
	c, rec := processRequestAs(t, "user-b", http.MethodPost, "/x", "")

	command, portEnv, ok := h.prepareCommandWithPorts(c, "sess-b", "pnpm dev")

	require.True(t, ok)
	require.Equal(t, "pnpm dev", command)
	require.Empty(t, portEnv)
	require.Equal(t, http.StatusOK, rec.Code, "no response is written on the success path")
}

func TestResolveScriptCommandRejectsUnconfiguredAndUnknownKinds(t *testing.T) {
	svc := newTestService(t, nil)
	empty := &models.Repository{ID: "repo-1"}

	for name, tc := range map[string]struct {
		kind       string
		scriptName string
		wantErr    string
	}{
		"setup not configured":   {kind: "setup", wantErr: "setup script not configured"},
		"cleanup not configured": {kind: "cleanup", wantErr: "cleanup script not configured"},
		"dev not configured":     {kind: "dev", wantErr: "dev script not configured"},
		"custom without name":    {kind: "custom", wantErr: "script_name is required for custom scripts"},
		"unknown kind":           {kind: "deploy", wantErr: "invalid script kind"},
		"empty kind":             {kind: "", wantErr: "invalid script kind"},
	} {
		t.Run(name, func(t *testing.T) {
			command, kind, scriptName, err := resolveScriptCommand(
				context.Background(), svc, empty, tc.kind, tc.scriptName)

			require.EqualError(t, err, tc.wantErr)
			require.Empty(t, command)
			require.Empty(t, kind)
			require.Empty(t, scriptName)
		})
	}
}

// TestResolveScriptCommandIsCaseInsensitiveOnKind pins the strings.ToLower on
// the kind: the UI has sent "Dev" before.
func TestResolveScriptCommandIsCaseInsensitiveOnKind(t *testing.T) {
	svc := newTestService(t, nil)
	repo := &models.Repository{ID: "repo-1", DevScript: "pnpm dev"}

	command, kind, _, err := resolveScriptCommand(context.Background(), svc, repo, "DEV", "")

	require.NoError(t, err)
	require.Equal(t, "pnpm dev", command)
	require.Equal(t, "dev", kind, "the normalized kind is reported back, not the caller's casing")
}

func TestResolveScriptCommandRejectsMissingAndEmptyCustomScripts(t *testing.T) {
	svc := newTestService(t, map[string][]*models.RepositoryScript{
		"repo-1": {
			{ID: "s1", RepositoryID: "repo-1", Name: "build", Command: "make build"},
			{ID: "s2", RepositoryID: "repo-1", Name: "blank", Command: "   "},
		},
	})
	repo := &models.Repository{ID: "repo-1"}

	_, _, _, err := resolveScriptCommand(context.Background(), svc, repo, "custom", "missing")
	require.EqualError(t, err, "custom script not found")

	_, _, _, err = resolveScriptCommand(context.Background(), svc, repo, "custom", "blank")
	require.EqualError(t, err, "script command is empty")
}
