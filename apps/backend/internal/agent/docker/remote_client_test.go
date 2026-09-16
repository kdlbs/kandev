package docker

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// stubEngine serves the subset of the Docker Engine API the remote client
// needs, over connections handed to it rather than over a listener. That is
// the same shape as a dial-stdio transport: there is no address to dial.
type stubEngine struct {
	apiVersion string
	conns      chan net.Conn
	server     *http.Server
}

func newStubEngine(t *testing.T, apiVersion string) *stubEngine {
	t.Helper()
	e := &stubEngine{
		apiVersion: apiVersion,
		conns:      make(chan net.Conn, 8),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Api-Version", e.apiVersion)
		w.Header().Set("Ostype", "linux")
		if strings.HasSuffix(r.URL.Path, "/version") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"ApiVersion": e.apiVersion,
				"Arch":       "arm64",
			})
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	e.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	go func() {
		for c := range e.conns {
			go func(conn net.Conn) { _ = e.server.Serve(newSingleConnListener(conn)) }(c)
		}
	}()
	t.Cleanup(func() { close(e.conns) })
	return e
}

// dial returns a dialer that hands one end of an in-memory pipe to the stub
// engine and the other to the caller, so no TCP dial happens at all. A client
// that ignores its configured dialer therefore fails rather than silently
// connecting somewhere else.
func (e *stubEngine) dial() DialContextFunc {
	return func(_ context.Context, _, _ string) (net.Conn, error) {
		client, server := net.Pipe()
		e.conns <- server
		return client, nil
	}
}

// singleConnListener serves exactly one already-established connection.
type singleConnListener struct {
	conn net.Conn
	done chan struct{}
}

func newSingleConnListener(c net.Conn) net.Listener {
	return &singleConnListener{conn: c, done: make(chan struct{})}
}

func (l *singleConnListener) Accept() (net.Conn, error) {
	select {
	case <-l.done:
		return nil, net.ErrClosed
	default:
	}
	close(l.done)
	return l.conn, nil
}

func (l *singleConnListener) Close() error   { return nil }
func (l *singleConnListener) Addr() net.Addr { return l.conn.LocalAddr() }

// TestNewRemoteClientPingsThroughDialer is the core contract: every Engine API
// request must travel through the supplied dialer. The pipe-based stub cannot
// be reached any other way, so a client that falls back to a TCP dial fails
// here instead of quietly talking to a local daemon.
func TestNewRemoteClientPingsThroughDialer(t *testing.T) {
	engine := newStubEngine(t, "1.51")

	cli, err := NewRemoteClient(engine.dial(), testLogger(t))
	if err != nil {
		t.Fatalf("NewRemoteClient: %v", err)
	}
	t.Cleanup(func() { _ = cli.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := cli.Ping(ctx); err != nil {
		t.Fatalf("Ping through remote dialer: %v", err)
	}
}

// TestNewRemoteClientRejectsNilDialer keeps a remote client from being
// constructed without a transport, which would silently dial the local daemon.
func TestNewRemoteClientRejectsNilDialer(t *testing.T) {
	if _, err := NewRemoteClient(nil, testLogger(t)); err == nil {
		t.Fatal("NewRemoteClient(nil) = nil error, want error")
	}
}

// TestRemoteClientOptionOrder pins the hazard that motivates this
// constructor. client.WithHost calls sockets.ConfigureTransport, whose
// default branch installs its own TCP DialContext, so WithDialContext must be
// applied after it. Reversed, the TCP dialer wins and the client tries to
// resolve the synthetic host instead of using the supplied transport.
//
// The assertion is that the dialer is never consulted, rather than that the
// ping fails: a failure could also come from a slow resolver, and would make
// this pass for the wrong reason.
func TestRemoteClientOptionOrder(t *testing.T) {
	engine := newStubEngine(t, "1.51")

	var mu sync.Mutex
	dialed := 0
	counting := func(ctx context.Context, network, addr string) (net.Conn, error) {
		mu.Lock()
		dialed++
		mu.Unlock()
		return engine.dial()(ctx, network, addr)
	}

	reversed, err := newRemoteClientWithOptionOrder(counting, testLogger(t), true)
	if err != nil {
		t.Fatalf("reversed option order should still construct: %v", err)
	}
	t.Cleanup(func() { _ = reversed.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_ = reversed.Ping(ctx)

	mu.Lock()
	got := dialed
	mu.Unlock()
	if got != 0 {
		t.Fatalf("reversed option order used the supplied dialer %d time(s); "+
			"WithHost no longer overrides it, so the ordering guarantee in "+
			"NewRemoteClient is now untested", got)
	}
}

// TestRemoteClientCorrectOrderUsesDialer is the positive half of the pair: in
// the shipped order the supplied dialer is the one that carries the request.
func TestRemoteClientCorrectOrderUsesDialer(t *testing.T) {
	engine := newStubEngine(t, "1.51")

	var mu sync.Mutex
	dialed := 0
	counting := func(ctx context.Context, network, addr string) (net.Conn, error) {
		mu.Lock()
		dialed++
		mu.Unlock()
		return engine.dial()(ctx, network, addr)
	}

	cli, err := NewRemoteClient(counting, testLogger(t))
	if err != nil {
		t.Fatalf("NewRemoteClient: %v", err)
	}
	t.Cleanup(func() { _ = cli.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := cli.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	mu.Lock()
	got := dialed
	mu.Unlock()
	if got == 0 {
		t.Fatal("Ping succeeded without using the supplied dialer")
	}
}
