package docker

import (
	"context"
	"encoding/json"
	"errors"
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

	cli, err := NewRemoteClient(RemoteTransport{Dial: engine.dial()}, testLogger(t))
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
	if _, err := NewRemoteClient(RemoteTransport{}, testLogger(t)); err == nil {
		t.Fatal("NewRemoteClient with no dialer = nil error, want error")
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

	reversed, err := newRemoteClientWithOptionOrder(RemoteTransport{Dial: counting}, testLogger(t), true)
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

	cli, err := NewRemoteClient(RemoteTransport{Dial: counting}, testLogger(t))
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

// TestPingVersionReportsTheDaemonAPIVersion gives the connection test
// something concrete to show. A bare success tells the user nothing about
// which daemon answered.
func TestPingVersionReportsTheDaemonAPIVersion(t *testing.T) {
	engine := newStubEngine(t, "1.51")

	cli, err := NewRemoteClient(RemoteTransport{Dial: engine.dial()}, testLogger(t))
	if err != nil {
		t.Fatalf("NewRemoteClient: %v", err)
	}
	t.Cleanup(func() { _ = cli.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	version, err := cli.PingVersion(ctx)
	if err != nil {
		t.Fatalf("PingVersion: %v", err)
	}
	if version != "1.51" {
		t.Fatalf("PingVersion = %q, want 1.51", version)
	}
}

// TestExplainRemoteFailureRestoresTheTransportCause pins the reason the
// transport keeps a cause at all. The Engine API client replaces a transport
// error whose text names a unix socket dial with a generic "cannot connect"
// error that wraps nothing, and a remote daemon reports its own socket
// failures in exactly that wording, so without this the caller cannot tell a
// denied socket from a dead daemon.
func TestExplainRemoteFailureRestoresTheTransportCause(t *testing.T) {
	cause := errors.New("remote user cannot access the Docker socket")
	cli, err := NewRemoteClient(RemoteTransport{
		Dial: func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("dial unix /var/run/docker.sock: connect: permission denied")
		},
		Cause: func() error { return cause },
	}, testLogger(t))
	if err != nil {
		t.Fatalf("NewRemoteClient: %v", err)
	}
	t.Cleanup(func() { _ = cli.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, pingErr := cli.PingVersion(ctx); !errors.Is(pingErr, cause) {
		t.Fatalf("PingVersion error = %v, want it to wrap %v", pingErr, cause)
	}
}

// TestExplainRemoteFailureLeavesDaemonErrorsAlone keeps the substitution
// narrow. A daemon that answered and refused the request has already named the
// cause, and replacing it with a stale transport failure would be a worse
// error than the one the daemon gave.
func TestExplainRemoteFailureLeavesDaemonErrorsAlone(t *testing.T) {
	engine := newStubEngine(t, "1.51")
	cli, err := NewRemoteClient(RemoteTransport{
		Dial:  engine.dial(),
		Cause: func() error { return errors.New("a failure from an earlier connection") },
	}, testLogger(t))
	if err != nil {
		t.Fatalf("NewRemoteClient: %v", err)
	}
	t.Cleanup(func() { _ = cli.Close() })

	fromDaemon := errors.New("no such image")
	if got := cli.ExplainRemoteFailure(fromDaemon); !errors.Is(got, fromDaemon) {
		t.Fatalf("ExplainRemoteFailure(%v) = %v, want it unchanged", fromDaemon, got)
	}
	if got := cli.ExplainRemoteFailure(nil); got != nil {
		t.Fatalf("ExplainRemoteFailure(nil) = %v, want nil", got)
	}
}

// TestExplainRemoteFailureWithoutACause returns the Engine API client's own
// error when the transport recorded nothing, rather than inventing one.
func TestExplainRemoteFailureWithoutACause(t *testing.T) {
	engine := newStubEngine(t, "1.51")
	cli, err := NewRemoteClient(RemoteTransport{Dial: engine.dial()}, testLogger(t))
	if err != nil {
		t.Fatalf("NewRemoteClient: %v", err)
	}
	t.Cleanup(func() { _ = cli.Close() })

	original := errors.New("some transport failure")
	if got := cli.ExplainRemoteFailure(original); !errors.Is(got, original) {
		t.Fatalf("ExplainRemoteFailure(%v) = %v, want it unchanged", original, got)
	}
}
