package kubernetes

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	gwebsocket "github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/httpstream"
	streamspdy "k8s.io/apimachinery/pkg/util/httpstream/spdy"
	portforwardconstants "k8s.io/apimachinery/pkg/util/portforward"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
)

func TestPortForwardDialerBoundsBlockedWebSocketPrimaryAndUsesPOSTFallback(t *testing.T) {
	getStarted := make(chan struct{})
	postStarted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodGet:
			close(getStarted)
			<-request.Context().Done()
		case http.MethodPost:
			close(postStarted)
			http.Error(response, "SPDY rejected", http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)
	target, err := url.Parse(server.URL + "/api/v1/namespaces/agents/pods/agent/portforward")
	require.NoError(t, err)
	dialer, cancel, err := NewPortForwardDialer(
		context.Background(), &rest.Config{Host: server.URL}, target, 40*time.Millisecond,
	)
	require.NoError(t, err)
	t.Cleanup(cancel)

	_, _, err = dialer.Dial(portforward.PortForwardProtocolV1Name)

	require.Error(t, err)
	requireClosed(t, getStarted, "WebSocket GET primary was not attempted")
	requireClosed(t, postStarted, "SPDY POST fallback was not attempted")
}

func TestPortForwardDialerCancelClosesBlockedWebSocketResponseRead(t *testing.T) {
	getStarted := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			http.Error(response, "fallback unavailable", http.StatusBadRequest)
			return
		}
		close(getStarted)
		<-release
	}))
	t.Cleanup(func() {
		close(release)
		server.Close()
	})
	target, err := url.Parse(server.URL + "/api/v1/namespaces/agents/pods/agent/portforward")
	require.NoError(t, err)
	ctx, cancelContext := context.WithCancel(context.Background())
	dialer, cancelDialer, err := NewPortForwardDialer(
		ctx, &rest.Config{Host: server.URL}, target, 5*time.Second,
	)
	require.NoError(t, err)
	result := make(chan error, 1)
	go func() {
		_, _, dialErr := dialer.Dial(portforward.PortForwardProtocolV1Name)
		result <- dialErr
	}()
	requireClosed(t, getStarted, "WebSocket GET primary was not attempted")

	cancelContext()
	cancelDialer()

	select {
	case err := <-result:
		require.Error(t, err)
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(250 * time.Millisecond):
		t.Fatal("blocked WebSocket response-header read ignored cancellation")
	}
}

func TestContextWebSocketDialerUsesConfiguredRESTDial(t *testing.T) {
	target, err := url.Parse("http://127.0.0.1:1/api/v1/namespaces/agents/pods/agent/portforward")
	require.NoError(t, err)
	dialErr := errors.New("configured REST dial invoked")
	dialer, err := newContextWebSocketDialer(target, &rest.Config{
		Host: target.Scheme + "://" + target.Host,
		Dial: func(context.Context, string, string) (net.Conn, error) {
			return nil, dialErr
		},
	})
	require.NoError(t, err)

	_, _, err = dialer.Dial(portforward.PortForwardProtocolV1Name)

	require.ErrorIs(t, err, dialErr)
}

func TestDetachableHandshakeConnRejectsDetachAfterCancellation(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = server.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	guarded := newDetachableHandshakeConn(ctx, client)
	cancel()

	err := guarded.detach()

	require.ErrorIs(t, err, context.Canceled)
	_, writeErr := guarded.Write([]byte("closed"))
	require.Error(t, writeErr, "cancellation that wins detachment must close the raw connection")
}

func TestPortForwardDialerCancelBoundsBlockedSPDYFallback(t *testing.T) {
	postStarted := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			http.Error(response, "upgrade rejected", http.StatusBadRequest)
			return
		}
		close(postStarted)
		select {
		case <-request.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(func() {
		close(release)
		server.Close()
	})
	target, err := url.Parse(server.URL + "/api/v1/namespaces/agents/pods/agent/portforward")
	require.NoError(t, err)
	ctx, cancelContext := context.WithCancel(context.Background())
	dialer, cancelDialer, err := NewPortForwardDialer(
		ctx, &rest.Config{Host: server.URL}, target, time.Second,
	)
	require.NoError(t, err)
	result := make(chan error, 1)
	go func() {
		_, _, dialErr := dialer.Dial(portforward.PortForwardProtocolV1Name)
		result <- dialErr
	}()
	requireClosed(t, postStarted, "SPDY POST fallback was not attempted")
	cancelContext()
	cancelDialer()

	select {
	case err := <-result:
		require.Error(t, err)
	case <-time.After(250 * time.Millisecond):
		t.Fatal("cancel left the SPDY fallback handshake goroutine blocked")
	}
}

func TestPortForwardSPDYFallbackSurvivesHandshakeContextAfterUpgrade(t *testing.T) {
	serverConnection := make(chan httpstream.Connection, 1)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			http.Error(response, "WebSocket unavailable", http.StatusBadRequest)
			return
		}
		protocol := request.Header.Get(httpstream.HeaderProtocolVersion)
		response.Header().Set(httpstream.HeaderProtocolVersion, protocol)
		connection := streamspdy.NewResponseUpgrader().UpgradeResponse(
			response, request, func(httpstream.Stream, <-chan struct{}) error { return nil },
		)
		serverConnection <- connection
	}))
	t.Cleanup(server.Close)
	target, err := url.Parse(server.URL + "/api/v1/namespaces/agents/pods/agent/portforward")
	require.NoError(t, err)
	handshakeTimeout := 25 * time.Millisecond
	dialer, cancel, err := NewPortForwardDialer(
		context.Background(), &rest.Config{Host: server.URL}, target, handshakeTimeout,
	)
	require.NoError(t, err)

	connection, protocol, err := dialer.Dial(portforward.PortForwardProtocolV1Name)

	require.NoError(t, err)
	require.Equal(t, portforward.PortForwardProtocolV1Name, protocol)
	time.Sleep(2 * handshakeTimeout)
	cancel()
	select {
	case <-connection.CloseChan():
		t.Fatal("successful SPDY fallback remained coupled to its handshake context")
	case <-time.After(50 * time.Millisecond):
	}
	require.NoError(t, connection.Close())
	select {
	case peer := <-serverConnection:
		if peer != nil {
			_ = peer.Close()
		}
	case <-time.After(time.Second):
		t.Fatal("SPDY server did not complete upgrade")
	}
}

// Reviewer-requested contract coverage: successful WebSocket upgrade must be
// detached from both handshake timer and returned dialer cancellation.
func TestPortForwardWebSocketPrimarySurvivesHandshakeContextAfterUpgrade(t *testing.T) {
	serverStreams := make(chan httpstream.Stream, 1)
	serverErrors := make(chan error, 1)
	stopServer := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		upgrader := gwebsocket.Upgrader{
			CheckOrigin:  func(*http.Request) bool { return true },
			Subprotocols: []string{portforwardconstants.WebsocketsSPDYTunnelingPortForwardV1},
		}
		websocketConnection, err := upgrader.Upgrade(response, request, nil)
		if err != nil {
			serverErrors <- err
			return
		}
		tunnel := portforward.NewTunnelingConnection("server", websocketConnection)
		serverConnection, err := streamspdy.NewServerConnection(
			tunnel, func(stream httpstream.Stream, _ <-chan struct{}) error {
				serverStreams <- stream
				return nil
			},
		)
		if err != nil {
			serverErrors <- err
			return
		}
		defer serverConnection.Close() //nolint:errcheck
		<-stopServer
	}))
	t.Cleanup(func() {
		close(stopServer)
		server.Close()
	})
	target, err := url.Parse(server.URL + "/api/v1/namespaces/agents/pods/agent/portforward")
	require.NoError(t, err)
	handshakeTimeout := 25 * time.Millisecond
	dialer, cancel, err := NewPortForwardDialer(
		context.Background(), &rest.Config{Host: server.URL}, target, handshakeTimeout,
	)
	require.NoError(t, err)

	connection, protocol, err := dialer.Dial(portforward.PortForwardProtocolV1Name)
	require.NoError(t, err)
	require.Equal(t, portforward.PortForwardProtocolV1Name, protocol)
	time.Sleep(2 * handshakeTimeout)
	cancel()
	select {
	case <-connection.CloseChan():
		t.Fatal("successful WebSocket primary remained coupled to handshake context")
	default:
	}

	stream, err := connection.CreateStream(http.Header{})
	require.NoError(t, err)
	_, err = io.Copy(stream, strings.NewReader("still-alive"))
	require.NoError(t, err)
	require.NoError(t, stream.Close())
	select {
	case serverStream := <-serverStreams:
		payload, readErr := io.ReadAll(serverStream)
		require.NoError(t, readErr)
		require.Equal(t, "still-alive", string(payload))
		_ = serverStream.Close()
	case serverErr := <-serverErrors:
		t.Fatalf("WebSocket SPDY server failed: %v", serverErr)
	case <-time.After(time.Second):
		t.Fatal("detached WebSocket connection was not usable")
	}
	require.NoError(t, connection.Close())
}

func TestDetachablePortForwardContextPreservesDeadlineError(t *testing.T) {
	ctx := newDetachablePortForwardContext(context.Background(), 10*time.Millisecond)

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("detachable context deadline did not fire")
	}
	require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
}

func TestContextSPDYDialerClosesUpgradeStreamWhenConnectionInitFails(t *testing.T) {
	stream := &trackingUpgradedStream{}
	target, err := url.Parse("https://cluster.example/api/v1/namespaces/agents/pods/agent/portforward")
	require.NoError(t, err)
	dialer := &contextSPDYDialer{
		targetURL: target,
		client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusSwitchingProtocols,
				Status:     "101 Switching Protocols",
				Header: http.Header{
					httpstream.HeaderConnection:      []string{httpstream.HeaderUpgrade},
					httpstream.HeaderUpgrade:         []string{streamspdy.HeaderSpdy31},
					httpstream.HeaderProtocolVersion: []string{portforward.PortForwardProtocolV1Name},
				},
				Body: stream,
			}, nil
		})},
		newConnection: func(net.Conn, time.Duration) (httpstream.Connection, error) {
			return nil, errors.New("SPDY init failed")
		},
	}

	_, _, err = dialer.Dial(portforward.PortForwardProtocolV1Name)

	require.ErrorContains(t, err, "SPDY init failed")
	require.True(t, stream.closed, "failed SPDY initialization must close upgraded response stream")
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type trackingUpgradedStream struct {
	closed bool
}

func (*trackingUpgradedStream) Read([]byte) (int, error)       { return 0, io.EOF }
func (*trackingUpgradedStream) Write(data []byte) (int, error) { return len(data), nil }
func (s *trackingUpgradedStream) Close() error {
	s.closed = true
	return nil
}

func requireClosed(t *testing.T, channel <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-channel:
	case <-time.After(time.Second):
		t.Fatal(message)
	}
}
