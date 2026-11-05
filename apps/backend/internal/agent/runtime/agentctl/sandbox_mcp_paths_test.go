package client

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/common/sandboxmcp"
	"github.com/stretchr/testify/require"
)

func TestSandboxMCPForwardPreservesPackagePathAndCapturedAuthority(t *testing.T) {
	type receivedRequest struct{ path, auth, route, attempt string }
	received := make(chan receivedRequest, 2)
	var calls atomic.Int32
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		received <- receivedRequest{path: request.URL.Path, auth: request.Header.Get("Authorization"),
			route: request.Header.Get(sandboxmcp.MCPPathHeader), attempt: request.Header.Get("X-Kandev-Sandbox-Attempt")}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(peer.Close)
	client := newHTTPOnlyClient(peer.URL)
	client.httpClient.Transport = &authTransport{token: "control"}
	forward := client.SandboxMCPForward("attempt", 4, strings.Repeat("a", 64))
	for _, path := range []string{"", "/mcp/packages/rich-output"} {
		headers := make(http.Header)
		headers.Set(sandboxmcp.MCPPathHeader, path)
		headers.Set("Authorization", "untrusted")
		headers.Set("X-Kandev-Sandbox-Attempt", "foreign")
		response, err := forward(t.Context(), http.MethodPost, headers, []byte(`{}`))
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		request := <-received
		expected := "/api/v1/agent/sandbox/mcp"
		if path != "" {
			expected += "/packages/rich-output"
		}
		require.Equal(t, expected, request.path)
		require.Empty(t, request.route, "the relay routing marker must not enter the application request")
		require.Equal(t, "Bearer control", request.auth)
		require.Equal(t, "attempt", request.attempt)
	}
	for _, path := range []string{"/api/v1/settings", "/mcp/packages/../configuration", "/mcp/packages/%72ich-output", "/mcp/packages/rich-output?other=1"} {
		headers := make(http.Header)
		headers.Set(sandboxmcp.MCPPathHeader, path)
		_, err := forward(t.Context(), http.MethodPost, headers, []byte(`{}`))
		require.ErrorIs(t, err, ErrSandboxLaunchUnsupported)
	}
	require.EqualValues(t, 2, calls.Load(), "invalid routing must not make a control request")
}
