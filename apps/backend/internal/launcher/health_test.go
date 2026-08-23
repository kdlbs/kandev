package launcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeChild struct {
	exited bool
	code   int
}

func (f fakeChild) Exited() (bool, int) { return f.exited, f.code }

type toggledChild struct {
	exited atomic.Bool
	code   atomic.Int32
}

func (c *toggledChild) Exited() (bool, int) {
	return c.exited.Load(), int(c.code.Load())
}

func TestHealthTimeoutUsesDefaultWhenEnvUnusable(t *testing.T) {
	for name, raw := range map[string]string{
		"unset":       "",
		"not-numeric": "soon",
		"zero":        "0",
		"negative":    "-5",
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("KANDEV_HEALTH_TIMEOUT_MS", raw)
			if got := healthTimeout(1500); got != 1500*time.Millisecond {
				t.Fatalf("healthTimeout() = %s, want 1.5s", got)
			}
		})
	}
}

func TestHealthTimeoutHonorsEnvOverride(t *testing.T) {
	t.Setenv("KANDEV_HEALTH_TIMEOUT_MS", "250")
	if got := healthTimeout(1500); got != 250*time.Millisecond {
		t.Fatalf("healthTimeout() = %s, want 250ms", got)
	}
}

func TestWaitForHealthReturnsNilOnHealthyResponse(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("path = %q, want /health", r.URL.Path)
		}
		// The first probe is unhealthy so the poll loop iterates at least once,
		// exercising body drain + connection reuse across iterations.
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("starting"))
			return
		}
		w.Header().Set("X-Kandev-Desktop-Health-Token", "expected")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)

	var failures int
	err := waitForHealth(context.Background(), srv.URL, fakeChild{}, 5*time.Second, "expected", func() { failures++ })
	if err != nil {
		t.Fatalf("waitForHealth() = %v, want nil", err)
	}
	if failures != 0 {
		t.Fatalf("onFailure called %d times, want 0", failures)
	}
	if got := calls.Load(); got < 2 {
		t.Fatalf("probe calls = %d, want at least 2", got)
	}
}

func TestWaitForHealthIgnoresHealthyResponseWithoutMatchingToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Kandev-Desktop-Health-Token", "wrong")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	err := waitForHealth(context.Background(), srv.URL, fakeChild{}, 400*time.Millisecond, "expected", nil)
	if err == nil {
		t.Fatal("waitForHealth() = nil, want different-process error")
	}
	want := "answered a health check from a different process (missing/mismatched launcher token)"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want %q", err, want)
	}
	if !strings.Contains(err.Error(), "runtime bundle predates v0.66.0") {
		t.Fatalf("error = %v, want legacy runtime guidance", err)
	}
}

func TestWaitForHealthTimesOutOnNon2xxResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	var failures int
	err := waitForHealth(context.Background(), srv.URL, fakeChild{}, 400*time.Millisecond, "", func() { failures++ })
	if err == nil {
		t.Fatal("waitForHealth() = nil, want timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("error = %v, want a timeout error", err)
	}
	if failures != 1 {
		t.Fatalf("onFailure called %d times, want 1", failures)
	}
}

func TestWaitForHealthReturnsWhenServerHangs(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		// Accept the connection but never respond, the failure mode that made
		// the unbounded http.DefaultClient block the launcher forever.
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() {
		close(release)
		srv.Close()
	})

	done := make(chan error, 1)
	go func() {
		done <- waitForHealth(context.Background(), srv.URL, fakeChild{}, 300*time.Millisecond, "", nil)
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("waitForHealth() = nil, want timeout error")
		}
		if !strings.Contains(err.Error(), "timed out") {
			t.Fatalf("error = %v, want a timeout error", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("waitForHealth blocked on a hanging server instead of timing out")
	}
}

func TestWaitForHealthReturnsWhenContextCanceled(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() {
		close(release)
		srv.Close()
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		// A generous timeout: only cancellation can end this wait in time.
		done <- waitForHealth(ctx, srv.URL, fakeChild{}, time.Minute, "", nil)
	}()
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("waitForHealth() = nil, want cancellation error")
		}
		if !strings.Contains(err.Error(), "canceled") {
			t.Fatalf("error = %v, want a cancellation error", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("waitForHealth ignored context cancellation")
	}
}

func TestWaitForHealthCallsOnFailureWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var failures atomic.Int32
	err := waitForHealth(ctx, "http://127.0.0.1:1", fakeChild{}, time.Minute, "", func() {
		failures.Add(1)
	})
	if err == nil {
		t.Fatal("waitForHealth() = nil, want cancellation error")
	}
	if !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("error = %v, want a cancellation error", err)
	}
	if got := failures.Load(); got != 1 {
		t.Fatalf("onFailure called %d times, want 1", got)
	}
}

func TestWaitForReadyReturnsNilOnReadyResponse(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ready" {
			t.Errorf("path = %q, want /ready", r.URL.Path)
		}
		// The first probe still reports 503 (bootstrap handler / not-ready-yet),
		// exercising the poll loop, before flipping to ready.
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"starting"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	t.Cleanup(srv.Close)

	if err := waitForReady(context.Background(), srv.URL, fakeChild{}); err != nil {
		t.Fatalf("waitForReady() = %v, want nil", err)
	}
	if got := calls.Load(); got < 2 {
		t.Fatalf("probe calls = %d, want at least 2", got)
	}
}

func TestWaitForReadyRejectsChildExitDuringSuccessfulProbe(t *testing.T) {
	requestStarted := make(chan struct{})
	responseRelease := make(chan struct{})
	var child toggledChild

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ready" {
			t.Errorf("path = %q, want /ready", r.URL.Path)
		}
		close(requestStarted)
		<-responseRelease
		w.WriteHeader(http.StatusOK)
	}))
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(responseRelease) })
		srv.Close()
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() {
		done <- waitForReady(ctx, srv.URL, &child)
	}()

	select {
	case <-requestStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("waitForReady did not issue a readiness request")
	}
	child.code.Store(7)
	child.exited.Store(true)
	releaseOnce.Do(func() { close(responseRelease) })

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("waitForReady() = nil, want exit error when child exits during probe")
		}
		if !strings.Contains(err.Error(), "code 7") {
			t.Fatalf("error = %v, want the backend exit code", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waitForReady did not return after the readiness probe completed")
	}
}

func TestWaitForReadyIgnoresMissingDesktopHealthToken(t *testing.T) {
	// readyHandler never sets X-Kandev-Desktop-Health-Token (that header is
	// /health-only); probeReady must accept a bare 2xx regardless.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	if err := waitForReady(context.Background(), srv.URL, fakeChild{}); err != nil {
		t.Fatalf("waitForReady() = %v, want nil", err)
	}
}

func TestWaitForReadyKeepsPollingPastWhatWouldBeAHealthTimeout(t *testing.T) {
	// waitForReady has no timeout of its own: unlike waitForHealth's
	// healthTimeoutReleaseMS/healthTimeoutDevMS budget, a long readiness wait
	// (startup recovery still running) must not be treated as a failure.
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()
	err := waitForReady(ctx, srv.URL, fakeChild{})
	if err == nil {
		t.Fatal("waitForReady() = nil, want the injected context deadline to end the wait")
	}
	if !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("error = %v, want a cancellation error", err)
	}
	if got := calls.Load(); got < 2 {
		t.Fatalf("probe calls = %d, want at least 2 (proves it kept polling past a single failure)", got)
	}
}

func TestWaitForReadyReturnsWhenContextCanceled(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() {
		close(release)
		srv.Close()
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- waitForReady(ctx, srv.URL, fakeChild{})
	}()
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("waitForReady() = nil, want cancellation error")
		}
		if !strings.Contains(err.Error(), "canceled") {
			t.Fatalf("error = %v, want a cancellation error", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("waitForReady ignored context cancellation")
	}
}

func TestWaitForReadyFailsFastWhenBackendExited(t *testing.T) {
	err := waitForReady(context.Background(), "http://127.0.0.1:1", fakeChild{exited: true, code: 3})
	if err == nil {
		t.Fatal("waitForReady() = nil, want exit error")
	}
	if !strings.Contains(err.Error(), "code 3") {
		t.Fatalf("error = %v, want the backend exit code", err)
	}
}

func TestWaitForHealthFailsFastWhenBackendExited(t *testing.T) {
	var failures int
	err := waitForHealth(
		context.Background(),
		"http://127.0.0.1:1",
		fakeChild{exited: true, code: 3},
		time.Minute,
		"",
		func() { failures++ },
	)
	if err == nil {
		t.Fatal("waitForHealth() = nil, want exit error")
	}
	if !strings.Contains(err.Error(), "code 3") {
		t.Fatalf("error = %v, want the backend exit code", err)
	}
	if failures != 1 {
		t.Fatalf("onFailure called %d times, want 1", failures)
	}
}
