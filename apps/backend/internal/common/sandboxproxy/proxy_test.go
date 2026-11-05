package sandboxproxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/sandbox"
	"github.com/stretchr/testify/require"
)

func TestSandboxEgressProxyEnforcesExactHostAndPort(t *testing.T) {
	allowed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "allowed") }))
	defer allowed.Close()
	denied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("denied destination received traffic") }))
	defer denied.Close()
	target, err := url.Parse(allowed.URL)
	require.NoError(t, err)
	forbidden, err := url.Parse(denied.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(target.Port())
	require.NoError(t, err)
	proxy, err := Open(t.Context(), []sandbox.NetworkGrant{{Host: target.Hostname(), Port: uint16(port), Protocol: "tcp"}})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, proxy.Close()) })
	for _, authority := range []string{forbidden.Host, "localhost:" + target.Port(), target.Host + "@evil", target.Host + "/path"} {
		conn, e := net.DialTimeout("tcp", proxy.Address(), time.Second)
		require.NoError(t, e)
		_, e = fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", authority, authority)
		require.NoError(t, e)
		response, e := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
		require.NoError(t, e)
		require.NotEqual(t, http.StatusOK, response.StatusCode)
		require.NoError(t, conn.Close())
	}
	conn, err := net.DialTimeout("tcp", proxy.Address(), time.Second)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	_, err = fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target.Host, target.Host)
	require.NoError(t, err)
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, &http.Request{Method: "CONNECT"})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	_, err = fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", target.Host)
	require.NoError(t, err)
	response, err = http.ReadResponse(reader, &http.Request{Method: "GET"})
	require.NoError(t, err)
	content, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, "allowed", string(content))
	require.NoError(t, proxy.Close())
	_, err = net.DialTimeout("tcp", proxy.Address(), time.Second)
	require.Error(t, err)
}

func TestSandboxEgressProxyCancellationClosesTunnels(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()
	host, portText, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	proxy, err := Open(ctx, []sandbox.NetworkGrant{{Host: host, Port: uint16(port), Protocol: "tcp"}})
	require.NoError(t, err)
	defer func() { _ = proxy.Close() }()
	conn, err := net.DialTimeout("tcp", proxy.Address(), time.Second)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	_, err = fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", listener.Addr(), listener.Addr())
	require.NoError(t, err)
	upstream, err := listener.Accept()
	require.NoError(t, err)
	defer func() { _ = upstream.Close() }()
	response, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
	require.NoError(t, err)
	require.Equal(t, 200, response.StatusCode)
	cancel()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
	_, err = conn.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.EOF)
}

func TestProxyRejectsRuntimeControlPortsAcrossHostAliases(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "localhost", "control.example.com", "192.0.2.10"} {
		proxy, err := Open(t.Context(), []sandbox.NetworkGrant{{Host: host, Port: 32123, Protocol: "tcp"}}, 32123)
		if proxy != nil {
			_ = proxy.Close()
		}
		require.ErrorIs(t, err, sandbox.ErrInvalidPolicy)
	}
	proxy, err := Open(t.Context(), []sandbox.NetworkGrant{{Host: "127.0.0.1", Port: 32124, Protocol: "tcp"}}, 32123)
	require.NoError(t, err)
	require.NoError(t, proxy.Close())
}
