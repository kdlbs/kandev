package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/mcp/plugintools"
)

func TestNewClient_BuildsBaseURLAndTimeouts(t *testing.T) {
	c := NewClient("agentctl.internal", 40123, newTestLogger())

	if c.BaseURL() != "http://agentctl.internal:40123" {
		t.Errorf("BaseURL = %q, want http://agentctl.internal:40123", c.BaseURL())
	}
	if c.Host() != "agentctl.internal" {
		t.Errorf("Host = %q, want agentctl.internal", c.Host())
	}
	if c.httpClient.Timeout != 60*time.Second {
		t.Errorf("httpClient timeout = %v, want 60s", c.httpClient.Timeout)
	}
	// Inference prompts routinely outlast the 60s default, so the long-running
	// client must keep its own 5m budget.
	if c.longRunningHTTPClient.Timeout != 5*time.Minute {
		t.Errorf("longRunningHTTPClient timeout = %v, want 5m", c.longRunningHTTPClient.Timeout)
	}
	if c.AuthToken() != "" {
		t.Errorf("AuthToken = %q, want empty", c.AuthToken())
	}
	if c.authTransport == nil {
		t.Error("mutable auth transport was not installed")
	}
}

func TestNewClient_OptionsInstallAuthAndInstanceHeaders(t *testing.T) {
	var gotAuth, gotInstance string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotInstance = r.Header.Get("X-Instance-ID")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	host, port := splitTestServerHostPort(t, srv)
	c := NewClient(host, port, newTestLogger(),
		WithAuthToken("secret-token"),
		WithExecutionID("exec-1"),
		WithSessionID("sess-1"),
	)

	if c.AuthToken() != "secret-token" {
		t.Errorf("AuthToken = %q, want secret-token", c.AuthToken())
	}
	if c.executionID != "exec-1" || c.sessionID != "sess-1" {
		t.Errorf("execution/session = %q / %q, want exec-1 / sess-1", c.executionID, c.sessionID)
	}

	if err := c.Health(context.Background()); err != nil {
		t.Fatalf("Health: %v", err)
	}
	if gotAuth != "Bearer secret-token" {
		t.Errorf("Authorization = %q, want Bearer secret-token", gotAuth)
	}
	// Both transports are chained; a lost X-Instance-ID lets a recycled port
	// serve a stale client.
	if gotInstance != "exec-1" {
		t.Errorf("X-Instance-ID = %q, want exec-1", gotInstance)
	}
}

func TestClientSetAuthTokenUpdatesHTTPAndWebSocketAuthentication(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	host, port := splitTestServerHostPort(t, srv)
	c := NewClient(host, port, newTestLogger(), WithAuthToken("stale-token"))
	c.SetAuthToken("rotated-token")

	if err := c.Health(context.Background()); err != nil {
		t.Fatalf("Health: %v", err)
	}
	if gotAuth != "Bearer rotated-token" {
		t.Fatalf("Authorization = %q, want rotated token", gotAuth)
	}
	if got := c.wsAuthHeaders().Get("Authorization"); got != "Bearer rotated-token" {
		t.Fatalf("WebSocket Authorization = %q, want rotated token", got)
	}
	if got := c.AuthToken(); got != "rotated-token" {
		t.Fatalf("AuthToken = %q, want rotated token", got)
	}
}

func TestClientSetAuthTokenIsSafeWithConcurrentRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	host, port := splitTestServerHostPort(t, srv)
	c := NewClient(host, port, newTestLogger(), WithAuthToken("initial-token"))

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if i%2 == 0 {
				c.SetAuthToken("even-token")
			} else {
				c.SetAuthToken("odd-token")
			}
			if err := c.Health(context.Background()); err != nil {
				t.Errorf("Health: %v", err)
			}
		}()
	}
	wg.Wait()
	c.SetAuthToken("final-token")
	if got := c.AuthToken(); got != "final-token" {
		t.Fatalf("AuthToken = %q, want final token", got)
	}
}

func TestNewClient_LongRunningClientCarriesTheSameHeaders(t *testing.T) {
	var gotAuth, gotInstance string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotInstance = r.Header.Get("X-Instance-ID")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"a.txt","size_bytes":1}`))
	}))
	t.Cleanup(srv.Close)

	host, port := splitTestServerHostPort(t, srv)
	c := NewClient(host, port, newTestLogger(),
		WithAuthToken("secret-token"), WithExecutionID("exec-1"))

	// MaterializeAttachment is the one call that uses longRunningHTTPClient.
	if _, err := c.MaterializeAttachment(context.Background(),
		"sess-1", "att-1", "a.txt", "text/plain", 1, strings.NewReader("x")); err != nil {
		t.Fatalf("MaterializeAttachment: %v", err)
	}
	if gotAuth != "Bearer secret-token" {
		t.Errorf("Authorization on long-running client = %q, want Bearer secret-token", gotAuth)
	}
	if gotInstance != "exec-1" {
		t.Errorf("X-Instance-ID on long-running client = %q, want exec-1", gotInstance)
	}
}

func TestWsAuthHeaders(t *testing.T) {
	tests := []struct {
		name        string
		token       string
		executionID string
		wantNil     bool
		wantAuth    string
		wantID      string
	}{
		{name: "neither set", wantNil: true},
		{name: "token only", token: "t", wantAuth: "Bearer t"},
		{name: "execution only", executionID: "e", wantID: "e"},
		{name: "both", token: "t", executionID: "e", wantAuth: "Bearer t", wantID: "e"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &Client{authToken: tc.token, executionID: tc.executionID}
			headers := c.wsAuthHeaders()
			if tc.wantNil {
				if headers != nil {
					t.Fatalf("headers = %v, want nil when unauthenticated", headers)
				}
				return
			}
			if headers == nil {
				t.Fatal("headers = nil, want a populated header set")
			}
			if got := headers.Get("Authorization"); got != tc.wantAuth {
				t.Errorf("Authorization = %q, want %q", got, tc.wantAuth)
			}
			if got := headers.Get("X-Instance-ID"); got != tc.wantID {
				t.Errorf("X-Instance-ID = %q, want %q", got, tc.wantID)
			}
		})
	}
}

func TestHost_FallsBackToBaseURLWhenUnparseable(t *testing.T) {
	c := &Client{baseURL: "http://host with spaces:1/\x7f"}
	if got := c.Host(); got != c.baseURL {
		t.Errorf("Host = %q, want the raw baseURL as fallback", got)
	}
}

func TestSetTraceContext_ReplacesTheBackgroundParent(t *testing.T) {
	c := &Client{}
	if got := c.getTraceCtx(); got != context.Background() {
		t.Errorf("getTraceCtx = %v, want context.Background before any set", got)
	}

	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "v")
	c.SetTraceContext(ctx)
	if got := c.getTraceCtx().Value(key{}); got != "v" {
		t.Errorf("trace ctx value = %v, want the context set", got)
	}
}

func TestStatusResponse_IsAgentRunning(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{"running", true},
		{"starting", true},
		{"stopped", false},
		{"error", false},
		{"", false},
	}
	for _, tc := range tests {
		t.Run(tc.status, func(t *testing.T) {
			s := &StatusResponse{AgentStatus: tc.status}
			if got := s.IsAgentRunning(); got != tc.want {
				t.Errorf("IsAgentRunning(%q) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

func TestHealth_HitsHealthEndpoint(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `ok`))

	if err := newHTTPOnlyClient(srv.URL).Health(context.Background()); err != nil {
		t.Fatalf("Health: %v", err)
	}
	if got.Method != http.MethodGet || got.Path != "/health" {
		t.Errorf("request = %s %s, want GET /health", got.Method, got.Path)
	}
}

func TestHealth_FailsOnNonOKStatus(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusServiceUnavailable, ``))

	err := newHTTPOnlyClient(srv.URL).Health(context.Background())
	if err == nil || !strings.Contains(err.Error(), "health check failed: 503") {
		t.Fatalf("error = %v, want status 503 named", err)
	}
}

func TestGetStatus_DecodesAgentStatusAndProcessInfo(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK,
		`{"agent_status":"running","process_info":{"pid":4242,"command":"claude-code acp"}}`))

	status, err := newHTTPOnlyClient(srv.URL).GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	if got.Method != http.MethodGet || got.Path != "/api/v1/status" {
		t.Errorf("request = %s %s, want GET /api/v1/status", got.Method, got.Path)
	}
	if status.AgentStatus != "running" {
		t.Errorf("AgentStatus = %q, want running", status.AgentStatus)
	}
	if !status.IsAgentRunning() {
		t.Error("IsAgentRunning = false for a running agent")
	}
	if pid, ok := status.ProcessInfo["pid"].(float64); !ok || pid != 4242 {
		t.Errorf("ProcessInfo[pid] = %v, want 4242", status.ProcessInfo["pid"])
	}
	if status.ProcessInfo["command"] != "claude-code acp" {
		t.Errorf("ProcessInfo[command] = %v", status.ProcessInfo["command"])
	}
}

func TestGetStatus_FailureModes(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"non-2xx status", http.StatusInternalServerError, `boom`, "status request failed with status 500"},
		{"malformed body", http.StatusOK, `{"agent_status":`, "failed to parse status response"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := captureServer(t, jsonResponder(tc.status, tc.body))

			status, err := newHTTPOnlyClient(srv.URL).GetStatus(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
			if status != nil {
				t.Errorf("status = %+v, want nil on failure", status)
			}
		})
	}
}

func TestConfigureAgent_SendsCommandEnvAndApprovalPolicy(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{"success":true}`))

	err := newHTTPOnlyClient(srv.URL).ConfigureAgent(
		context.Background(), "claude-code acp", nil,
		map[string]string{"ANTHROPIC_API_KEY": "k"}, "never", "", nil)
	if err != nil {
		t.Fatalf("ConfigureAgent: %v", err)
	}

	if got.Method != http.MethodPost || got.Path != "/api/v1/agent/configure" {
		t.Errorf("request = %s %s, want POST /api/v1/agent/configure", got.Method, got.Path)
	}
	if got.ContentType != "application/json" {
		t.Errorf("content-type = %q, want application/json", got.ContentType)
	}

	var sent struct {
		Command        string            `json:"command"`
		Env            map[string]string `json:"env"`
		ApprovalPolicy string            `json:"approval_policy"`
	}
	if err := json.Unmarshal(got.Body, &sent); err != nil {
		t.Fatalf("decode sent body: %v", err)
	}
	if sent.Command != "claude-code acp" {
		t.Errorf("sent command = %q", sent.Command)
	}
	if sent.Env["ANTHROPIC_API_KEY"] != "k" {
		t.Errorf("sent env = %v", sent.Env)
	}
	if sent.ApprovalPolicy != "never" {
		t.Errorf("sent approval_policy = %q, want never", sent.ApprovalPolicy)
	}
}

func TestConfigureAgent_FailureModes(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"non-2xx status", http.StatusBadRequest, `bad`, "configure request failed with status 400"},
		{"malformed body", http.StatusOK, `{"success":`, "failed to parse configure response"},
		{"unsuccessful", http.StatusOK, `{"success":false,"error":"unknown protocol"}`, "configure failed: unknown protocol"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := captureServer(t, jsonResponder(tc.status, tc.body))

			err := newHTTPOnlyClient(srv.URL).ConfigureAgent(
				context.Background(), "cmd", nil, nil, "", "", nil)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestSetMcpMode_PutsModePayload(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{}`))

	if err := newHTTPOnlyClient(srv.URL).SetMcpMode(context.Background(), "office"); err != nil {
		t.Fatalf("SetMcpMode: %v", err)
	}

	if got.Method != http.MethodPut || got.Path != "/api/v1/mcp/mode" {
		t.Errorf("request = %s %s, want PUT /api/v1/mcp/mode", got.Method, got.Path)
	}
	if got.ContentType != "application/json" {
		t.Errorf("content-type = %q, want application/json", got.ContentType)
	}

	var sent struct {
		Mode string `json:"mode"`
	}
	if err := json.Unmarshal(got.Body, &sent); err != nil {
		t.Fatalf("decode sent body: %v", err)
	}
	if sent.Mode != "office" {
		t.Errorf("sent mode = %q, want office", sent.Mode)
	}
}

func TestSetMcpMode_FailsOnNonOKStatusAndEchoesBody(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusBadRequest, `unknown mode "turbo"`))

	err := newHTTPOnlyClient(srv.URL).SetMcpMode(context.Background(), "turbo")
	if err == nil || !strings.Contains(err.Error(), "set MCP mode failed with status 400") {
		t.Fatalf("error = %v, want status 400 named", err)
	}
	if !strings.Contains(err.Error(), `unknown mode "turbo"`) {
		t.Errorf("error = %v, want the server body echoed", err)
	}
}

func TestSetMcpProviders_PutsProviderListUnderMcpProvidersKey(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{}`))

	err := newHTTPOnlyClient(srv.URL).SetMcpProviders(
		context.Background(), []string{"github", "gitlab"})
	if err != nil {
		t.Fatalf("SetMcpProviders: %v", err)
	}

	if got.Method != http.MethodPut || got.Path != "/api/v1/mcp/providers" {
		t.Errorf("request = %s %s, want PUT /api/v1/mcp/providers", got.Method, got.Path)
	}

	var sent struct {
		Providers []string `json:"mcp_providers"`
	}
	if err := json.Unmarshal(got.Body, &sent); err != nil {
		t.Fatalf("decode sent body: %v", err)
	}
	if len(sent.Providers) != 2 || sent.Providers[0] != "github" || sent.Providers[1] != "gitlab" {
		t.Errorf("sent mcp_providers = %v, want [github gitlab]", sent.Providers)
	}
}

func TestSetMcpProviders_FailsOnNonOKStatus(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusInternalServerError, `boom`))

	err := newHTTPOnlyClient(srv.URL).SetMcpProviders(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "set MCP providers failed with status 500") {
		t.Fatalf("error = %v, want status 500 named", err)
	}
}

func TestSetPluginToolsPutsSnapshotUnderControlEndpoint(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{}`))
	snapshot := plugintools.Snapshot{Generation: "g", Revision: 7}
	if err := newHTTPOnlyClient(srv.URL).SetPluginTools(context.Background(), snapshot); err != nil {
		t.Fatalf("SetPluginTools: %v", err)
	}
	if got.Method != http.MethodPut || got.Path != "/api/v1/mcp/plugin-tools" {
		t.Fatalf("request = %s %s", got.Method, got.Path)
	}
	var sent plugintools.Snapshot
	if err := json.Unmarshal(got.Body, &sent); err != nil {
		t.Fatal(err)
	}
	if sent.Generation != "g" || sent.Revision != 7 {
		t.Fatalf("snapshot = %#v", sent)
	}
}

func TestStart_ReturnsTheExecutedCommand(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK,
		`{"success":true,"command":"claude-code acp --workspace-root /w"}`))

	command, err := newHTTPOnlyClient(srv.URL).Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if got.Method != http.MethodPost || got.Path != "/api/v1/start" {
		t.Errorf("request = %s %s, want POST /api/v1/start", got.Method, got.Path)
	}
	if len(got.Body) != 0 {
		t.Errorf("body = %q, want an empty POST", got.Body)
	}
	if command != "claude-code acp --workspace-root /w" {
		t.Errorf("command = %q, want the full executed command", command)
	}
}

func TestStart_FailureModes(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"non-2xx status", http.StatusConflict, `already running`, "start request failed with status 409"},
		{"malformed body", http.StatusOK, `{"success":`, "failed to parse start response"},
		{"unsuccessful", http.StatusOK, `{"success":false,"error":"binary not found"}`, "start failed: binary not found"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := captureServer(t, jsonResponder(tc.status, tc.body))

			command, err := newHTTPOnlyClient(srv.URL).Start(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
			if command != "" {
				t.Errorf("command = %q, want empty on failure", command)
			}
		})
	}
}

func TestStop_PostsToStopEndpoint(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{"success":true}`))

	if err := newHTTPOnlyClient(srv.URL).Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if got.Method != http.MethodPost || got.Path != "/api/v1/stop" {
		t.Errorf("request = %s %s, want POST /api/v1/stop", got.Method, got.Path)
	}
}

func TestStop_FailureModes(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"non-2xx status", http.StatusInternalServerError, `boom`, "stop request failed with status 500"},
		{"malformed body", http.StatusOK, `{"success":`, "failed to parse stop response"},
		{"unsuccessful", http.StatusOK, `{"success":false,"error":"no agent running"}`, "stop failed: no agent running"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := captureServer(t, jsonResponder(tc.status, tc.body))

			err := newHTTPOnlyClient(srv.URL).Stop(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestWaitForReady_ReturnsOnceHealthSucceeds(t *testing.T) {
	// The handler runs on the server's goroutine while the test goroutine reads
	// the counter, so the count is atomic rather than a plain int.
	var probes atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if probes.Add(1) < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	if err := newHTTPOnlyClient(srv.URL).WaitForReady(context.Background(), 5*time.Second); err != nil {
		t.Fatalf("WaitForReady: %v", err)
	}
	if n := probes.Load(); n < 2 {
		t.Errorf("probes = %d, want the poll loop to retry past the first failure", n)
	}
}

func TestWaitForReady_ReturnsContextErrorWhenCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := newHTTPOnlyClient(srv.URL).WaitForReady(ctx, 5*time.Second)
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("error = %v, want context canceled", err)
	}
}

func TestWaitForReady_TimesOutWhenNeverHealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	client := newHTTPOnlyClient(srv.URL)

	// The deadline is only checked on a 500ms tick, so a zero timeout still
	// waits one full tick before expiring. synctest advances that fake tick
	// instantly once the poll loop is durably blocked — with a zero timeout the
	// loop never reaches Health, so nothing inside the bubble does real I/O.
	synctest.Test(t, func(t *testing.T) {
		err := client.WaitForReady(context.Background(), 0)
		if err == nil || !strings.Contains(err.Error(), "timeout waiting for agentctl to be ready") {
			t.Fatalf("error = %v, want the ready timeout", err)
		}
	})
}

func TestStartVscode_PostsThemeAndDecodesResponse(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK,
		`{"success":true,"status":"running","port":8443}`))

	resp, err := newHTTPOnlyClient(srv.URL).StartVscode(context.Background(), "dark")
	if err != nil {
		t.Fatalf("StartVscode: %v", err)
	}

	if got.Method != http.MethodPost || got.Path != "/api/v1/vscode/start" {
		t.Errorf("request = %s %s, want POST /api/v1/vscode/start", got.Method, got.Path)
	}
	if got.ContentType != "application/json" {
		t.Errorf("content-type = %q, want application/json", got.ContentType)
	}

	var sent struct {
		Theme string `json:"theme"`
	}
	if err := json.Unmarshal(got.Body, &sent); err != nil {
		t.Fatalf("decode sent body: %v", err)
	}
	if sent.Theme != "dark" {
		t.Errorf("sent theme = %q, want dark", sent.Theme)
	}
	if !resp.Success || resp.Port != 8443 || resp.Status != "running" {
		t.Errorf("response = %+v, want success running on port 8443", resp)
	}
}

func TestStartVscode_OmitsEmptyTheme(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{"success":true}`))

	if _, err := newHTTPOnlyClient(srv.URL).StartVscode(context.Background(), ""); err != nil {
		t.Fatalf("StartVscode: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(got.Body, &raw); err != nil {
		t.Fatalf("decode sent body: %v", err)
	}
	if _, present := raw["theme"]; present {
		t.Errorf("theme sent despite being empty: %s", got.Body)
	}
}

func TestStartVscode_FailureModes(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"non-2xx status", http.StatusInternalServerError, `boom`, "vscode start failed with status 500"},
		{"malformed body", http.StatusOK, `{"success":`, "failed to parse vscode start response"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := captureServer(t, jsonResponder(tc.status, tc.body))

			resp, err := newHTTPOnlyClient(srv.URL).StartVscode(context.Background(), "dark")
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
			if resp != nil {
				t.Errorf("response = %+v, want nil on failure", resp)
			}
		})
	}
}

func TestStopVscode_PostsAndFailsOnNonOKStatus(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{"success":true}`))
	if err := newHTTPOnlyClient(srv.URL).StopVscode(context.Background()); err != nil {
		t.Fatalf("StopVscode: %v", err)
	}
	if got.Method != http.MethodPost || got.Path != "/api/v1/vscode/stop" {
		t.Errorf("request = %s %s, want POST /api/v1/vscode/stop", got.Method, got.Path)
	}

	failing, _ := captureServer(t, jsonResponder(http.StatusInternalServerError, `not running`))
	err := newHTTPOnlyClient(failing.URL).StopVscode(context.Background())
	if err == nil || !strings.Contains(err.Error(), "vscode stop failed with status 500") {
		t.Fatalf("error = %v, want status 500 named", err)
	}
	if !strings.Contains(err.Error(), "not running") {
		t.Errorf("error = %v, want the server body echoed", err)
	}
}

func TestVscodeOpenFile_PostsPathLineAndColumn(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{"success":true}`))

	err := newHTTPOnlyClient(srv.URL).VscodeOpenFile(context.Background(), "src/main.go", 42, 7)
	if err != nil {
		t.Fatalf("VscodeOpenFile: %v", err)
	}

	if got.Method != http.MethodPost || got.Path != "/api/v1/vscode/open-file" {
		t.Errorf("request = %s %s, want POST /api/v1/vscode/open-file", got.Method, got.Path)
	}

	var sent types.VscodeOpenFileRequest
	if err := json.Unmarshal(got.Body, &sent); err != nil {
		t.Fatalf("decode sent body: %v", err)
	}
	if sent.Path != "src/main.go" {
		t.Errorf("sent path = %q, want src/main.go", sent.Path)
	}
	if sent.Line != 42 || sent.Col != 7 {
		t.Errorf("sent line/col = %d / %d, want 42 / 7", sent.Line, sent.Col)
	}
}

func TestVscodeOpenFile_FailureModes(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"non-2xx status", http.StatusBadGateway, `down`, "vscode open-file failed with status 502"},
		{"malformed body", http.StatusOK, `{"success":`, "failed to parse vscode open-file response"},
		{"unsuccessful", http.StatusOK, `{"success":false,"error":"no code-server"}`, "vscode open-file failed: no code-server"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := captureServer(t, jsonResponder(tc.status, tc.body))

			err := newHTTPOnlyClient(srv.URL).VscodeOpenFile(context.Background(), "a.go", 1, 1)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestVscodeStatus_DecodesRunningState(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK,
		`{"status":"running","port":8443,"url":"http://127.0.0.1:8443/","message":"ready"}`))

	resp, err := newHTTPOnlyClient(srv.URL).VscodeStatus(context.Background())
	if err != nil {
		t.Fatalf("VscodeStatus: %v", err)
	}

	if got.Method != http.MethodGet || got.Path != "/api/v1/vscode/status" {
		t.Errorf("request = %s %s, want GET /api/v1/vscode/status", got.Method, got.Path)
	}
	if resp.Status != "running" || resp.Port != 8443 {
		t.Errorf("status/port = %q / %d, want running / 8443", resp.Status, resp.Port)
	}
	if resp.URL != "http://127.0.0.1:8443/" {
		t.Errorf("URL = %q", resp.URL)
	}
	if resp.Message != "ready" {
		t.Errorf("Message = %q, want ready", resp.Message)
	}
}

func TestVscodeStatus_FailureModes(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"non-2xx status", http.StatusInternalServerError, `boom`, "vscode status failed with status 500"},
		{"malformed body", http.StatusOK, `{"status":`, "failed to parse vscode status response"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := captureServer(t, jsonResponder(tc.status, tc.body))

			resp, err := newHTTPOnlyClient(srv.URL).VscodeStatus(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
			if resp != nil {
				t.Errorf("response = %+v, want nil on failure", resp)
			}
		})
	}
}

func TestClientHTTPMethods_HonourContextCancellation(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusOK, `{"success":true}`))
	c := newHTTPOnlyClient(srv.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	calls := map[string]func() error{
		"Health": func() error { return c.Health(ctx) },
		"GetStatus": func() error {
			_, err := c.GetStatus(ctx)
			return err
		},
		"ConfigureAgent":  func() error { return c.ConfigureAgent(ctx, "cmd", nil, nil, "", "", nil) },
		"SetMcpMode":      func() error { return c.SetMcpMode(ctx, "task") },
		"SetMcpProviders": func() error { return c.SetMcpProviders(ctx, nil) },
		"Start": func() error {
			_, err := c.Start(ctx)
			return err
		},
		"Stop": func() error { return c.Stop(ctx) },
		"StartVscode": func() error {
			_, err := c.StartVscode(ctx, "dark")
			return err
		},
		"StopVscode":     func() error { return c.StopVscode(ctx) },
		"VscodeOpenFile": func() error { return c.VscodeOpenFile(ctx, "a.go", 1, 1) },
		"VscodeStatus": func() error {
			_, err := c.VscodeStatus(ctx)
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil || !strings.Contains(err.Error(), "context canceled") {
				t.Fatalf("error = %v, want context canceled", err)
			}
		})
	}
}

func TestTruncateBody(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"short body is unchanged", "hello", "hello"},
		{"empty body", "", ""},
		{"exactly at the cap", strings.Repeat("x", 200), strings.Repeat("x", 200)},
		{"over the cap is elided", strings.Repeat("x", 201), strings.Repeat("x", 200) + "..."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := truncateBody([]byte(tc.body)); got != tc.want {
				t.Errorf("truncateBody(%d bytes) length = %d, want %d",
					len(tc.body), len(got), len(tc.want))
			}
		})
	}
}

// A truncated error body must still name the status, so an HTML error page from
// a misrouted proxy is diagnosable from the log line alone.
func TestGetStatus_TruncatesOversizedMalformedBodyInError(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusOK, strings.Repeat("z", 5000)))

	_, err := newHTTPOnlyClient(srv.URL).GetStatus(context.Background())
	if err == nil {
		t.Fatal("GetStatus returned nil error for a 5000-byte non-JSON body")
	}
	if !strings.Contains(err.Error(), "...") {
		t.Errorf("error = %v, want the body elided", err)
	}
	if len(err.Error()) > 500 {
		t.Errorf("error message is %d bytes, want the body truncated to ~200", len(err.Error()))
	}
}

// Close is safe on a client that never opened a stream, and is idempotent.
func TestClose_IsSafeAndIdempotentWithoutStreams(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusOK, `{}`))
	c := newHTTPOnlyClient(srv.URL)

	c.Close()
	c.Close()

	if !c.closed {
		t.Error("closed = false after Close")
	}
	if c.HasAgentStream() {
		t.Error("HasAgentStream = true on a client that never streamed")
	}
}
