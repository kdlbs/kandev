package mcp

import (
	"bufio"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/mcp/plugintools"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

const (
	workspaceMCPToken      = "workspace-token"
	workspaceMCPOtherToken = "workspace-other-token"
	workspaceMCPToolName   = "kandev_kandev_workspace_reader_read_workspace_data"
)

type workspaceIdentityBackend struct {
	mu             sync.Mutex
	callIdentities []authn.Identity
	identityFound  []bool
	calls          []map[string]any
}

func (b *workspaceIdentityBackend) RequestPayload(ctx context.Context, action string, payload, result interface{}) error {
	switch action {
	case ws.ActionMCPListPluginTools:
		tool := plugintools.Definition{
			PluginID: "kandev-workspace-reader", LocalName: "read_workspace_data",
			ExposedName:     plugintools.ExposedName("kandev-workspace-reader", "read_workspace_data"),
			Description:     "Reads workspace-owned data.",
			Surfaces:        []string{plugintools.SurfaceKanban, plugintools.SurfaceExternal},
			InvocationScope: plugintools.InvocationScopeWorkspace,
			InstallationID:  "installation-1",
			PolicyVersion:   "policy-1",
			InputSchema: []byte(`{"type":"object","properties":{"key":{"type":"string"},` +
				`"kandev_workspace_id":{"type":"string","minLength":1}},"required":["kandev_workspace_id"]}`),
			ReadOnlyHint: true,
		}
		return decodeInto(plugintools.Snapshot{Generation: "workspace", Revision: 1,
			Tools: []plugintools.Definition{tool}}, result)
	case ws.ActionMCPInvokePluginTool:
		identity, ok := authn.IdentityFromContext(ctx)
		values, _ := payload.(map[string]any)
		b.mu.Lock()
		b.callIdentities = append(b.callIdentities, identity)
		b.identityFound = append(b.identityFound, ok)
		b.calls = append(b.calls, values)
		b.mu.Unlock()
		return decodeInto(map[string]any{"text": "workspace data", "is_error": false}, result)
	}
	return nil
}

func (b *workspaceIdentityBackend) recordedCalls() ([]authn.Identity, []bool, []map[string]any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]authn.Identity(nil), b.callIdentities...),
		append([]bool(nil), b.identityFound...),
		append([]map[string]any(nil), b.calls...)
}

type workspaceMCPAuth struct {
	mu       sync.RWMutex
	disabled bool
	revoked  map[string]bool
}

func newWorkspaceMCPAuth() *workspaceMCPAuth {
	return &workspaceMCPAuth{revoked: map[string]bool{}}
}

func (a *workspaceMCPAuth) middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		a.mu.RLock()
		disabled := a.disabled
		a.mu.RUnlock()
		if disabled {
			authn.SetOnGin(c, authn.Identity{UserID: "synthetic-user", Role: authn.RoleAdmin, Synthetic: true})
			c.Next()
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer"))
		a.mu.RLock()
		revoked := a.revoked[token]
		a.mu.RUnlock()
		if token == "" || revoked {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		userID := "user-1"
		if token == workspaceMCPOtherToken {
			userID = "user-2"
		}
		authn.SetOnGin(c, authn.Identity{UserID: userID})
		c.Next()
	}
}

func (a *workspaceMCPAuth) revoke(token string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.revoked[token] = true
}

func (a *workspaceMCPAuth) disable() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.disabled = true
}

func newWorkspaceIdentityServer(t *testing.T, auth *workspaceMCPAuth) (*workspaceIdentityBackend, *httptest.Server) {
	t.Helper()
	backend := &workspaceIdentityBackend{}
	srv := NewExternal(backend, newTestLogger(t), "")
	t.Cleanup(func() { require.NoError(t, srv.Close(context.Background())) })
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(auth.middleware())
	srv.RegisterBackendRoutes(router)
	ts := httptest.NewServer(router)
	t.Cleanup(ts.Close)
	return backend, ts
}

func postMCPWithToken(t *testing.T, url, body, sessionID, token string) httpResp {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	return httpResp{
		statusCode: resp.StatusCode,
		body:       buf.String(),
		sessionID:  resp.Header.Get("Mcp-Session-Id"),
	}
}

func initializeWorkspaceMCPSession(t *testing.T, url, token string) string {
	t.Helper()
	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}`
	resp := postMCPWithToken(t, url, initReq, "", token)
	require.Equal(t, http.StatusOK, resp.statusCode, resp.body)
	require.NotEmpty(t, resp.sessionID)
	postMCPWithToken(t, url, `{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`, resp.sessionID, token)
	return resp.sessionID
}

func TestExternalWorkspaceToolCallerIdentity(t *testing.T) {
	t.Run("streamable HTTP resolves the caller per request", func(t *testing.T) {
		auth := newWorkspaceMCPAuth()
		backend, ts := newWorkspaceIdentityServer(t, auth)
		sessionID := initializeWorkspaceMCPSession(t, ts.URL+"/mcp", workspaceMCPToken)

		list := postMCPWithToken(t, ts.URL+"/mcp",
			`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`, sessionID, workspaceMCPOtherToken)
		require.Equal(t, http.StatusOK, list.statusCode, list.body)
		listLine := extractJSONRPCIDLine(t, list.body, 2)
		require.Contains(t, listLine, workspaceMCPToolName)
		require.Contains(t, listLine, `"kandev_workspace_id"`)

		callReq := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"` + workspaceMCPToolName +
			`","arguments":{"kandev_workspace_id":"ws-1","key":"value"}}}`
		call := postMCPWithToken(t, ts.URL+"/mcp", callReq, sessionID, workspaceMCPOtherToken)
		require.Equal(t, http.StatusOK, call.statusCode, call.body)
		require.Contains(t, extractJSONRPCIDLine(t, call.body, 3), "workspace data")

		identities, found, calls := backend.recordedCalls()
		require.Len(t, calls, 1)
		require.True(t, found[0])
		require.Equal(t, "user-2", identities[0].UserID)
		arguments, ok := calls[0]["arguments"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "ws-1", arguments["kandev_workspace_id"])
		require.Equal(t, "value", arguments["key"])

		auth.revoke(workspaceMCPOtherToken)
		revoked := postMCPWithToken(t, ts.URL+"/mcp", callReq, sessionID, workspaceMCPOtherToken)
		require.Equal(t, http.StatusUnauthorized, revoked.statusCode)
		_, _, calls = backend.recordedCalls()
		require.Len(t, calls, 1, "a revoked token must not reach the plugin handler")
	})

	t.Run("SSE resolves the caller from the message request", func(t *testing.T) {
		auth := newWorkspaceMCPAuth()
		backend, ts := newWorkspaceIdentityServer(t, auth)

		streamCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		sseReq, err := http.NewRequestWithContext(streamCtx, http.MethodGet, ts.URL+"/mcp/sse", nil)
		require.NoError(t, err)
		sseReq.Header.Set("Accept", "text/event-stream")
		sseReq.Header.Set("Authorization", "Bearer "+workspaceMCPToken)
		sseResp, err := http.DefaultClient.Do(sseReq)
		require.NoError(t, err)
		defer func() { _ = sseResp.Body.Close() }()

		lines := make(chan string, 64)
		go func() {
			scanner := bufio.NewScanner(sseResp.Body)
			for scanner.Scan() {
				lines <- scanner.Text()
			}
			close(lines)
		}()
		endpoint := nextSSEData(t, lines, func(value string) bool {
			return strings.HasPrefix(value, "/mcp/message?sessionId=")
		})
		messageURL := ts.URL + endpoint

		postSSEMessageWithToken(t, messageURL, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}`, workspaceMCPToken)
		nextSSEData(t, lines, func(value string) bool { return strings.Contains(value, `"id":1`) })

		postSSEMessageWithToken(t, messageURL, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`, workspaceMCPToken)
		listLine := nextSSEData(t, lines, func(value string) bool { return strings.Contains(value, `"id":2`) })
		require.Contains(t, listLine, workspaceMCPToolName)

		callReq := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"` + workspaceMCPToolName +
			`","arguments":{"kandev_workspace_id":"ws-1"}}}`
		postSSEMessageWithToken(t, messageURL, callReq, workspaceMCPOtherToken)
		callLine := nextSSEData(t, lines, func(value string) bool { return strings.Contains(value, `"id":3`) })
		require.Contains(t, callLine, "workspace data")

		identities, found, calls := backend.recordedCalls()
		require.Len(t, calls, 1)
		require.True(t, found[0])
		require.Equal(t, "user-2", identities[0].UserID,
			"the message request identity must reach the tool handler")

		auth.revoke(workspaceMCPOtherToken)
		status := postSSEMessageStatus(t, messageURL, callReq, workspaceMCPOtherToken)
		require.Equal(t, http.StatusUnauthorized, status)
		_, _, calls = backend.recordedCalls()
		require.Len(t, calls, 1, "a revoked token must not reach the plugin handler")
	})

	t.Run("disabled authentication accepts the call without an identity", func(t *testing.T) {
		auth := newWorkspaceMCPAuth()
		auth.disable()
		backend, ts := newWorkspaceIdentityServer(t, auth)
		sessionID := initializeWorkspaceMCPSession(t, ts.URL+"/mcp", "")

		list := postMCPWithToken(t, ts.URL+"/mcp",
			`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`, sessionID, "")
		require.Equal(t, http.StatusOK, list.statusCode, list.body)
		require.Contains(t, extractJSONRPCIDLine(t, list.body, 1), workspaceMCPToolName)

		callReq := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"` + workspaceMCPToolName +
			`","arguments":{"kandev_workspace_id":"ws-1"}}}`
		call := postMCPWithToken(t, ts.URL+"/mcp", callReq, sessionID, "")
		require.Equal(t, http.StatusOK, call.statusCode, call.body)
		require.Contains(t, extractJSONRPCIDLine(t, call.body, 2), "workspace data")

		identities, found, calls := backend.recordedCalls()
		require.Len(t, calls, 1)
		require.True(t, found[0])
		require.True(t, identities[0].Synthetic)
	})
}

func postSSEMessageWithToken(t *testing.T, url, body, token string) {
	t.Helper()
	require.Equal(t, http.StatusAccepted, postSSEMessageStatus(t, url, body, token))
}

func postSSEMessageStatus(t *testing.T, url, body, token string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}
