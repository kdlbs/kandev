package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
)

// splitTestServerHostPort turns an httptest listener address into the
// (host, port) pair the New*Client constructors take.
func splitTestServerHostPort(t *testing.T, srv *httptest.Server) (string, int) {
	t.Helper()
	host, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("split server address: %v", err)
	}
	var portNumber int
	if _, err := fmt.Sscanf(port, "%d", &portNumber); err != nil {
		t.Fatalf("parse server port: %v", err)
	}
	return host, portNumber
}

// newTestControlClient builds a ControlClient pointed at srv, going through the
// real constructor so base-URL assembly and the auth transport are exercised.
func newTestControlClient(t *testing.T, srv *httptest.Server, opts ...ControlClientOption) *ControlClient {
	t.Helper()
	host, port := splitTestServerHostPort(t, srv)
	return NewControlClient(host, port, newTestLogger(), opts...)
}

func TestNewControlClient_BuildsBaseURLFromHostAndPort(t *testing.T) {
	c := NewControlClient("agentctl.internal", 7777, newTestLogger())
	if c.baseURL != "http://agentctl.internal:7777" {
		t.Errorf("baseURL = %q, want http://agentctl.internal:7777", c.baseURL)
	}
	if c.httpClient.Timeout != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", c.httpClient.Timeout)
	}
	if c.httpClient.Transport != nil {
		t.Error("transport installed without an auth token")
	}
	if c.AuthToken() != "" {
		t.Errorf("AuthToken = %q, want empty", c.AuthToken())
	}
}

func TestControlClient_SetAuthTokenIsSentOnSubsequentRequests(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	c := newTestControlClient(t, srv)
	if err := c.Health(context.Background()); err != nil {
		t.Fatalf("Health before token: %v", err)
	}

	c.SetAuthToken("rotated-token")
	if c.AuthToken() != "rotated-token" {
		t.Errorf("AuthToken = %q, want rotated-token", c.AuthToken())
	}
	if err := c.Health(context.Background()); err != nil {
		t.Fatalf("Health after token: %v", err)
	}

	if len(seen) != 2 {
		t.Fatalf("saw %d requests, want 2", len(seen))
	}
	if seen[0] != "" {
		t.Errorf("first Authorization = %q, want empty before SetAuthToken", seen[0])
	}
	if seen[1] != "Bearer rotated-token" {
		t.Errorf("second Authorization = %q, want Bearer rotated-token", seen[1])
	}
}

func TestControlClient_Health(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		wantErr string
	}{
		{"healthy", http.StatusOK, ""},
		{"unavailable", http.StatusServiceUnavailable, "health check failed: 503"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath, gotMethod string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath, gotMethod = r.URL.Path, r.Method
				w.WriteHeader(tc.status)
			}))
			t.Cleanup(srv.Close)

			err := newTestControlClient(t, srv).Health(context.Background())
			if gotMethod != http.MethodGet || gotPath != "/health" {
				t.Errorf("request = %s %s, want GET /health", gotMethod, gotPath)
			}
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Health: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestControlClient_HealthHonoursContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := newTestControlClient(t, srv).Health(ctx)
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("error = %v, want context canceled", err)
	}
}

func TestHandshake_PostsNonceAndAdoptsReturnedToken(t *testing.T) {
	var gotPath, gotMethod, gotContentType string
	var gotBody map[string]string
	var secondAuth string
	requests := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			gotPath, gotMethod = r.URL.Path, r.Method
			gotContentType = r.Header.Get("Content-Type")
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = w.Write([]byte(`{"token":"issued-token"}`))
			return
		}
		secondAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	c := newTestControlClient(t, srv)
	token, err := c.Handshake(context.Background(), "one-time-nonce")
	if err != nil {
		t.Fatalf("Handshake: %v", err)
	}

	if gotMethod != http.MethodPost || gotPath != "/auth/handshake" {
		t.Errorf("request = %s %s, want POST /auth/handshake", gotMethod, gotPath)
	}
	if gotContentType != "application/json" {
		t.Errorf("content-type = %q, want application/json", gotContentType)
	}
	if gotBody["nonce"] != "one-time-nonce" {
		t.Errorf("nonce = %q, want one-time-nonce", gotBody["nonce"])
	}
	if token != "issued-token" {
		t.Errorf("token = %q, want issued-token", token)
	}
	if c.AuthToken() != "issued-token" {
		t.Errorf("AuthToken = %q, want the handshake to adopt the token", c.AuthToken())
	}

	// The adopted token must actually reach the wire, not just the field.
	if err := c.Health(context.Background()); err != nil {
		t.Fatalf("Health after handshake: %v", err)
	}
	if secondAuth != "Bearer issued-token" {
		t.Errorf("post-handshake Authorization = %q, want Bearer issued-token", secondAuth)
	}
}

func TestHandshake_FailureModes(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{
			name:    "rejected with structured error",
			status:  http.StatusUnauthorized,
			body:    `{"error":"nonce already consumed"}`,
			wantErr: "handshake rejected: nonce already consumed (status 401)",
		},
		{
			name:    "rejected without structured error",
			status:  http.StatusForbidden,
			body:    `not json`,
			wantErr: "handshake failed: status 403",
		},
		{
			name:    "empty token",
			status:  http.StatusOK,
			body:    `{"token":""}`,
			wantErr: "handshake returned empty token",
		},
		{
			name:    "malformed success body",
			status:  http.StatusOK,
			body:    `{"token":`,
			wantErr: "failed to decode handshake response",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(jsonResponder(tc.status, tc.body))
			t.Cleanup(srv.Close)

			c := newTestControlClient(t, srv)
			token, err := c.Handshake(context.Background(), "n")
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
			if token != "" {
				t.Errorf("token = %q, want empty on failure", token)
			}
			if c.AuthToken() != "" {
				t.Errorf("AuthToken = %q, want no token adopted on failure", c.AuthToken())
			}
		})
	}
}

func TestSubprocessAdmission_FailureModes(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"non-OK status", http.StatusInternalServerError, `{}`, "failed to get subprocess admission: status 500"},
		{"malformed body", http.StatusOK, `{"pool":`, "failed to decode subprocess admission"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(jsonResponder(tc.status, tc.body))
			t.Cleanup(srv.Close)

			snapshot, err := newTestControlClient(t, srv).SubprocessAdmission(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
			if snapshot.Pool != "" || snapshot.Capacity != 0 {
				t.Errorf("snapshot = %+v, want the zero value on failure", snapshot)
			}
		})
	}
}

func TestCreateInstance_MarshalsFullRequestAndDecodesIDAndPort(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusCreated, `{"id":"inst-1","port":41234}`))

	autoApprove := true
	req := &CreateInstanceRequest{
		ID:                     "inst-1",
		WorkspacePath:          "/workspace/task-1",
		AgentCommand:           "claude-code acp",
		Protocol:               "acp",
		AgentType:              "codex",
		WorkspaceFlag:          "--workspace-root",
		Env:                    map[string]string{"FOO": "bar"},
		AutoStart:              true,
		AutoApprovePermissions: &autoApprove,
		McpServers: []McpServerConfig{{
			Name:    "kandev",
			URL:     "http://127.0.0.1:9/mcp",
			Type:    "http",
			Command: "kandev-mcp",
			Args:    []string{"--stdio"},
			Env:     map[string]string{"TOKEN": "t"},
			Headers: map[string]string{"X-Trace": "1"},
		}},
		SessionID:            "sess-1",
		TaskID:               "task-1",
		DisableAskQuestion:   true,
		AssumeMcpSse:         true,
		AssumeMcpHttp:        true,
		McpMode:              "task",
		McpProviders:         []string{"github"},
		McpProfile:           &mcpprofile.Context{},
		RequiresProcessKill:  true,
		StripEnv:             []string{"ANTHROPIC_API_KEY"},
		BaseBranches:         map[string]string{"": "origin/main", "web": "origin/develop"},
		RemoteContributions:  map[string]models.RemoteContribution{"": {}},
		WorkspaceSourceRoots: []string{"/sources/a"},
	}

	resp, err := newTestControlClient(t, srv).CreateInstance(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}

	if got.Method != http.MethodPost || got.Path != "/api/v1/instances" {
		t.Errorf("request = %s %s, want POST /api/v1/instances", got.Method, got.Path)
	}
	if got.ContentType != "application/json" {
		t.Errorf("content-type = %q, want application/json", got.ContentType)
	}

	var sent CreateInstanceRequest
	if err := json.Unmarshal(got.Body, &sent); err != nil {
		t.Fatalf("decode sent body: %v", err)
	}
	if sent.ID != "inst-1" || sent.WorkspacePath != "/workspace/task-1" {
		t.Errorf("sent id/workspace = %q / %q", sent.ID, sent.WorkspacePath)
	}
	if sent.AgentCommand != "claude-code acp" || sent.Protocol != "acp" || sent.AgentType != "codex" {
		t.Errorf("sent command/protocol/type = %q / %q / %q", sent.AgentCommand, sent.Protocol, sent.AgentType)
	}
	if sent.WorkspaceFlag != "--workspace-root" || sent.Env["FOO"] != "bar" || !sent.AutoStart {
		t.Errorf("sent flag/env/autostart = %q / %v / %v", sent.WorkspaceFlag, sent.Env, sent.AutoStart)
	}
	if sent.AutoApprovePermissions == nil || !*sent.AutoApprovePermissions {
		t.Errorf("sent AutoApprovePermissions = %v, want pointer to true", sent.AutoApprovePermissions)
	}
	if len(sent.McpServers) != 1 {
		t.Fatalf("sent McpServers = %d, want 1", len(sent.McpServers))
	}
	mcp := sent.McpServers[0]
	if mcp.Name != "kandev" || mcp.URL != "http://127.0.0.1:9/mcp" || mcp.Type != "http" {
		t.Errorf("sent mcp identity = %+v", mcp)
	}
	if mcp.Command != "kandev-mcp" || len(mcp.Args) != 1 || mcp.Args[0] != "--stdio" {
		t.Errorf("sent mcp command/args = %q / %v", mcp.Command, mcp.Args)
	}
	if mcp.Env["TOKEN"] != "t" || mcp.Headers["X-Trace"] != "1" {
		t.Errorf("sent mcp env/headers = %v / %v", mcp.Env, mcp.Headers)
	}
	if sent.SessionID != "sess-1" || sent.TaskID != "task-1" {
		t.Errorf("sent session/task = %q / %q", sent.SessionID, sent.TaskID)
	}
	if !sent.DisableAskQuestion || !sent.AssumeMcpSse || !sent.AssumeMcpHttp {
		t.Errorf("sent mcp capability flags = %v / %v / %v",
			sent.DisableAskQuestion, sent.AssumeMcpSse, sent.AssumeMcpHttp)
	}
	if sent.McpMode != "task" || len(sent.McpProviders) != 1 || sent.McpProviders[0] != "github" {
		t.Errorf("sent mcp mode/providers = %q / %v", sent.McpMode, sent.McpProviders)
	}
	if sent.McpProfile == nil {
		t.Error("sent McpProfile = nil, want the backend-owned profile forwarded")
	}
	if !sent.RequiresProcessKill {
		t.Error("sent RequiresProcessKill = false, want true")
	}
	if len(sent.StripEnv) != 1 || sent.StripEnv[0] != "ANTHROPIC_API_KEY" {
		t.Errorf("sent StripEnv = %v", sent.StripEnv)
	}
	if sent.BaseBranches[""] != "origin/main" || sent.BaseBranches["web"] != "origin/develop" {
		t.Errorf("sent BaseBranches = %v", sent.BaseBranches)
	}
	if _, ok := sent.RemoteContributions[""]; !ok {
		t.Errorf("sent RemoteContributions = %v, want the root binding", sent.RemoteContributions)
	}
	if len(sent.WorkspaceSourceRoots) != 1 || sent.WorkspaceSourceRoots[0] != "/sources/a" {
		t.Errorf("sent WorkspaceSourceRoots = %v", sent.WorkspaceSourceRoots)
	}

	if resp.ID != "inst-1" || resp.Port != 41234 {
		t.Errorf("response = %+v, want inst-1 on port 41234", resp)
	}
}

// TestCreateInstance_MarshalsMCPToolNamePresentationCapability covers the
// wire portion of AC-TASKS-MCP-TOOL-NAMES-001.5.
func TestCreateInstance_MarshalsMCPToolNamePresentationCapability(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusCreated, `{"id":"inst-1","port":41234}`))
	req := &CreateInstanceRequest{
		WorkspacePath:              "/workspace/task-1",
		NamespacesMCPToolsByServer: true,
	}

	if _, err := newTestControlClient(t, srv).CreateInstance(context.Background(), req); err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(got.Body, &body); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if got, ok := body["namespaces_mcp_tools_by_server"].(bool); !ok || !got {
		t.Fatalf("serialized capability = %#v, want true", body["namespaces_mcp_tools_by_server"])
	}
}

// 200 is accepted alongside 201 — agentctl returns 200 when it reuses an
// existing instance for the same ID.
func TestCreateInstance_AcceptsBoth200And201(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusCreated} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(jsonResponder(status, `{"id":"i","port":1}`))
			t.Cleanup(srv.Close)

			resp, err := newTestControlClient(t, srv).CreateInstance(
				context.Background(), &CreateInstanceRequest{WorkspacePath: "/w"})
			if err != nil {
				t.Fatalf("CreateInstance on %d: %v", status, err)
			}
			if resp.ID != "i" || resp.Port != 1 {
				t.Errorf("response = %+v", resp)
			}
		})
	}
}

func TestCreateInstance_FailureModes(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{
			name:    "structured error",
			status:  http.StatusConflict,
			body:    `{"error":"port range exhausted"}`,
			wantErr: "failed to create instance: port range exhausted (status 409)",
		},
		{
			name:    "unparseable error body",
			status:  http.StatusBadGateway,
			body:    `<html>502</html>`,
			wantErr: "failed to decode error response",
		},
		{
			name:    "malformed success body",
			status:  http.StatusCreated,
			body:    `{"id":`,
			wantErr: "failed to decode response",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(jsonResponder(tc.status, tc.body))
			t.Cleanup(srv.Close)

			resp, err := newTestControlClient(t, srv).CreateInstance(
				context.Background(), &CreateInstanceRequest{WorkspacePath: "/w"})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
			if resp != nil {
				t.Errorf("response = %+v, want nil on failure", resp)
			}
		})
	}
}

func TestDeleteInstance_UsesDeleteVerbWithIDInPath(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusNoContent} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv, got := captureServer(t, jsonResponder(status, ``))

			if err := newTestControlClient(t, srv).DeleteInstance(
				context.Background(), "inst-9"); err != nil {
				t.Fatalf("DeleteInstance: %v", err)
			}
			if got.Method != http.MethodDelete {
				t.Errorf("method = %q, want DELETE", got.Method)
			}
			if got.Path != "/api/v1/instances/inst-9" {
				t.Errorf("path = %q, want the ID as the last segment", got.Path)
			}
		})
	}
}

func TestDeleteInstance_LostResponseThenNotFoundIsSuccessfulRetry(t *testing.T) {
	transport := &lostDeleteResponseTransport{}
	client := NewControlClient("agentctl.test", 80, newTestLogger())
	client.httpClient = &http.Client{Transport: transport}

	if err := client.DeleteInstance(context.Background(), "inst-9"); err == nil {
		t.Fatal("first DeleteInstance error = nil, want lost-response error")
	}
	if err := client.DeleteInstance(context.Background(), "inst-9"); err != nil {
		t.Fatalf("DeleteInstance retry after completed delete: %v", err)
	}
	if transport.requests != 2 {
		t.Fatalf("requests = %d, want 2", transport.requests)
	}
}

type lostDeleteResponseTransport struct {
	requests int
}

func (t *lostDeleteResponseTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.requests++
	if t.requests == 1 {
		return nil, errors.New("delete response lost")
	}
	return &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(strings.NewReader(`{"error":"instance not found"}`)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func TestDeleteInstance_FailureModes(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{
			name:    "structured error",
			status:  http.StatusConflict,
			body:    `{"error":"instance still stopping"}`,
			wantErr: "failed to delete instance: instance still stopping (status 409)",
		},
		{
			name:    "unparseable error body",
			status:  http.StatusInternalServerError,
			body:    `boom`,
			wantErr: "failed to decode error response",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(jsonResponder(tc.status, tc.body))
			t.Cleanup(srv.Close)

			err := newTestControlClient(t, srv).DeleteInstance(context.Background(), "inst-9")
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestGetInstance_DecodesEveryField(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{
		"id":"inst-3","port":40001,"status":"running",
		"workspace_path":"/workspace/task-3","agent_command":"claude-code acp",
		"env":{"FOO":"bar"},"created_at":"2026-08-10T12:34:56Z"
	}`))

	info, err := newTestControlClient(t, srv).GetInstance(context.Background(), "inst-3")
	if err != nil {
		t.Fatalf("GetInstance: %v", err)
	}

	if got.Method != http.MethodGet || got.Path != "/api/v1/instances/inst-3" {
		t.Errorf("request = %s %s, want GET /api/v1/instances/inst-3", got.Method, got.Path)
	}

	if info.ID != "inst-3" || info.Port != 40001 || info.Status != "running" {
		t.Errorf("id/port/status = %q / %d / %q", info.ID, info.Port, info.Status)
	}
	if info.WorkspacePath != "/workspace/task-3" || info.AgentCommand != "claude-code acp" {
		t.Errorf("workspace/command = %q / %q", info.WorkspacePath, info.AgentCommand)
	}
	if info.Env["FOO"] != "bar" {
		t.Errorf("Env = %v, want FOO=bar", info.Env)
	}
	wantCreated := time.Date(2026, 8, 10, 12, 34, 56, 0, time.UTC)
	if !info.CreatedAt.Equal(wantCreated) {
		t.Errorf("CreatedAt = %v, want %v", info.CreatedAt, wantCreated)
	}
}

func TestGetInstance_FailureModes(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"not found", http.StatusNotFound, `{}`, `instance "inst-3" not found`},
		{"other status", http.StatusInternalServerError, `{}`, "failed to get instance: status 500"},
		{"malformed body", http.StatusOK, `{"id":`, "failed to decode response"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(jsonResponder(tc.status, tc.body))
			t.Cleanup(srv.Close)

			info, err := newTestControlClient(t, srv).GetInstance(context.Background(), "inst-3")
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
			if info != nil {
				t.Errorf("info = %+v, want nil on failure", info)
			}
		})
	}
}

func TestListInstances_UnwrapsInstancesEnvelope(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{
		"instances":[
			{"id":"a","port":1,"status":"running","workspace_path":"/w/a","agent_command":"cmd-a"},
			{"id":"b","port":2,"status":"stopped","workspace_path":"/w/b","agent_command":"cmd-b"}
		]
	}`))

	instances, err := newTestControlClient(t, srv).ListInstances(context.Background())
	if err != nil {
		t.Fatalf("ListInstances: %v", err)
	}

	if got.Method != http.MethodGet || got.Path != "/api/v1/instances" {
		t.Errorf("request = %s %s, want GET /api/v1/instances", got.Method, got.Path)
	}
	if len(instances) != 2 {
		t.Fatalf("instances = %d, want 2", len(instances))
	}
	if instances[0].ID != "a" || instances[0].Port != 1 ||
		instances[0].Status != "running" || instances[0].WorkspacePath != "/w/a" ||
		instances[0].AgentCommand != "cmd-a" {
		t.Errorf("instances[0] = %+v", instances[0])
	}
	if instances[1].ID != "b" || instances[1].Port != 2 || instances[1].Status != "stopped" {
		t.Errorf("instances[1] = %+v", instances[1])
	}
}

func TestListInstances_FailureModes(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"non-OK status", http.StatusBadGateway, `{}`, "failed to list instances: status 502"},
		{"malformed body", http.StatusOK, `{"instances":`, "failed to decode response"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(jsonResponder(tc.status, tc.body))
			t.Cleanup(srv.Close)

			instances, err := newTestControlClient(t, srv).ListInstances(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
			if instances != nil {
				t.Errorf("instances = %+v, want nil on failure", instances)
			}
		})
	}
}
