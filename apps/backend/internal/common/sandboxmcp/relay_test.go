package sandboxmcp

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSandboxMCPRelayRestrictsRouteAndHeaders(t *testing.T) {
	var calls atomic.Int32
	relay, err := Open(t.Context(), func(ctx context.Context, method string, headers http.Header, body []byte) (*http.Response, error) {
		calls.Add(1)
		require.Equal(t, http.MethodPost, method)
		require.Equal(t, "application/json", headers.Get("Content-Type"))
		require.Empty(t, headers.Get("Authorization"))
		require.Empty(t, headers.Get("X-Session-ID"))
		require.JSONEq(t, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, string(body))
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}, "Set-Cookie": {"private"}}, Body: io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`))}, nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, relay.Close()) })
	for _, path := range []string{"/api/v1/settings", "/mcp", "/other/mcp", relay.path + "?session=other", strings.ReplaceAll(relay.path, "/mcp", "/%6dcp")} {
		request, e := http.NewRequest(http.MethodPost, "http://"+relay.Address()+path, strings.NewReader(`{}`))
		require.NoError(t, e)
		response, e := http.DefaultClient.Do(request)
		require.NoError(t, e)
		require.Equal(t, http.StatusForbidden, response.StatusCode)
		require.NoError(t, response.Body.Close())
	}
	request, err := http.NewRequest(http.MethodPost, relay.URL(), strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer untrusted")
	request.Header.Set("X-Session-ID", "other")
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	require.Equal(t, 200, response.StatusCode)
	require.True(t, response.Close, "each MCP request uses a new attempt-scoped connection")
	require.Empty(t, response.Header.Get("Set-Cookie"))
	require.NoError(t, response.Body.Close())
	require.EqualValues(t, 1, calls.Load())
}

func TestSandboxMCPRelayRejectsRedirectAndOversize(t *testing.T) {
	var calls atomic.Int32
	relay, err := Open(t.Context(), func(context.Context, string, http.Header, []byte) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": {"http://forbidden.invalid"}}, Body: io.NopCloser(strings.NewReader("private-upstream"))}, nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, relay.Close()) })
	response, err := http.Post(relay.URL(), "application/json", strings.NewReader(strings.Repeat("x", (1<<20)+1)))
	require.NoError(t, err)
	require.Equal(t, http.StatusRequestEntityTooLarge, response.StatusCode)
	require.NoError(t, response.Body.Close())
	require.Zero(t, calls.Load())
	response, err = http.Post(relay.URL(), "application/json", strings.NewReader(`{}`))
	require.NoError(t, err)
	require.Equal(t, http.StatusBadGateway, response.StatusCode)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NotContains(t, string(body), "private-upstream")
	require.NoError(t, response.Body.Close())
}

func TestSandboxMCPRelayCancellationRevokesPendingRequest(t *testing.T) {
	entered := make(chan struct{})
	cancelled := make(chan struct{})
	relay, err := Open(t.Context(), func(ctx context.Context, _ string, _ http.Header, _ []byte) (*http.Response, error) {
		close(entered)
		<-ctx.Done()
		close(cancelled)
		return nil, ctx.Err()
	})
	require.NoError(t, err)
	done := make(chan struct{})
	go func() {
		defer close(done)
		response, e := http.Post(relay.URL(), "application/json", strings.NewReader(`{}`))
		if e == nil {
			_ = response.Body.Close()
		}
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("upstream not called")
	}
	require.NoError(t, relay.Close())
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("upstream not cancelled")
	}
	<-done
	_, err = http.Post(relay.URL(), "application/json", strings.NewReader(`{}`))
	require.Error(t, err)
}
