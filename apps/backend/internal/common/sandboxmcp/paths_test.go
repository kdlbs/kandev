package sandboxmcp

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSandboxMCPRelayPreservesSelectedPackageRoutes(t *testing.T) {
	paths := []string{"/mcp", "/mcp/packages/rich-output"}
	var calls atomic.Int32
	received := make(chan http.Header, 2)
	relay, err := OpenWithPaths(t.Context(), func(_ context.Context, _ string, headers http.Header, _ []byte) (*http.Response, error) {
		calls.Add(1)
		received <- headers.Clone()
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	}, paths)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, relay.Close()) })
	paths[1] = "/mcp/packages/configuration"
	for _, test := range []struct {
		suffix string
		path   string
	}{
		{"", ""},
		{"/packages/rich-output", "/mcp/packages/rich-output"},
	} {
		request, requestErr := http.NewRequest(http.MethodPost, relay.URL()+test.suffix, strings.NewReader(`{}`))
		require.NoError(t, requestErr)
		request.Header.Set(MCPPathHeader, "/mcp/packages/configuration")
		request.Header.Set("Authorization", "untrusted")
		response, responseErr := http.DefaultClient.Do(request)
		require.NoError(t, responseErr)
		require.Equal(t, http.StatusOK, response.StatusCode)
		require.NoError(t, response.Body.Close())
		headers := <-received
		require.Equal(t, test.path, headers.Get(MCPPathHeader))
		require.Empty(t, headers.Get("Authorization"))
	}
	for _, suffix := range []string{"/packages/configuration", "/packages/rich-output/child", "/packages/%72ich-output", "/packages/rich-output?other=1"} {
		response, responseErr := http.Post(relay.URL()+suffix, "application/json", strings.NewReader(`{}`))
		require.NoError(t, responseErr)
		require.Equal(t, http.StatusForbidden, response.StatusCode)
		require.NoError(t, response.Body.Close())
	}
	require.EqualValues(t, 2, calls.Load(), "unselected routes must never reach the forwarder")
}
