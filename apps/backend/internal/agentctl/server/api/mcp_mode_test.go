package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
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

func TestHandleSetMcpMode_AcceptsSupportedModes(t *testing.T) {
	s := newTestServerWithMCP(t)

	for _, mode := range []string{mcpserver.ModeTask, mcpserver.ModeConfig, mcpserver.ModeOffice} {
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

func TestHandleSetMcpMode_RejectsUnsupportedMode(t *testing.T) {
	s := newTestServerWithMCP(t)

	rec := setMcpMode(t, s, mcpserver.ModeExternal)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}
