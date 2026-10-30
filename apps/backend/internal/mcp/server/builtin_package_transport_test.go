package mcp

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestBuiltinPackagePreservesModernNotificationCompatibility(t *testing.T) {
	s := NewWithProfile(&testBackend{}, "session", "task", 10005, newTestLogger(t), "", false, packageTestProfile(mcpprofile.PackageCore, mcpprofile.PackageRichOutput))
	t.Cleanup(func() { require.NoError(t, s.Close(context.Background())) })
	router := gin.New()
	s.RegisterRoutes(router)
	for _, path := range []string{"/mcp", "/mcp/packages/rich-output"} {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/roots/list_changed"}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json, text/event-stream")
		request.Header.Set(protocolVersionHeader, modernProtocolVersion)
		request.Header.Set(methodHeader, rootsListChanged)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		require.Equal(t, http.StatusAccepted, response.Code, "%s: %s", path, response.Body.String())
	}
}

// @covers AC-AGENTS-BUILTIN-MCP-002.4
func TestBuiltinSSEPackageRouting(t *testing.T) {
	router := gin.New()
	host := httptest.NewUnstartedServer(router)
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	s := NewWithProfile(backend, "session", "task", 10005, log, "", false, packageTestProfile(mcpprofile.PackageCore, mcpprofile.PackageRichOutput, mcpprofile.PackageDiagnostics), WithSSEBaseURL("http://"+host.Listener.Addr().String()))
	t.Cleanup(func() { require.NoError(t, s.Close(context.Background())) })
	s.RegisterRoutes(router)
	host.Start()
	t.Cleanup(host.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, host.URL+"/sse/packages/rich-output", nil)
	require.NoError(t, err)
	response, err := host.Client().Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, response.Body.Close()) }()
	reader := bufio.NewReader(response.Body)
	_, err = reader.ReadString('\n')
	require.NoError(t, err)
	data, err := reader.ReadString('\n')
	require.NoError(t, err)
	endpoint, err := url.Parse(strings.TrimSpace(strings.TrimPrefix(data, "data: ")))
	require.NoError(t, err)
	require.Equal(t, "/message/packages/rich-output", endpoint.Path)
	require.NotEmpty(t, endpoint.Query().Get("sessionId"))
	for _, path := range []string{"/message/packages/rich-output", "/message/packages/diagnostics", "/message"} {
		target := *endpoint
		target.Path = path
		body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`
		post, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), strings.NewReader(body))
		require.NoError(t, err)
		post.Header.Set("Content-Type", "application/json")
		reply, err := host.Client().Do(post)
		require.NoError(t, err)
		require.NoError(t, reply.Body.Close())
		if path == endpoint.Path {
			require.Equal(t, http.StatusAccepted, reply.StatusCode)
		} else {
			require.Equal(t, http.StatusBadRequest, reply.StatusCode, path)
		}
	}
}
