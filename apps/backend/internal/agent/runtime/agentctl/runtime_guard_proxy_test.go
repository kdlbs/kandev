package client

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestRuntimeBoundProxyPreservesWebsocketUpgrade(t *testing.T) {
	upgrader := websocket.Upgrader{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		for {
			kind, message, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if err := conn.WriteMessage(kind, message); err != nil {
				return
			}
		}
	}))
	defer upstream.Close()
	host, port := splitTestServerHostPort(t, upstream)
	owner := NewRuntimeOwner(nil, newTestLogger(), "proxy-upgrade")
	defer owner.Stop()
	candidate, err := owner.PrepareBinding()
	require.NoError(t, err)
	require.NoError(t, candidate.Configure(host, port, "proxy-secret", 1, nil))
	require.NoError(t, candidate.Commit())
	lease, err := owner.Acquire(context.Background())
	require.NoError(t, err)
	client := lease.NewInstanceClient(host, port, newTestLogger())
	lease.Close()
	defer client.Close()
	target, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = client.ProxyTransport()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel, err := client.RuntimeBoundContext(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		defer cancel()
		proxy.ServeHTTP(w, r.WithContext(ctx))
	}))
	defer server.Close()
	conn, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if response != nil {
		defer func() { _ = response.Body.Close() }()
	}
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("workspace echo")))
	_, message, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, "workspace echo", string(message))
	require.True(t, owner.MarkUnavailableEpoch(candidate.Epoch(), AvailabilityReasonAgentctlExited))
	_, _, err = conn.ReadMessage()
	require.Error(t, err, "retiring a runtime must close its upgraded proxy")
}

type proxyDuplexRecorder struct {
	bytes.Buffer
	closed bool
}

func (body *proxyDuplexRecorder) Close() error {
	body.closed = true
	return nil
}

func TestRuntimeBoundDuplexBodyFencesWritesAfterRetirement(t *testing.T) {
	owner := NewRuntimeOwner(nil, newTestLogger(), "proxy-writes")
	defer owner.Stop()
	candidate := configuredCandidate(t, owner, "proxy-secret", nil)
	require.NoError(t, candidate.Commit())
	lease, err := owner.Acquire(context.Background())
	require.NoError(t, err)
	defer lease.Close()
	recorder := &proxyDuplexRecorder{}
	cancelled := false
	body := &runtimeBoundDuplexBody{
		runtimeBoundResponseBody: &runtimeBoundResponseBody{
			ReadCloser: recorder, guard: lease.bindingGuard(), cancel: func() { cancelled = true },
		},
		writer: recorder,
	}
	n, err := body.Write([]byte("before"))
	require.NoError(t, err)
	require.Equal(t, len("before"), n)
	require.True(t, owner.MarkUnavailableEpoch(candidate.Epoch(), AvailabilityReasonAgentctlExited))
	n, err = body.Write([]byte("after"))
	require.ErrorIs(t, err, ErrRuntimeLeaseRetired)
	require.Zero(t, n)
	require.Equal(t, "before", recorder.String())
	require.True(t, recorder.closed)
	require.True(t, cancelled)
}
