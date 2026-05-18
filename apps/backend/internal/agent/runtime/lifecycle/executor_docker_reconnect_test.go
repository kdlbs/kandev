package lifecycle

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// stubHostPortLookup satisfies hostPortLookup for resolveDockerEndpoint
// tests without needing a real docker daemon.
type stubHostPortLookup struct {
	host string
	port int
	err  error
}

func (s stubHostPortLookup) GetContainerHostPort(_ context.Context, _ string, _ int) (string, int, error) {
	return s.host, s.port, s.err
}

func TestResolveDockerEndpoint_PrefersPublishedHostPort(t *testing.T) {
	host, port := resolveDockerEndpoint(
		context.Background(),
		stubHostPortLookup{host: "127.0.0.1", port: 54321},
		"container-x", 4000, "172.17.0.5",
		newTestDockerLogger(),
	)
	if host != "127.0.0.1" || port != 54321 {
		t.Fatalf("got %s:%d, want 127.0.0.1:54321 (published port wins)", host, port)
	}
}

func TestResolveDockerEndpoint_FallsBackToContainerIPOnLookupError(t *testing.T) {
	// Non-Linux dev hosts can't reach the container IP directly, so
	// resolveDockerEndpoint falls back to the published host port. When the
	// lookup itself errors (e.g. docker daemon hiccup), the fallback path is
	// the container IP at the requested container port — anything else
	// silently routes traffic to the wrong endpoint.
	host, port := resolveDockerEndpoint(
		context.Background(),
		stubHostPortLookup{err: errors.New("port not published")},
		"container-x", 4000, "172.17.0.5",
		newTestDockerLogger(),
	)
	if host != "172.17.0.5" || port != 4000 {
		t.Fatalf("got %s:%d, want 172.17.0.5:4000 (container IP fallback)", host, port)
	}
}

// stubHealthChecker satisfies healthChecker for waitForAgentctlHealth tests.
// Health() succeeds after `becomesHealthyAfter` invocations and returns
// `failErr` until then. Tracks call count so tests can assert the retry
// budget was respected.
type stubHealthChecker struct {
	becomesHealthyAfter int32
	failErr             error
	calls               int32
}

func (s *stubHealthChecker) Health(_ context.Context) error {
	n := atomic.AddInt32(&s.calls, 1)
	if n >= s.becomesHealthyAfter {
		return nil
	}
	return s.failErr
}

func TestWaitForAgentctlHealth_HappyPathReturnsImmediately(t *testing.T) {
	stub := &stubHealthChecker{becomesHealthyAfter: 1}
	r := &DockerExecutor{logger: newTestDockerLogger()}

	if err := waitForAgentctlHealthWith(context.Background(), stub, 5, time.Millisecond); err != nil {
		t.Fatalf("happy-path Health() should not error: %v", err)
	}
	_ = r // keep the parameter shape parallel with production callers
	if got := atomic.LoadInt32(&stub.calls); got != 1 {
		t.Fatalf("expected 1 Health() call, got %d", got)
	}
}

func TestWaitForAgentctlHealth_RetriesUntilHealthy(t *testing.T) {
	stub := &stubHealthChecker{
		becomesHealthyAfter: 3,
		failErr:             errors.New("connection refused"),
	}

	if err := waitForAgentctlHealthWith(context.Background(), stub, 10, time.Millisecond); err != nil {
		t.Fatalf("expected eventual success, got %v", err)
	}
	if got := atomic.LoadInt32(&stub.calls); got != 3 {
		t.Fatalf("expected 3 Health() calls, got %d", got)
	}
}

func TestWaitForAgentctlHealth_GivesUpAfterMaxRetries(t *testing.T) {
	stub := &stubHealthChecker{
		becomesHealthyAfter: 999, // never succeeds within the test budget
		failErr:             errors.New("connection refused"),
	}

	err := waitForAgentctlHealthWith(context.Background(), stub, 4, time.Millisecond)
	if err == nil {
		t.Fatal("expected exhaustion error, got nil")
	}
	if got := atomic.LoadInt32(&stub.calls); got != 4 {
		t.Fatalf("expected 4 Health() calls (one per retry), got %d", got)
	}
	if !errors.Is(err, stub.failErr) {
		t.Fatalf("exhaustion error should wrap the last underlying error: %v", err)
	}
}

func TestWaitForAgentctlHealth_HonoursContextCancellation(t *testing.T) {
	stub := &stubHealthChecker{
		becomesHealthyAfter: 999,
		failErr:             errors.New("connection refused"),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancelled

	err := waitForAgentctlHealthWith(ctx, stub, 100, time.Millisecond)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	// One Health() call before the loop notices cancellation; the cancelable
	// select exits immediately via ctx.Done() so cancellation is observed
	// within the same iteration.
	if got := atomic.LoadInt32(&stub.calls); got > 2 {
		t.Fatalf("expected ≤2 Health() calls before honouring cancellation, got %d", got)
	}
}
