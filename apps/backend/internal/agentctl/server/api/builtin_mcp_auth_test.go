package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	p "github.com/kandev/kandev/internal/mcp/profile"
	mcpserver "github.com/kandev/kandev/internal/mcp/server"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest/observer"
)

const (
	testControlSecret = "control-secret"
	testMCPCredential = "instance-mcp-credential"
	initializeBody    = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`
)

type callerAuthServer struct {
	server   *Server
	observed *observer.ObservedLogs
}

func newCallerAuthServer(t *testing.T, enforced bool, credential string) callerAuthServer {
	t.Helper()
	profile := p.New(p.SurfaceKanbanTask, nil, nil)
	profile.BuiltinPackages = &p.BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: profile.Surface,
		Packages: []p.PackageID{p.PackageCore, p.PackageConfiguration}, Origins: map[p.PackageID]string{p.PackageCore: "required", p.PackageConfiguration: "task"}}
	cfg, err := (&config.Config{AuthToken: testControlSecret}).NewInstanceConfig(12345, &config.InstanceOverrides{
		WorkDir: t.TempDir(), TaskID: "task", SessionID: "session", McpProfile: &profile,
		BuiltinMCPCredential: credential, BuiltinMCPAuthEnforced: enforced,
	})
	require.NoError(t, err)
	log, observed := newObservedGitAPILogger(t)
	manager := process.NewManager(cfg, log)
	backend := mcpserver.NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	mcp := mcpserver.NewWithProfile(backend, "session", "task", cfg.Port, log, "", false, profile)
	t.Cleanup(func() { require.NoError(t, mcp.Close(context.Background())) })
	return callerAuthServer{server: NewServer(cfg, manager, mcp, nil, log), observed: observed}
}

func mcpRequest(method, path, bearer string) *http.Request {
	var body *strings.Reader
	if method == http.MethodPost {
		body = strings.NewReader(initializeBody)
	} else {
		body = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	return req
}

// postRoutes are every built-in MCP route that accepts a request without
// opening a long-lived stream.
func postRoutes() []string {
	var routes []string
	for _, path := range builtinMCPRoutePaths() {
		if !strings.HasPrefix(path, "/sse") {
			routes = append(routes, path)
		}
	}
	return routes
}

func TestBuiltinMCPCallerAuth_RejectsMissingCredential(t *testing.T) {
	s := newCallerAuthServer(t, true, testMCPCredential)
	for _, path := range builtinMCPRoutePaths() {
		method := http.MethodPost
		if strings.HasPrefix(path, "/sse") {
			method = http.MethodGet
		}
		rec := httptest.NewRecorder()
		s.server.router.ServeHTTP(rec, mcpRequest(method, path, ""))
		require.Equal(t, http.StatusUnauthorized, rec.Code, path)
		require.Contains(t, rec.Body.String(), errBuiltinMCPCredentialRequired, path)
	}
}

func TestBuiltinMCPCallerAuth_RejectsForeignCredential(t *testing.T) {
	s := newCallerAuthServer(t, true, testMCPCredential)
	for _, path := range postRoutes() {
		rec := httptest.NewRecorder()
		s.server.router.ServeHTTP(rec, mcpRequest(http.MethodPost, path, "another-instance-credential"))
		require.Equal(t, http.StatusUnauthorized, rec.Code, path)
	}
}

func TestBuiltinMCPCallerAuth_ControlCredentialRejected(t *testing.T) {
	s := newCallerAuthServer(t, true, testMCPCredential)
	rec := httptest.NewRecorder()
	s.server.router.ServeHTTP(rec, mcpRequest(http.MethodPost, "/mcp", testControlSecret))
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestInstanceAuth_MCPCredentialRejectedOnControlRoutes(t *testing.T) {
	s := newCallerAuthServer(t, true, testMCPCredential)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	req.Header.Set("Authorization", "Bearer "+testMCPCredential)
	rec := httptest.NewRecorder()
	s.server.router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestBuiltinMCPCallerAuth_HealthStaysOpen(t *testing.T) {
	s := newCallerAuthServer(t, true, testMCPCredential)
	rec := httptest.NewRecorder()
	s.server.router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestBuiltinMCPCallerAuth_NoOAuthMetadata(t *testing.T) {
	s := newCallerAuthServer(t, true, testMCPCredential)
	rec := httptest.NewRecorder()
	s.server.router.ServeHTTP(rec, mcpRequest(http.MethodPost, "/mcp", ""))
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Empty(t, rec.Header().Get("WWW-Authenticate"))
}

func TestBuiltinMCPCallerAuth_OwnCredentialServesToolsAndSSE(t *testing.T) {
	s := newCallerAuthServer(t, true, testMCPCredential)
	for _, path := range []string{"/mcp", "/mcp/packages/configuration"} {
		rec := httptest.NewRecorder()
		s.server.router.ServeHTTP(rec, mcpRequest(http.MethodPost, path, testMCPCredential))
		require.Equal(t, http.StatusOK, rec.Code, path+": "+rec.Body.String())
	}
	httpServer := httptest.NewServer(s.server.router)
	t.Cleanup(httpServer.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, httpServer.URL+"/sse", nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+testMCPCredential)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "text/event-stream")
}

func TestBuiltinMCPCallerAuth_DisabledPassesThrough(t *testing.T) {
	s := newCallerAuthServer(t, false, testMCPCredential)
	rec := httptest.NewRecorder()
	s.server.router.ServeHTTP(rec, mcpRequest(http.MethodPost, "/mcp", ""))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, testMCPCredential, s.server.cfg.BuiltinMCPCredential)
}

func TestBuiltinMCPCallerAuth_EnforcementRequiresCredential(t *testing.T) {
	s := newCallerAuthServer(t, true, "")
	require.False(t, s.server.cfg.BuiltinMCPAuthEnforced)
}

func TestBuiltinMCPCallerAuth_RejectionLogIsRateLimited(t *testing.T) {
	s := newCallerAuthServer(t, true, testMCPCredential)
	for range 3 {
		rec := httptest.NewRecorder()
		s.server.router.ServeHTTP(rec, mcpRequest(http.MethodPost, "/mcp", "wrong-credential"))
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	}
	entries := s.observed.FilterMessageSnippet("rejected built-in MCP request").All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Equal(t, "/mcp", fields["route"])
	require.Equal(t, "session", fields["session_id"])
	require.Equal(t, "task", fields["task_id"])
	require.Equal(t, builtinMCPRejectInvalid, fields["reason"])
	for _, entry := range s.observed.All() {
		for _, value := range entry.ContextMap() {
			require.NotContains(t, fmt.Sprint(value), testMCPCredential)
			require.NotContains(t, fmt.Sprint(value), "wrong-credential")
		}
	}
}

func TestRejectionLogLimiter_ReportsSuppressedCount(t *testing.T) {
	now := time.Unix(0, 0)
	limiter := newRejectionLogLimiter(time.Minute, func() time.Time { return now })
	emit, _ := limiter.allow("k")
	require.True(t, emit)
	emit, _ = limiter.allow("k")
	require.False(t, emit)
	emit, _ = limiter.allow("other")
	require.True(t, emit)
	now = now.Add(time.Minute)
	emit, suppressed := limiter.allow("k")
	require.True(t, emit)
	require.Equal(t, 1, suppressed)
}

func TestBuiltinMCPCredentialRoute_RequiresControlCredential(t *testing.T) {
	s := newCallerAuthServer(t, true, testMCPCredential)
	for _, tc := range []struct {
		bearer string
		status int
	}{
		{bearer: "", status: http.StatusUnauthorized},
		{bearer: testMCPCredential, status: http.StatusUnauthorized},
		{bearer: testControlSecret, status: http.StatusOK},
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/mcp/builtin-credential", nil)
		if tc.bearer != "" {
			req.Header.Set("Authorization", "Bearer "+tc.bearer)
		}
		rec := httptest.NewRecorder()
		s.server.router.ServeHTTP(rec, req)
		require.Equal(t, tc.status, rec.Code, tc.bearer)
		if tc.status == http.StatusOK {
			require.JSONEq(t, `{"credential":"`+testMCPCredential+`","enforced":true}`, rec.Body.String())
			require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
		}
	}
}
