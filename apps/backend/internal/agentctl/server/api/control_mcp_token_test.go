package api

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/instance"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/agent"
)

// tokenFixture builds a control server over a manager whose instance servers
// are the real instance API server, so the MCP guard and /identity are real.
func tokenFixture(t *testing.T) (*ControlServer, *instance.Manager) {
	t.Helper()
	log := logger.Default()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("probe port: %v", err)
	}
	lo := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	mgr := instance.NewManager(&config.Config{
		Ports:    config.PortConfig{Base: lo, Max: lo + 20},
		Defaults: config.InstanceDefaults{Protocol: agent.ProtocolACP},
	}, log)
	t.Cleanup(func() { _ = mgr.Shutdown(t.Context()) })
	mgr.SetServerFactory(func(cfg *config.InstanceConfig, pm *process.Manager, l *logger.Logger) http.Handler {
		return NewServer(cfg, pm, nil, nil, l).Router()
	})
	return NewControlServer(&config.Config{}, mgr, log), mgr
}

func controlDo(cs *ControlServer, method, path, body string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	cs.Router().ServeHTTP(rec, req)
	return rec
}

func createTokenInstance(t *testing.T, cs *ControlServer, id, token string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"id": id, "workspace_path": t.TempDir(), "session_mcp_token": token})
	return controlDo(cs, http.MethodPost, "/api/v1/instances", string(body))
}

func TestCreateInstanceDuplicateAnswers409AndKeepsExisting(t *testing.T) {
	cs, mgr := tokenFixture(t)
	if rec := createTokenInstance(t, cs, "dup", "tok-1"); rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	t.Cleanup(func() { _ = mgr.StopInstance(t.Context(), "dup") })
	rec := createTokenInstance(t, cs, "dup", "tok-2")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "instance_exists") {
		t.Fatalf("duplicate: %d %s", rec.Code, rec.Body.String())
	}
	inst, ok := mgr.GetInstance("dup")
	if !ok || inst.MCPTokenFingerprint() != config.TokenFingerprint("tok-1") {
		t.Fatal("existing instance changed by refused create")
	}
}

func TestDeleteWithFingerprintPrecondition(t *testing.T) {
	cs, mgr := tokenFixture(t)
	if rec := createTokenInstance(t, cs, "pre", "tok-1"); rec.Code != http.StatusCreated {
		t.Fatalf("create: %d", rec.Code)
	}
	t.Cleanup(func() { _ = mgr.StopInstance(t.Context(), "pre") })

	if rec := controlDo(cs, http.MethodDelete, "/api/v1/instances/pre", "", "X-Expected-Token-Fingerprint", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty precondition: %d", rec.Code)
	}
	if rec := controlDo(cs, http.MethodDelete, "/api/v1/instances/pre", "", "X-Expected-Token-Fingerprint", config.TokenFingerprint("other")); rec.Code != http.StatusOK {
		t.Fatalf("mismatch must answer success: %d", rec.Code)
	}
	if _, ok := mgr.GetInstance("pre"); !ok {
		t.Fatal("mismatching precondition deleted the instance")
	}
	if rec := controlDo(cs, http.MethodDelete, "/api/v1/instances/pre", "", "X-Expected-Token-Fingerprint", config.TokenFingerprint("tok-1")); rec.Code != http.StatusOK {
		t.Fatalf("matching: %d", rec.Code)
	}
	if _, ok := mgr.GetInstance("pre"); ok {
		t.Fatal("matching precondition did not delete")
	}
}

func TestInstanceIdentityAdvertisesFingerprintsAndRevoke(t *testing.T) {
	cs, mgr := tokenFixture(t)
	if rec := createTokenInstance(t, cs, "idn", "tok-1"); rec.Code != http.StatusCreated {
		t.Fatalf("create: %d", rec.Code)
	}
	t.Cleanup(func() { _ = mgr.StopInstance(t.Context(), "idn") })
	inst, _ := mgr.GetInstance("idn")
	base := "http://127.0.0.1:" + strconv.Itoa(inst.Port)

	read := func() map[string]any {
		resp, err := http.Get(base + "/identity")
		if err != nil {
			t.Fatalf("identity: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		var out map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("identity body: %v", err)
		}
		return out
	}
	id := read()
	if id["mcp_token_fingerprint"] != config.TokenFingerprint("tok-1") {
		t.Fatalf("fingerprint = %v", id["mcp_token_fingerprint"])
	}
	if caps, _ := id["capabilities"].([]any); !containsCapability(caps, SessionMCPTokenCapability) {
		t.Fatalf("capabilities = %v", id["capabilities"])
	}
	if id["broker_upstream_fingerprint"] != "" {
		t.Fatalf("broker fingerprint = %v", id["broker_upstream_fingerprint"])
	}

	if rec := controlDo(cs, http.MethodPost, "/api/v1/instances/idn/mcp-token/revoke", ""); rec.Code != http.StatusOK {
		t.Fatalf("revoke: %d", rec.Code)
	}
	if rec := controlDo(cs, http.MethodPost, "/api/v1/instances/idn/mcp-token/revoke", ""); rec.Code != http.StatusOK {
		t.Fatalf("revoke is idempotent: %d", rec.Code)
	}
	if got := read()["mcp_token_fingerprint"]; got != "" {
		t.Fatalf("fingerprint after revoke = %v", got)
	}
	req, _ := http.NewRequest(http.MethodGet, base+"/sse", nil)
	req.Header.Set("Authorization", "Bearer tok-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("sse: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("MCP after revoke: %d", resp.StatusCode)
	}
	if rec := controlDo(cs, http.MethodPost, "/api/v1/instances/nope/mcp-token/revoke", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown instance revoke: %d", rec.Code)
	}
}

func containsCapability(caps []any, want string) bool {
	for _, c := range caps {
		if c == want {
			return true
		}
	}
	return false
}
