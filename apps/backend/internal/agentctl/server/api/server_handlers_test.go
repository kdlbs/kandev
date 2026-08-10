package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
)

func serverGet(t *testing.T, srv *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func serverPost(t *testing.T, srv *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}")))
	return rec
}

// TestHandleHealth_ReportsOKWithTimestamp pins the liveness probe. It is the
// one route exempted from bearer auth, so an executor's readiness check depends
// on both the fixed status string and a parseable RFC3339 timestamp.
func TestHandleHealth_ReportsOKWithTimestamp(t *testing.T) {
	srv := newTestServer(t)

	rec := serverGet(t, srv, "/health")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	var body HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status = %q, want %q", body.Status, "ok")
	}
	if _, err := time.Parse(time.RFC3339, body.Timestamp); err != nil {
		t.Errorf("timestamp %q is not RFC3339: %v", body.Timestamp, err)
	}
}

// TestHandleInfo_EchoesInstanceConfig asserts the endpoint reports this
// instance's own configuration. The backend uses it to confirm it is talking to
// the agentctl it just launched, so a hardcoded or stale value is a real bug.
func TestHandleInfo_EchoesInstanceConfig(t *testing.T) {
	log := newTestLogger()
	workDir := t.TempDir()
	cfg := &config.InstanceConfig{Port: 45321, WorkDir: workDir, AgentCommand: "claude-agent-acp --stdio"}
	srv := NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)

	rec := serverGet(t, srv, "/api/v1/info")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	var body InfoResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode info: %v", err)
	}
	if body.WorkDir != workDir {
		t.Errorf("work_dir = %q, want %q", body.WorkDir, workDir)
	}
	if body.Port != 45321 {
		t.Errorf("port = %d, want %d", body.Port, 45321)
	}
	if body.AgentCommand != "claude-agent-acp --stdio" {
		t.Errorf("agent_command = %q, want the configured command", body.AgentCommand)
	}
	if body.Version == "" {
		t.Errorf("version is empty")
	}
}

// TestHandleStatus_ReportsIdleAgent covers the status endpoint before anything
// has been started: the agent status has to be a real value from the manager
// and the process info map must be present rather than null.
func TestHandleStatus_ReportsIdleAgent(t *testing.T) {
	srv := newTestServer(t)

	rec := serverGet(t, srv, "/api/v1/status")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	var body StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if body.AgentStatus != string(srv.procMgr.Status()) {
		t.Errorf("agent_status = %q, want the manager's %q", body.AgentStatus, srv.procMgr.Status())
	}
	if body.ProcessInfo == nil {
		t.Errorf("process_info is null; the field must always be an object")
	}
}

// TestHandleStart_WithoutConfiguredCommandIs500 covers the failure branch of
// the start endpoint. Nothing has called /agent/configure, so the manager has
// no command to launch and the reason must reach the caller.
func TestHandleStart_WithoutConfiguredCommandIs500(t *testing.T) {
	srv := newTestServer(t)

	rec := serverPost(t, srv, "/api/v1/start")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (body %s)", rec.Code, rec.Body.String())
	}
	var body StartResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode start response: %v", err)
	}
	if body.Success {
		t.Errorf("success = true, want false")
	}
	if body.Error == "" {
		t.Errorf("error is empty; the launch failure must reach the caller")
	}
}

// TestHandleStop_WhenNothingRunning asserts stopping an idle instance is a
// success, not an error — the backend calls it unconditionally during teardown.
func TestHandleStop_WhenNothingRunning(t *testing.T) {
	srv := newTestServer(t)

	rec := serverPost(t, srv, "/api/v1/stop")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var body StopResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode stop response: %v", err)
	}
	if !body.Success || body.Message != "agent stopped" {
		t.Errorf("stop response = %+v, want an unqualified success", body)
	}
}

// TestShellEndpoints_WithoutShell pins the two different no-shell answers.
// Status reports availability inside a 200 so the UI can render a disabled
// terminal pane, while the buffer read is a 503 because there is nothing to
// return at all.
func TestShellEndpoints_WithoutShell(t *testing.T) {
	srv := newTestServer(t)

	status := serverGet(t, srv, "/api/v1/shell/status")
	if status.Code != http.StatusOK {
		t.Fatalf("shell status = %d, want 200 (body %s)", status.Code, status.Body.String())
	}
	var statusBody ShellStatusResponse
	if err := json.Unmarshal(status.Body.Bytes(), &statusBody); err != nil {
		t.Fatalf("decode shell status: %v", err)
	}
	if statusBody.Available {
		t.Errorf("available = true with no shell configured")
	}
	if statusBody.Error != "shell not available" {
		t.Errorf("error = %q, want %q", statusBody.Error, "shell not available")
	}

	buffer := serverGet(t, srv, "/api/v1/shell/buffer")
	if buffer.Code != http.StatusServiceUnavailable {
		t.Fatalf("shell buffer = %d, want 503 (body %s)", buffer.Code, buffer.Body.String())
	}
	var bufferBody map[string]string
	if err := json.Unmarshal(buffer.Body.Bytes(), &bufferBody); err != nil {
		t.Fatalf("decode shell buffer: %v", err)
	}
	if bufferBody["error"] != "shell not available" {
		t.Errorf("error = %q, want %q", bufferBody["error"], "shell not available")
	}
}

// TestShellEndpoints_AfterExplicitStart covers the passthrough-mode path: the
// shell is started without the agent, and both read endpoints then describe a
// live PTY. Asserting the pid and shell path is what distinguishes a genuinely
// spawned process from a status struct that merely says Running.
func TestShellEndpoints_AfterExplicitStart(t *testing.T) {
	log := newTestLogger()
	workDir := t.TempDir()
	cfg := &config.InstanceConfig{WorkDir: workDir, ShellEnabled: true}
	procMgr := process.NewManager(cfg, log)
	srv := NewServer(cfg, procMgr, nil, nil, log)
	t.Cleanup(func() {
		if shell := procMgr.Shell(); shell != nil {
			if err := shell.Stop(); err != nil {
				t.Errorf("stop shell: %v", err)
			}
		}
	})

	start := serverPost(t, srv, "/api/v1/shell/start")
	if start.Code != http.StatusOK {
		t.Fatalf("shell start = %d (body %s)", start.Code, start.Body.String())
	}
	var startBody map[string]string
	if err := json.Unmarshal(start.Body.Bytes(), &startBody); err != nil {
		t.Fatalf("decode shell start: %v", err)
	}
	if startBody["status"] != "shell started" {
		t.Errorf("status = %q, want %q", startBody["status"], "shell started")
	}

	status := serverGet(t, srv, "/api/v1/shell/status")
	if status.Code != http.StatusOK {
		t.Fatalf("shell status = %d (body %s)", status.Code, status.Body.String())
	}
	var statusBody ShellStatusResponse
	if err := json.Unmarshal(status.Body.Bytes(), &statusBody); err != nil {
		t.Fatalf("decode shell status: %v", err)
	}
	if !statusBody.Available || !statusBody.Running {
		t.Fatalf("shell status = %+v, want an available, running shell", statusBody)
	}
	if statusBody.Pid <= 0 {
		t.Errorf("pid = %d, want a real process id", statusBody.Pid)
	}
	if statusBody.Shell == "" {
		t.Errorf("shell is empty; the status must name the spawned program")
	}
	if statusBody.Cwd != workDir {
		t.Errorf("cwd = %q, want the workspace root %q", statusBody.Cwd, workDir)
	}

	buffer := serverGet(t, srv, "/api/v1/shell/buffer")
	if buffer.Code != http.StatusOK {
		t.Fatalf("shell buffer = %d, want 200 (body %s)", buffer.Code, buffer.Body.String())
	}
	var bufferBody ShellBufferResponse
	if err := json.Unmarshal(buffer.Body.Bytes(), &bufferBody); err != nil {
		t.Fatalf("decode shell buffer: %v", err)
	}
}

// TestPprofRoutes_GatedOnEnv pins the debug surface behind its environment
// switch. pprof exposes goroutine stacks and heap contents, so registering it
// unconditionally would leak them from every shipped executor.
func TestPprofRoutes_GatedOnEnv(t *testing.T) {
	t.Run("disabled by default", func(t *testing.T) {
		t.Setenv("KANDEV_DEBUG_PPROF_ENABLED", "")
		srv := newTestServer(t)

		for _, path := range []string{"/debug/pprof/", "/debug/pprof/goroutine", "/api/v1/debug/memory"} {
			if rec := serverGet(t, srv, path); rec.Code != http.StatusNotFound {
				t.Errorf("%s status = %d, want 404 while pprof is disabled", path, rec.Code)
			}
		}
	})

	t.Run("enabled by env", func(t *testing.T) {
		t.Setenv("KANDEV_DEBUG_PPROF_ENABLED", "true")
		srv := newTestServer(t)

		for _, path := range []string{"/debug/pprof/", "/debug/pprof/goroutine?debug=1", "/debug/pprof/cmdline"} {
			if rec := serverGet(t, srv, path); rec.Code != http.StatusOK {
				t.Errorf("%s status = %d, want 200 while pprof is enabled", path, rec.Code)
			}
		}

		rec := serverGet(t, srv, "/api/v1/debug/memory")
		if rec.Code != http.StatusOK {
			t.Fatalf("memory status = %d (body %s)", rec.Code, rec.Body.String())
		}
		var memory map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &memory); err != nil {
			t.Fatalf("decode memory stats: %v", err)
		}
		for _, key := range []string{"heap_alloc_mb", "heap_objects", "goroutines", "num_gc", "sys_mb"} {
			if _, ok := memory[key]; !ok {
				t.Errorf("memory stats are missing %q: %v", key, memory)
			}
		}
	})
}
