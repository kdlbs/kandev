package backendapp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/auth/httpmw"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	mcpserver "github.com/kandev/kandev/internal/mcp/server"
	userstore "github.com/kandev/kandev/internal/user/store"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

const externalModernProtocolVersion = "2026-07-28"

func TestExternalMCPModernRequestsResolvePATIdentityPerRequest(t *testing.T) {
	cfg := &config.Config{}
	cfg.Features.Auth = true
	authService := newEnabledAuthService(t, cfg)
	_, token, err := authService.MintToken(context.Background(), userstore.DefaultUserID, "mcp-compatibility", 0)
	require.NoError(t, err)

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)
	requestIdentities := make(chan authn.Identity, 3)
	backendIdentities := make(chan authn.Identity, 2)
	dispatcher := ws.NewDispatcher()
	dispatcher.RegisterFunc(ws.ActionMCPListPluginTools, func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		identity, ok := authn.IdentityFromContext(ctx)
		if !ok {
			t.Error("plugin refresh dispatcher context has no authenticated identity")
		} else {
			backendIdentities <- identity
		}
		return ws.NewResponse(msg.ID, msg.Action, map[string]any{
			"generation": "compatibility-test",
			"revision":   1,
			"tools":      []any{},
		})
	})
	dispatcher.RegisterFunc(ws.ActionMCPListWorkspaces, func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		identity, ok := authn.IdentityFromContext(ctx)
		if !ok {
			t.Error("backend dispatcher context has no authenticated identity")
		} else {
			backendIdentities <- identity
		}
		return ws.NewResponse(msg.ID, msg.Action, map[string]any{
			"workspaces": []map[string]string{{"id": "ws-1", "name": "Test"}},
			"total":      1,
		})
	})

	srv := mcpserver.NewExternal(
		mcpserver.NewExternalDispatcherBackendClient(dispatcher, log),
		log,
		"",
	)
	t.Cleanup(func() { require.NoError(t, srv.Close(context.Background())) })

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(
		httpmw.Middleware(authService),
		externalMCPAuthMiddleware(authService),
		func(c *gin.Context) {
			identity, ok := authn.FromGin(c)
			if !ok {
				t.Error("external MCP route has no authenticated identity")
			} else {
				requestIdentities <- identity
			}
			c.Next()
		},
	)
	srv.RegisterBackendRoutes(router)

	httpServer := httptest.NewServer(router)
	defer httpServer.Close()

	requests := []map[string]any{
		{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "server/discover",
			"params": map[string]any{
				"_meta": externalModernRequestMeta(),
			},
		},
		{
			"jsonrpc": "2.0",
			"id":      2,
			"method":  "tools/list",
			"params": map[string]any{
				"_meta": externalModernRequestMeta(),
			},
		},
		{
			"jsonrpc": "2.0",
			"id":      3,
			"method":  "tools/call",
			"params": map[string]any{
				"name":      "list_workspaces_kandev",
				"arguments": map[string]any{},
				"_meta":     externalModernRequestMeta(),
			},
		},
	}
	for _, payload := range requests {
		response := postExternalModernMCPRequest(t, httpServer.URL+"/mcp", token, payload)
		require.Equal(t, http.StatusOK, response.StatusCode, "body = %s", response.Body)
		require.Empty(t, response.Header.Get("Mcp-Session-Id"), "modern requests must stay stateless")
		message := decodeExternalMCPResponse(t, response.Body)
		require.NotContains(t, message, "error", "body = %s", response.Body)
		if payload["method"] == "tools/call" {
			require.Contains(t, response.Body, "ws-1")
		}
	}

	for range requests {
		identity := receiveExternalMCPIdentity(t, requestIdentities, "route")
		require.Equal(t, userstore.DefaultUserID, identity.UserID)
		require.NotEmpty(t, identity.TokenID)
		require.False(t, identity.Synthetic)
	}
	for range []int{0, 1} {
		identity := receiveExternalMCPIdentity(t, backendIdentities, "dispatcher")
		require.Equal(t, userstore.DefaultUserID, identity.UserID)
		require.NotEmpty(t, identity.TokenID)
		require.False(t, identity.Synthetic)
	}
}

type externalMCPHTTPResponse struct {
	*http.Response
	Body string
}

func postExternalModernMCPRequest(t *testing.T, url, token string, payload map[string]any) externalMCPHTTPResponse {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Mcp-Protocol-Version", externalModernProtocolVersion)
	request.Header.Set("Mcp-Method", payload["method"].(string))
	if payload["method"] == "tools/call" {
		request.Header.Set("Mcp-Name", "list_workspaces_kandev")
	}
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	var responseBody bytes.Buffer
	_, err = responseBody.ReadFrom(response.Body)
	require.NoError(t, err)
	return externalMCPHTTPResponse{Response: response, Body: responseBody.String()}
}

func externalModernRequestMeta() map[string]any {
	return map[string]any{
		"io.modelcontextprotocol/protocolVersion": externalModernProtocolVersion,
		"io.modelcontextprotocol/clientInfo": map[string]any{
			"name":    "external-compatibility-test",
			"version": "1.0.0",
		},
		"io.modelcontextprotocol/clientCapabilities": map[string]any{},
	}
}

func decodeExternalMCPResponse(t *testing.T, body string) map[string]any {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "data:") {
			body = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			break
		}
	}
	var message map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &message), "body = %q", body)
	return message
}

func receiveExternalMCPIdentity(t *testing.T, identities <-chan authn.Identity, source string) authn.Identity {
	t.Helper()
	select {
	case identity := <-identities:
		return identity
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s identity", source)
		return authn.Identity{}
	}
}
