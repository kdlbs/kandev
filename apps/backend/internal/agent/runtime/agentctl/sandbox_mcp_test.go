package client

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

func TestSandboxMCPForwardCapturesAuthorityAndDoesNotRedirect(t *testing.T) {
	redirected := false
	forbidden := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected = true }))
	defer forbidden.Close()
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/agent/sandbox/mcp", r.URL.Path)
		require.Empty(t, r.URL.RawQuery)
		require.Equal(t, "Bearer control", r.Header.Get("Authorization"))
		require.Equal(t, "attempt", r.Header.Get("X-Kandev-Sandbox-Attempt"))
		require.Equal(t, "4", r.Header.Get("X-Kandev-Sandbox-Generation"))
		require.Equal(t, strings.Repeat("a", 64), r.Header.Get("X-Kandev-Sandbox-Policy"))
		w.Header().Set("Location", forbidden.URL)
		w.WriteHeader(302)
	}))
	defer peer.Close()
	host, portText, err := net.SplitHostPort(peer.Listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	require.NoError(t, err)
	client := NewClient(host, port, log, WithAuthToken("control"))
	forward := client.SandboxMCPForward("attempt", 4, strings.Repeat("a", 64))
	response, err := forward(t.Context(), http.MethodPost, http.Header{"Authorization": {"Bearer attacker"}, "X-Kandev-Sandbox-Attempt": {"other"}}, []byte(`{}`))
	require.NoError(t, err)
	require.Equal(t, 302, response.StatusCode)
	_, err = io.Copy(io.Discard, response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.False(t, redirected)
}
