package client

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// pipeListener hands the server end of each tunnel to an http.Server.
type pipeListener struct {
	conns chan net.Conn
	done  chan struct{}
	once  sync.Once
}

func newPipeListener() *pipeListener {
	return &pipeListener{conns: make(chan net.Conn), done: make(chan struct{})}
}

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.conns:
		return conn, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *pipeListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

func (l *pipeListener) Addr() net.Addr { return &net.TCPAddr{} }

// tunnelFixture serves handler behind a host-tunnel lease. Every dial of the
// tunnel dialer is one tunnel stream.
type tunnelFixture struct {
	dials   atomic.Int32
	mu      sync.Mutex
	streams []net.Conn
	client  *Client
}

func (f *tunnelFixture) dialer(listener *pipeListener) TunnelDialer {
	return func(ctx context.Context) (net.Conn, error) {
		f.dials.Add(1)
		local, remote := net.Pipe()
		select {
		case listener.conns <- remote:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		f.mu.Lock()
		f.streams = append(f.streams, local)
		f.mu.Unlock()
		return local, nil
	}
}

func (f *tunnelFixture) dropStreams() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, stream := range f.streams {
		_ = stream.Close()
	}
	f.streams = nil
}

func newTunnelFixture(t *testing.T, handler http.Handler, lease func(TunnelDialer) *ConnectionLease) *tunnelFixture {
	t.Helper()
	listener := newPipeListener()
	server := httptest.NewUnstartedServer(handler)
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	fixture := &tunnelFixture{}
	dial := fixture.dialer(listener)
	resolver := func(context.Context) (*ConnectionLease, error) { return lease(dial), nil }
	client, err := newEndpointClient(context.Background(), resolver, newTestLogger(), endpointTransportDependencies{}, nil)
	if err != nil {
		t.Fatalf("newEndpointClient() error = %v", err)
	}
	fixture.client = client
	return fixture
}

func tunnelLease(dial TunnelDialer) *ConnectionLease {
	return &ConnectionLease{Transport: LeaseTransportHostTunnel, Generation: "g1", ExpiresAt: time.Now().Add(time.Hour), Dial: dial}
}

func get(t *testing.T, client *Client, path string) (string, error) {
	t.Helper()
	response, err := client.httpClient.Get(client.baseURL + path)
	if err != nil {
		return "", err
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	return string(body), err
}

func TestHostTunnelLeaseCarriesHTTPWithoutResolvingAnyAddress(t *testing.T) {
	fixture := newTunnelFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello " + r.URL.Path))
	}), tunnelLease)
	if strings.Contains(fixture.client.baseURL, "https") {
		t.Fatalf("tunnel base URL %q must not claim https", fixture.client.baseURL)
	}
	body, err := get(t, fixture.client, "/api/v1/status")
	if err != nil || body != "hello /api/v1/status" {
		t.Fatalf("GET = %q, %v", body, err)
	}
	if fixture.dials.Load() != 1 {
		t.Fatalf("tunnel dials = %d, want 1", fixture.dials.Load())
	}
}

func TestHostTunnelLeaseCarriesWebSocketTraffic(t *testing.T) {
	upgrader := websocket.Upgrader{}
	fixture := newTunnelFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		kind, message, err := conn.ReadMessage()
		if err == nil {
			_ = conn.WriteMessage(kind, message)
		}
	}), tunnelLease)
	conn, _, err := fixture.client.dialWebSocket(context.Background(), "/ws", nil)
	if err != nil {
		t.Fatalf("dialWebSocket() error = %v", err)
	}
	defer func() { _ = conn.Close() }()
	if err := conn.WriteMessage(websocket.TextMessage, []byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, message, err := conn.ReadMessage(); err != nil || string(message) != "ping" {
		t.Fatalf("echo = %q, %v", message, err)
	}
}

func TestHostTunnelLeaseWithoutADialerIsRejected(t *testing.T) {
	resolver := func(context.Context) (*ConnectionLease, error) { return tunnelLease(nil), nil }
	if _, err := newEndpointClient(context.Background(), resolver, newTestLogger(), endpointTransportDependencies{}, nil); err == nil {
		t.Fatal("a host_tunnel lease without a dialer must be rejected")
	}
}

func TestHostTunnelLeaseRejectsABaseURLAndUnknownTransports(t *testing.T) {
	now := time.Now()
	cases := map[string]ConnectionLease{
		"base_url":  {Transport: LeaseTransportHostTunnel, BaseURL: "https://executor.example", Generation: "g", ExpiresAt: now.Add(time.Hour), Dial: func(context.Context) (net.Conn, error) { return nil, nil }},
		"transport": {Transport: "carrier_pigeon", Generation: "g", ExpiresAt: now.Add(time.Hour)},
	}
	for name, lease := range cases {
		if _, err := validateConnectionLease(lease, now); err == nil {
			t.Errorf("%s: lease was accepted", name)
		}
	}
}

func TestHTTPSValidationIsUnchangedForTheDefaultTransport(t *testing.T) {
	now := time.Now()
	for _, base := range []string{"http://executor.example", "https://127.0.0.1", "https://[::1]"} {
		lease := ConnectionLease{BaseURL: base, Generation: "g", ExpiresAt: now.Add(time.Hour)}
		if _, err := validateConnectionLease(lease, now); err == nil {
			t.Errorf("%s was accepted as an https lease", base)
		}
	}
	explicit := ConnectionLease{Transport: "https", BaseURL: "https://executor.example", Generation: "g", ExpiresAt: now.Add(time.Hour)}
	if _, err := validateConnectionLease(explicit, now); err != nil {
		t.Errorf("explicit https lease rejected: %v", err)
	}
}

func TestNextRequestAfterATunnelLossUsesANewStream(t *testing.T) {
	fixture := newTunnelFixture(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}), tunnelLease)
	if _, err := get(t, fixture.client, "/a"); err != nil {
		t.Fatalf("first GET: %v", err)
	}
	fixture.dropStreams()
	if body, err := get(t, fixture.client, "/b"); err != nil || body != "ok" {
		t.Fatalf("GET after a stream loss = %q, %v", body, err)
	}
	if fixture.dials.Load() != 2 {
		t.Fatalf("tunnel dials = %d, want 2 (a new stream)", fixture.dials.Load())
	}
}

func TestMutatingRequestIsNotReplayedAfterATunnelLoss(t *testing.T) {
	var posts atomic.Int32
	fixture := newTunnelFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts.Add(1)
			hijacker, _ := w.(http.Hijacker)
			conn, _, _ := hijacker.Hijack()
			_ = conn.Close() // the stream dies after the request was received
			return
		}
		_, _ = w.Write([]byte("ok"))
	}), tunnelLease)
	if _, err := get(t, fixture.client, "/warm"); err != nil {
		t.Fatalf("warm-up GET: %v", err)
	}
	response, err := fixture.client.httpClient.Post(fixture.client.baseURL+"/mutate", "text/plain", strings.NewReader("x"))
	if err == nil {
		_ = response.Body.Close()
		t.Fatal("POST over a lost stream must fail rather than succeed")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("POST timed out instead of failing: %v", err)
	}
	if posts.Load() != 1 {
		t.Fatalf("the server received the POST %d times, want exactly 1", posts.Load())
	}
}
