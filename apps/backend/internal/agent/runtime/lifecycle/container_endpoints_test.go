package lifecycle

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// recordingForwarder stands in for SSH local port forwarding, counting how
// many forwards were established and for which remote ports.
type recordingForwarder struct {
	mu       sync.Mutex
	forwards map[int]int // remote published port -> local port
	opened   []int
	err      error
	closed   int
	nextPort int
}

func newRecordingForwarder() *recordingForwarder {
	return &recordingForwarder{forwards: map[int]int{}, nextPort: 40000}
}

func (f *recordingForwarder) Forward(remotePort int) (int, func() error, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return 0, nil, f.err
	}
	f.opened = append(f.opened, remotePort)
	f.nextPort++
	local := f.nextPort
	f.forwards[remotePort] = local
	return local, func() error {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.closed++
		return nil
	}, nil
}

func (f *recordingForwarder) openedPorts() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]int, len(f.opened))
	copy(out, f.opened)
	return out
}

func (f *recordingForwarder) closedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

// stubPublishedPorts answers the "which host port is this container port
// published on" question that GetContainerHostPort answers in production.
type stubPublishedPorts struct {
	byContainerPort map[int]struct {
		host string
		port int
	}
	err error
}

func (s stubPublishedPorts) PublishedPort(_ context.Context, _ string, containerPort int) (string, int, error) {
	if s.err != nil {
		return "", 0, s.err
	}
	entry, ok := s.byContainerPort[containerPort]
	if !ok {
		return "", 0, errors.New("port not published")
	}
	return entry.host, entry.port, nil
}

// TestLocalEndpointResolverIsUnchanged characterizes today's behavior: the
// published host endpoint is used directly, with no forwarding.
func TestLocalEndpointResolverIsUnchanged(t *testing.T) {
	published := stubPublishedPorts{byContainerPort: map[int]struct {
		host string
		port int
	}{
		8765: {host: "127.0.0.1", port: 49001},
	}}
	resolver := newLocalEndpointResolver(published)

	host, port, err := resolver.Resolve(context.Background(), "container-1", 8765, "172.17.0.2")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if host != "127.0.0.1" || port != 49001 {
		t.Fatalf("Resolve = %s:%d, want 127.0.0.1:49001", host, port)
	}
}

// TestLocalEndpointResolverFallsBackToContainerIP preserves the existing
// behavior when the published port cannot be read.
func TestLocalEndpointResolverFallsBackToContainerIP(t *testing.T) {
	resolver := newLocalEndpointResolver(stubPublishedPorts{err: errors.New("inspect failed")})

	host, port, err := resolver.Resolve(context.Background(), "container-1", 8765, "172.17.0.2")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if host != "172.17.0.2" || port != 8765 {
		t.Fatalf("Resolve = %s:%d, want the container IP fallback 172.17.0.2:8765", host, port)
	}
}

// TestRemoteEndpointResolverForwardsToBackendLoopback is the contract that
// makes a remote container reachable: the published port lives on the remote
// daemon's loopback, which the backend cannot dial, so it must come back
// through a forward to the backend's own loopback.
func TestRemoteEndpointResolverForwardsToBackendLoopback(t *testing.T) {
	published := stubPublishedPorts{byContainerPort: map[int]struct {
		host string
		port int
	}{
		8765: {host: "127.0.0.1", port: 49001},
	}}
	fwd := newRecordingForwarder()
	resolver := newRemoteEndpointResolver(published, fwd)
	t.Cleanup(func() { _ = resolver.Close() })

	host, port, err := resolver.Resolve(context.Background(), "container-1", 8765, "172.17.0.2")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if host != "127.0.0.1" {
		t.Fatalf("host = %q, want the backend loopback", host)
	}
	if port == 49001 {
		t.Fatal("returned the remote published port directly; the backend cannot dial the remote loopback")
	}
	if got := fwd.openedPorts(); len(got) != 1 || got[0] != 49001 {
		t.Fatalf("forwarded ports = %v, want exactly [49001]", got)
	}
}

// TestRemoteEndpointResolverReusesForwards keeps repeated lookups of one port
// on a single forward. Instance ports are resolved more than once per session,
// and a forward per lookup leaks file descriptors for the session's life.
func TestRemoteEndpointResolverReusesForwards(t *testing.T) {
	published := stubPublishedPorts{byContainerPort: map[int]struct {
		host string
		port int
	}{
		8765:  {host: "127.0.0.1", port: 49001},
		41001: {host: "127.0.0.1", port: 49002},
	}}
	fwd := newRecordingForwarder()
	resolver := newRemoteEndpointResolver(published, fwd)
	t.Cleanup(func() { _ = resolver.Close() })

	ctx := context.Background()
	first, _, err := resolvePort(ctx, resolver, 8765)
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	second, _, err := resolvePort(ctx, resolver, 8765)
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if first != second {
		t.Fatalf("same container port resolved to %d then %d; forward was not reused", first, second)
	}

	// A different container port is a genuinely different forward.
	if _, _, err := resolvePort(ctx, resolver, 41001); err != nil {
		t.Fatalf("instance port resolve: %v", err)
	}
	if got := fwd.openedPorts(); len(got) != 2 {
		t.Fatalf("opened %d forwards for 2 distinct ports, want 2 (%v)", len(got), got)
	}
}

// TestRemoteEndpointResolverClosesForwards proves the session owns its
// forwards, so they do not outlive it.
func TestRemoteEndpointResolverClosesForwards(t *testing.T) {
	published := stubPublishedPorts{byContainerPort: map[int]struct {
		host string
		port int
	}{
		8765:  {host: "127.0.0.1", port: 49001},
		41001: {host: "127.0.0.1", port: 49002},
	}}
	fwd := newRecordingForwarder()
	resolver := newRemoteEndpointResolver(published, fwd)

	ctx := context.Background()
	if _, _, err := resolvePort(ctx, resolver, 8765); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if _, _, err := resolvePort(ctx, resolver, 41001); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if err := resolver.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := fwd.closedCount(); got != 2 {
		t.Fatalf("closed %d forwards, want 2", got)
	}
}

// TestRemoteEndpointResolverFailsLoudly surfaces a cause instead of handing
// back an unreachable endpoint that would hang on first use.
func TestRemoteEndpointResolverFailsLoudly(t *testing.T) {
	t.Run("published port unreadable", func(t *testing.T) {
		resolver := newRemoteEndpointResolver(
			stubPublishedPorts{err: errors.New("inspect failed")},
			newRecordingForwarder(),
		)
		t.Cleanup(func() { _ = resolver.Close() })
		// The local resolver falls back to the container IP here, but a
		// remote caller cannot reach a container IP across the network.
		if _, _, err := resolvePort(context.Background(), resolver, 8765); err == nil {
			t.Fatal("Resolve = nil error; a remote endpoint cannot fall back to the container IP")
		}
	})

	t.Run("forward fails", func(t *testing.T) {
		published := stubPublishedPorts{byContainerPort: map[int]struct {
			host string
			port int
		}{
			8765: {host: "127.0.0.1", port: 49001},
		}}
		fwd := newRecordingForwarder()
		fwd.err = errors.New("forward refused")
		resolver := newRemoteEndpointResolver(published, fwd)
		t.Cleanup(func() { _ = resolver.Close() })

		if _, _, err := resolvePort(context.Background(), resolver, 8765); err == nil {
			t.Fatal("Resolve = nil error when the forward could not be established")
		}
	})
}

func resolvePort(ctx context.Context, r containerEndpointResolver, containerPort int) (int, string, error) {
	host, port, err := r.Resolve(ctx, "container-1", containerPort, "172.17.0.2")
	return port, host, err
}
