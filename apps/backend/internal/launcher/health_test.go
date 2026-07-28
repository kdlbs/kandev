package launcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type fakeChild struct {
	exited bool
	code   int
}

func (f fakeChild) Exited() (bool, int) { return f.exited, f.code }

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
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)

	var failures int
	err := waitForHealth(context.Background(), srv.URL, fakeChild{}, 5*time.Second, func() { failures++ })
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

func TestWaitForHealthTimesOutOnNon2xxResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	var failures int
	err := waitForHealth(context.Background(), srv.URL, fakeChild{}, 400*time.Millisecond, func() { failures++ })
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
		done <- waitForHealth(context.Background(), srv.URL, fakeChild{}, 300*time.Millisecond, nil)
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
		done <- waitForHealth(ctx, srv.URL, fakeChild{}, time.Minute, nil)
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

func TestWaitForHealthFailsFastWhenBackendExited(t *testing.T) {
	var failures int
	err := waitForHealth(
		context.Background(),
		"http://127.0.0.1:1",
		fakeChild{exited: true, code: 3},
		time.Minute,
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
