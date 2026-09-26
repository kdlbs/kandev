package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/mcpmode"
	"github.com/kandev/kandev/internal/mcp/plugintools"
	mcpserver "github.com/kandev/kandev/internal/mcp/server"
)

func newTestServerWithMCP(t *testing.T) *Server {
	t.Helper()
	log := newTestLogger()
	cfg := &config.InstanceConfig{
		Port:    0,
		WorkDir: "/tmp/test",
	}
	procMgr := process.NewManager(cfg, log)
	backend := mcpserver.NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	mcpServer := mcpserver.New(backend, "test-session", "test-task", 0, log, "", false, mcpserver.ModeTask)
	return NewServer(cfg, procMgr, mcpServer, nil, log)
}

func setMcpMode(t *testing.T, s *Server, mode string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{"mode": mode})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/mcp/mode", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.router.ServeHTTP(rec, req)
	return rec
}

func setMcpProviders(t *testing.T, s *Server, providers []string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string][]string{"mcp_providers": providers})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/mcp/providers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.router.ServeHTTP(rec, req)
	return rec
}

func TestHandleSetMcpMode_AcceptsSupportedModes(t *testing.T) {
	s := newTestServerWithMCP(t)

	for _, mode := range mcpmode.InstanceModes() {
		t.Run(mode, func(t *testing.T) {
			rec := setMcpMode(t, s, mode)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
			}

			var body struct {
				Mode string `json:"mode"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}
			if body.Mode != mode {
				t.Fatalf("mode = %q, want %q", body.Mode, mode)
			}
		})
	}
}

// orchestratorEmittableModes is every non-default value the backend's
// orchestrator/executor resolveTaskSessionMCPMode can put on this route. The
// values come from the shared wire contract. The producer half of the pin is
// executor.TestMcpModeConstants_MatchTheAgentctlWireValues.
//
// handleSetMcpMode and that resolver are two allowlists over one field.
// v0.92.1 taught the resolver "automation" and left this one alone; the
// existing-workspace launch path calls SetMcpMode before starting the agent,
// so every automation-origin task failed with HTTP 400 at launch.
var orchestratorEmittableModes = []string{
	mcpmode.Config, mcpmode.Office, mcpmode.TaskTitlePending, mcpmode.Automation,
}

func TestHandleSetMcpMode_AcceptsEveryModeTheOrchestratorCanEmit(t *testing.T) {
	s := newTestServerWithMCP(t)

	for _, mode := range orchestratorEmittableModes {
		t.Run(mode, func(t *testing.T) {
			if rec := setMcpMode(t, s, mode); rec.Code != http.StatusOK {
				t.Fatalf("mode %q: status = %d, want %d; body=%s", mode, rec.Code, http.StatusOK, rec.Body.String())
			}
		})
	}
}

func TestHandleSetMcpMode_RejectsUnsupportedMode(t *testing.T) {
	s := newTestServerWithMCP(t)

	rec := setMcpMode(t, s, mcpserver.ModeExternal)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleSetMcpProviders_NormalizesAndPreservesMode(t *testing.T) {
	log := newTestLogger()
	cfg := &config.InstanceConfig{Port: 0, WorkDir: "/tmp/test"}
	procMgr := process.NewManager(cfg, log)
	backend := mcpserver.NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	mcpServer := mcpserver.New(backend, "test-session", "test-task", 0, log, "", false, mcpserver.ModeTaskTitlePending, []string{"github"})
	s := NewServer(cfg, procMgr, mcpServer, nil, log)

	rec := setMcpProviders(t, s, []string{" GITLAB ", "unsupported", "gitlab"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var body struct {
		Providers []string `json:"mcp_providers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got, want := body.Providers, []string{"gitlab"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("providers = %v, want %v", got, want)
	}
}

func TestHandleSetPluginToolsReplacesLiveCatalog(t *testing.T) {
	s := newTestServerWithMCP(t)
	definition := plugintools.Definition{
		PluginID: "echo", LocalName: "echo", ExposedName: plugintools.ExposedName("echo", "echo"),
		Description: "Echo", Surfaces: []string{plugintools.SurfaceKanban},
		InputSchema: []byte(`{"type":"object"}`),
	}
	body, err := json.Marshal(plugintools.Snapshot{Generation: "g", Revision: 1, Tools: []plugintools.Definition{definition}})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/mcp/plugin-tools", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Generation string `json:"generation"`
		Revision   uint64 `json:"revision"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Generation != "g" || response.Revision != 1 {
		t.Fatalf("response = %#v", response)
	}
}
