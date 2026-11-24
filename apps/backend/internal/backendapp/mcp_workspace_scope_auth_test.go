package backendapp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/auth/httpmw"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/mcp/plugintools"
	mcpserver "github.com/kandev/kandev/internal/mcp/server"
	userstore "github.com/kandev/kandev/internal/user/store"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type workspaceScopeAuthBackend struct {
	mu             sync.Mutex
	callIdentities []authn.Identity
	calls          []map[string]any
}

func (b *workspaceScopeAuthBackend) RequestPayload(ctx context.Context, action string, payload, result interface{}) error {
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
			InputSchema:     []byte(`{"type":"object","properties":{"kandev_workspace_id":{"type":"string","minLength":1}},"required":["kandev_workspace_id"]}`),
			ReadOnlyHint:    true,
		}
		return decodeWorkspaceScopePayload(plugintools.Snapshot{Generation: "workspace", Revision: 1,
			Tools: []plugintools.Definition{tool}}, result)
	case ws.ActionMCPInvokePluginTool:
		identity, ok := authn.IdentityFromContext(ctx)
		if !ok {
			return nil
		}
		values, _ := payload.(map[string]any)
		b.mu.Lock()
		b.callIdentities = append(b.callIdentities, identity)
		b.calls = append(b.calls, values)
		b.mu.Unlock()
		return decodeWorkspaceScopePayload(map[string]any{"text": "workspace data", "is_error": false}, result)
	}
	return nil
}

func (b *workspaceScopeAuthBackend) recorded() ([]authn.Identity, []map[string]any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]authn.Identity(nil), b.callIdentities...), append([]map[string]any(nil), b.calls...)
}

func decodeWorkspaceScopePayload(value, result interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, result)
}

func registerWorkspaceScopeDispatcher(dispatcher *ws.Dispatcher, backend *workspaceScopeAuthBackend) {
	dispatcher.RegisterFunc(ws.ActionMCPListPluginTools, func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var result plugintools.Snapshot
		if err := backend.RequestPayload(ctx, msg.Action, map[string]string{"surface": "external"}, &result); err != nil {
			return nil, err
		}
		return ws.NewResponse(msg.ID, msg.Action, result)
	})
	dispatcher.RegisterFunc(ws.ActionMCPInvokePluginTool, func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var payload map[string]any
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			return nil, err
		}
		var result map[string]any
		if err := backend.RequestPayload(ctx, msg.Action, payload, &result); err != nil {
			return nil, err
		}
		return ws.NewResponse(msg.ID, msg.Action, result)
	})
}

const workspaceScopeToolName = "kandev_kandev_workspace_reader_read_workspace_data"

func TestExternalWorkspaceToolPATIdentityHTTPAndSSE(t *testing.T) {
	cfg := &config.Config{}
	cfg.Features.Auth = true
	authService := newEnabledAuthService(t, cfg)
	apiToken, token, err := authService.MintToken(context.Background(), userstore.DefaultUserID, "workspace-scope", 0)
	require.NoError(t, err)

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)
	backend := &workspaceScopeAuthBackend{}
	dispatcher := ws.NewDispatcher()
	registerWorkspaceScopeDispatcher(dispatcher, backend)
	srv := mcpserver.NewExternal(mcpserver.NewExternalDispatcherBackendClient(dispatcher, log), log, "")
	t.Cleanup(func() { require.NoError(t, srv.Close(context.Background())) })

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpmw.Middleware(authService), externalMCPAuthMiddleware(authService))
	srv.RegisterBackendRoutes(router)
	httpServer := httptest.NewServer(router)
	defer httpServer.Close()

	callBody := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"` + workspaceScopeToolName +
		`","arguments":{"kandev_workspace_id":"ws-1"}}}`
	sessionID := initializeWorkspaceScopeHTTPSession(t, httpServer.URL+"/mcp", token)
	list := postWorkspaceScopeRequest(t, httpServer.URL+"/mcp",
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`, sessionID, token)
	require.Equal(t, http.StatusOK, list.statusCode, list.body)
	require.Contains(t, list.body, workspaceScopeToolName)
	call := postWorkspaceScopeRequest(t, httpServer.URL+"/mcp", callBody, sessionID, token)
	require.Equal(t, http.StatusOK, call.statusCode, call.body)
	require.Contains(t, call.body, "workspace data")

	streamCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sseReq, err := http.NewRequestWithContext(streamCtx, http.MethodGet, httpServer.URL+"/mcp/sse", nil)
	require.NoError(t, err)
	sseReq.Header.Set("Accept", "text/event-stream")
	sseReq.Header.Set("Authorization", "Bearer "+token)
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
	endpoint := nextWorkspaceScopeSSEData(t, lines, func(value string) bool {
		return strings.HasPrefix(value, "/mcp/message?sessionId=")
	})
	messageURL := httpServer.URL + endpoint
	postWorkspaceScopeSSEMessage(t, messageURL, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}`, token)
	nextWorkspaceScopeSSEData(t, lines, func(value string) bool { return strings.Contains(value, `"id":1`) })
	postWorkspaceScopeSSEMessage(t, messageURL, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`, token)
	nextWorkspaceScopeSSEData(t, lines, func(value string) bool { return strings.Contains(value, `"id":2`) })
	postWorkspaceScopeSSEMessage(t, messageURL, callBody, token)
	nextWorkspaceScopeSSEData(t, lines, func(value string) bool { return strings.Contains(value, `"id":3`) })

	identities, calls := backend.recorded()
	require.Len(t, calls, 2, "both transports must reach the backend")
	require.Equal(t, userstore.DefaultUserID, identities[0].UserID)
	require.Equal(t, userstore.DefaultUserID, identities[1].UserID)
	arguments, ok := calls[0]["arguments"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "ws-1", arguments["kandev_workspace_id"])

	require.NoError(t, authService.RevokeToken(context.Background(), apiToken.ID, userstore.DefaultUserID))
	revoked := postWorkspaceScopeRequest(t, httpServer.URL+"/mcp", callBody, sessionID, token)
	require.Equal(t, http.StatusUnauthorized, revoked.statusCode)
	revokedSSE := postWorkspaceScopeSSEMessageStatus(t, messageURL, callBody, token)
	require.Equal(t, http.StatusUnauthorized, revokedSSE)
	_, calls = backend.recorded()
	require.Len(t, calls, 2, "a revoked token must not reach the backend")
}

type workspaceScopeHTTPResponse struct {
	statusCode int
	body       string
	sessionID  string
}

func postWorkspaceScopeRequest(t *testing.T, url, body, sessionID, token string) workspaceScopeHTTPResponse {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	return workspaceScopeHTTPResponse{
		statusCode: resp.StatusCode,
		body:       buf.String(),
		sessionID:  resp.Header.Get("Mcp-Session-Id"),
	}
}

func initializeWorkspaceScopeHTTPSession(t *testing.T, url, token string) string {
	t.Helper()
	resp := postWorkspaceScopeRequest(t, url,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}`, "", token)
	require.Equal(t, http.StatusOK, resp.statusCode, resp.body)
	require.NotEmpty(t, resp.sessionID)
	postWorkspaceScopeRequest(t, url, `{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`, resp.sessionID, token)
	return resp.sessionID
}

func postWorkspaceScopeSSEMessage(t *testing.T, url, body, token string) {
	t.Helper()
	require.Equal(t, http.StatusAccepted, postWorkspaceScopeSSEMessageStatus(t, url, body, token))
}

func postWorkspaceScopeSSEMessageStatus(t *testing.T, url, body, token string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}

func nextWorkspaceScopeSSEData(t *testing.T, lines <-chan string, match func(string) bool) string {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				t.Fatal("SSE stream closed before a matching data line")
			}
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			value := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if match(value) {
				return value
			}
		case <-timer.C:
			t.Fatal("timed out waiting for an SSE data line")
		}
	}
}
