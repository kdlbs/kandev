//go:build !windows

package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/instance"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	mcpserver "github.com/kandev/kandev/internal/mcp/server"
)

// The tests in this file pin the pre-launch surface of agentctl: what a caller
// without the token can reach, and which processes agentctl may start before an
// authenticated instance exists.

const (
	surfaceControlToken = "control-secret-token"
	surfaceNonce        = "bootstrap-nonce-value"
	surfaceInstanceTok  = "instance-secret-token"
)

// controlUnauthenticatedRoutes is the complete set of control-server routes that
// are reachable without the bearer token (NewControlServer's exempt list).
var controlUnauthenticatedRoutes = []string{
	"GET /health",
	"POST /auth/handshake",
	"GET /identity",
	"POST /ownership/prove",
}

// instanceUnauthenticatedPaths is the complete set of instance-server paths that
// are reachable without the bearer token (NewServer's exempt list). The three
// MCP paths serve the agent subprocess, which does not hold the token.
var instanceUnauthenticatedPaths = []string{"/health", "/sse", "/message", "/mcp"}

// launchRecorder replaces the executables an unexpected launch would reach with
// shims that record their name and arguments and exit nonzero. Names that
// agentctl resolves through PATH are shadowed ahead of the real ones.
type launchRecorder struct{ logPath string }

func installLaunchRecorder(t *testing.T) *launchRecorder {
	t.Helper()
	dir := t.TempDir()
	rec := &launchRecorder{logPath: filepath.Join(dir, "launches.log")}
	require.NoError(t, os.WriteFile(rec.logPath, nil, 0o600))
	script := "#!/bin/sh\nname=${0##*/}\necho \"$name $*\" >> \"" + rec.logPath + "\"\nexit 1\n"
	for _, name := range []string{
		"pgrep", "ps", "git", "sh", "bash", "zsh", "code-server", "node", "npx", "npm", "gh", "claude-agent-acp", "codex", "opencode",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(script), 0o700))
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return rec
}

func (r *launchRecorder) launches(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(r.logPath)
	require.NoError(t, err)
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func (r *launchRecorder) reset(t *testing.T) {
	t.Helper()
	require.NoError(t, os.WriteFile(r.logPath, nil, 0o600))
}

func concretePath(route string) string {
	parts := strings.Split(route, "/")
	for i, part := range parts {
		switch {
		case strings.HasPrefix(part, ":"):
			parts[i] = "x"
		case strings.HasPrefix(part, "*"):
			parts[i] = "x"
		}
	}
	return strings.Join(parts, "/")
}

func serve(handler http.Handler, method, path, authorization string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func newSurfaceControlServer(t *testing.T) (*ControlServer, *instance.Manager) {
	t.Helper()
	mgr := instance.NewManager(&config.Config{AuthToken: surfaceControlToken}, logger.Default())
	t.Cleanup(func() { _ = mgr.Shutdown(t.Context()) })
	cfg := &config.Config{AuthToken: surfaceControlToken, BootstrapNonce: surfaceNonce}
	return NewControlServer(cfg, mgr, logger.Default()), mgr
}

// TestControlServerOnlyDocumentedRoutesAcceptRequestsWithoutTheToken enumerates
// every route registered on the real control router and requires that exactly
// the documented four answer without a valid bearer token. Only the unauthenticated
// requests are sent: the token-bearing routes create instances and request shutdown.
func TestControlServerOnlyDocumentedRoutesAcceptRequestsWithoutTheToken(t *testing.T) {
	server, _ := newSurfaceControlServer(t)
	routes := server.router.Routes()
	require.Greater(t, len(routes), 20, "route enumeration returned too few routes to be meaningful")

	var open []string
	for _, route := range routes {
		path := concretePath(route.Path)
		answered := false
		for _, authorization := range []string{"", "Bearer wrong-token", "Basic " + surfaceControlToken, "Bearer"} {
			if rec := serve(server.router, route.Method, path, authorization); rec.Code != http.StatusUnauthorized {
				answered = true
			}
		}
		if answered {
			open = append(open, route.Method+" "+route.Path)
		}
	}
	sort.Strings(open)
	want := append([]string(nil), controlUnauthenticatedRoutes...)
	sort.Strings(want)
	require.Equal(t, want, open, "the set of control routes reachable without the token changed")

	require.Equal(t, http.StatusOK, serve(server.router, http.MethodGet, "/api/v1/instances", "Bearer "+surfaceControlToken).Code,
		"the valid token must still be accepted")
}

// TestInstanceServerOnlyDocumentedPathsAcceptRequestsWithoutTheToken does the
// same for the per-instance router, with every conditional route enabled.
func TestInstanceServerOnlyDocumentedPathsAcceptRequestsWithoutTheToken(t *testing.T) {
	t.Setenv("KANDEV_DEBUG_PPROF_ENABLED", "true")
	t.Setenv("KANDEV_DEBUG_AGENT_MESSAGES", "true")
	t.Setenv("KANDEV_DEBUG_DEV_MODE", "true")
	srv := newSurfaceInstanceServer(t)

	routes := srv.router.Routes()
	require.Greater(t, len(routes), 100, "route enumeration returned too few routes to be meaningful")
	exempt := map[string]bool{}
	for _, path := range instanceUnauthenticatedPaths {
		exempt[path] = true
	}
	exemptSeen := map[string]bool{}
	for _, route := range routes {
		if exempt[route.Path] {
			exemptSeen[route.Path] = true
			continue
		}
		path := concretePath(route.Path)
		for _, authorization := range []string{"", "Bearer wrong-token", "Basic " + surfaceInstanceTok, "Bearer"} {
			rec := serve(srv.router, route.Method, path, authorization)
			require.Equal(t, http.StatusUnauthorized, rec.Code,
				"%s %s answered %d with authorization %q; every route outside the documented exemptions needs the token",
				route.Method, route.Path, rec.Code, authorization)
		}
	}
	require.Len(t, exemptSeen, len(instanceUnauthenticatedPaths), "an exempt path is no longer routed, so the documented list is stale")
}

func newSurfaceInstanceServer(t *testing.T) *Server {
	t.Helper()
	log := newTestLogger()
	cfg := &config.InstanceConfig{Port: 0, WorkDir: t.TempDir(), AuthToken: surfaceInstanceTok}
	procMgr := process.NewManager(cfg, log)
	backend := mcpserver.NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	mcpSrv := mcpserver.New(backend, "surface-session", "surface-task", 0, log, "", false, mcpserver.ModeTask)
	return NewServer(cfg, procMgr, mcpSrv, nil, log)
}

// TestUnauthenticatedCallsToSpawningInstanceRoutesStartNothing sends a
// well-formed request to every instance route that can start a process, without
// the token, and requires that no process of any kind was launched and no
// process-manager state changed.
func TestUnauthenticatedCallsToSpawningInstanceRoutesStartNothing(t *testing.T) {
	rec := installLaunchRecorder(t)
	srv := newSurfaceInstanceServer(t)
	rec.reset(t)

	spawning := []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/start", `{}`},
		{http.MethodPost, "/api/v1/agent/configure", `{"command":"codex","agent_args":["--acp"]}`},
		{http.MethodPost, "/api/v1/shell/start", `{}`},
		{http.MethodPost, "/api/v1/shell/terminal/start", `{"terminal_id":"t1"}`},
		{http.MethodPost, "/api/v1/processes/start", `{"session_id":"s","command":"echo hi"}`},
		{http.MethodPost, "/api/v1/vscode/start", `{}`},
		{http.MethodPost, "/api/v1/inference/probe", `{"agent_id":"a","inference_config":{"command":["claude-agent-acp"],"work_dir":"/tmp"}}`},
		{http.MethodPost, "/api/v1/inference/prompt", `{"agent_id":"a","prompt":"p","inference_config":{"command":["claude-agent-acp"],"work_dir":"/tmp"}}`},
		{http.MethodPost, "/api/v1/git/commit", `{"message":"m"}`},
		{http.MethodPost, "/api/v1/git/pull", `{}`},
		{http.MethodGet, "/api/v1/git/status", ``},
		{http.MethodGet, "/api/v1/workspace/tree", ``},
		{http.MethodPost, "/api/v1/workspace/rescan", `{}`},
		{http.MethodGet, "/api/v1/agent/stream", ``},
		{http.MethodGet, "/api/v1/workspace/stream", ``},
	}
	for _, call := range spawning {
		req := httptest.NewRequest(call.method, call.path, strings.NewReader(call.body))
		req.Header.Set("Content-Type", "application/json")
		out := httptest.NewRecorder()
		srv.router.ServeHTTP(out, req)
		require.Equal(t, http.StatusUnauthorized, out.Code, "%s %s", call.method, call.path)
	}

	require.Empty(t, rec.launches(t), "an unauthenticated request launched a process")
	require.Equal(t, process.StatusStopped, srv.procMgr.Status())
	require.Empty(t, srv.procMgr.ListProcesses(""))
	require.Nil(t, srv.procMgr.Shell())
	require.Equal(t, process.VscodeStatusStopped, srv.procMgr.VscodeInfo().Status)
}

// TestControlServerBeforeAuthenticatedCreateStartsNoInstanceOrProcess drives
// every control route without the token, then the handshake itself, and
// requires that agentctl still has no instance, no process manager and has
// launched no process. The handshake issues the token; it must not spawn.
func TestControlServerBeforeAuthenticatedCreateStartsNoInstanceOrProcess(t *testing.T) {
	rec := installLaunchRecorder(t)
	server, mgr := newSurfaceControlServer(t)
	rec.reset(t)

	for _, route := range server.router.Routes() {
		serve(server.router, route.Method, concretePath(route.Path), "")
	}

	wrong := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/auth/handshake", strings.NewReader(`{"nonce":"not-the-nonce"}`))
	req.Header.Set("Content-Type", "application/json")
	server.router.ServeHTTP(wrong, req)
	require.Equal(t, http.StatusForbidden, wrong.Code)

	good := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/auth/handshake", strings.NewReader(`{"nonce":"`+surfaceNonce+`"}`))
	req.Header.Set("Content-Type", "application/json")
	server.router.ServeHTTP(good, req)
	require.Equal(t, http.StatusOK, good.Code)
	require.Contains(t, good.Body.String(), surfaceControlToken, "the handshake must return the token it guards")

	again := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/auth/handshake", strings.NewReader(`{"nonce":"`+surfaceNonce+`"}`))
	req.Header.Set("Content-Type", "application/json")
	server.router.ServeHTTP(again, req)
	require.Equal(t, http.StatusForbidden, again.Code, "the nonce is one-shot")

	require.Empty(t, mgr.ListInstances(), "an instance exists without an authenticated create")
	require.Empty(t, rec.launches(t), "the control server launched a process without an authenticated create")
}

// TestAgentctlStartupLaunchesExactlyTheOrphanSweep is a characterization pin.
// Constructing the instance manager, which agentctl does at startup before any
// request, runs `pgrep -f code-server.*--bind-addr` and nothing else. That is
// the only process agentctl starts before an authenticated instance create. Any
// further startup launch makes this test fail.
func TestAgentctlStartupLaunchesExactlyTheOrphanSweep(t *testing.T) {
	rec := installLaunchRecorder(t)
	mgr := instance.NewManager(&config.Config{AuthToken: surfaceControlToken}, logger.Default())
	t.Cleanup(func() { _ = mgr.Shutdown(t.Context()) })
	NewControlServer(&config.Config{AuthToken: surfaceControlToken, BootstrapNonce: surfaceNonce}, mgr, logger.Default())

	require.Equal(t, []string{"pgrep -f code-server.*--bind-addr"}, rec.launches(t))
}
