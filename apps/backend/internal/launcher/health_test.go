package launcher

import (
	"context"
	"errors"
	"net"
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

func singleHealthTarget(url string) backendEndpointSet {
	return backendEndpointSet{
		bindHosts:     []string{"localhost"},
		healthTargets: []string{url + "/health"},
		accessURL:     url,
	}
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
	readyURL, err := waitForHealth(context.Background(), singleHealthTarget(srv.URL), fakeChild{}, 5*time.Second, "expected", func() { failures++ })
	if err != nil {
		t.Fatalf("waitForHealth() = %v, want nil", err)
	}
	if readyURL != srv.URL {
		t.Fatalf("ready URL = %q, want %q", readyURL, srv.URL)
	}
	if failures != 0 {
		t.Fatalf("onFailure called %d times, want 0", failures)
	}
	if got := calls.Load(); got < 2 {
		t.Fatalf("probe calls = %d, want at least 2", got)
	}
}

func TestWaitForHealthAcceptsHealthySiblingAfterHigherPriorityFailure(t *testing.T) {
	higherPriority := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(higherPriority.Close)
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Kandev-Desktop-Health-Token", "expected")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthy.Close)

	started := time.Now()
	readyURL, err := waitForHealth(context.Background(), backendEndpointSet{
		accessURL: healthy.URL,
		healthTargets: []string{
			higherPriority.URL + "/health",
			healthy.URL + "/health",
		},
	}, fakeChild{}, 2*time.Second, "expected", nil)
	if err != nil {
		t.Fatalf("waitForHealth() = %v, want nil", err)
	}
	if readyURL != healthy.URL {
		t.Fatalf("ready URL = %q, want healthy sibling %q", readyURL, healthy.URL)
	}
	if elapsed := time.Since(started); elapsed >= healthProbeTimeout {
		t.Fatalf("waitForHealth() took %s, want healthy sibling to win before probe timeout", elapsed)
	}
}

func TestWaitForHealthPreservesWildcardBrowserOrigin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Kandev-Desktop-Health-Token", "expected")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	browserURL := strings.Replace(srv.URL, "127.0.0.1", "localhost", 1)

	readyURL, err := waitForHealth(context.Background(), backendEndpointSet{
		healthTargets: []string{srv.URL + "/health"},
		accessURLs:    []string{browserURL},
		accessURL:     browserURL,
	}, fakeChild{}, time.Second, "expected", nil)
	if err != nil {
		t.Fatalf("waitForHealth() = %v, want wildcard readiness", err)
	}
	if readyURL != browserURL {
		t.Fatalf("ready URL = %q, want preserved browser origin %q", readyURL, browserURL)
	}
}

func TestWaitForHealthPrefersHigherPriorityTargetWhenLowerRespondsFirst(t *testing.T) {
	loopbackStarted := make(chan struct{})
	lanResponded := make(chan struct{})
	releaseLoopback := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseLoopback) }) }
	t.Cleanup(release)

	loopback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(loopbackStarted)
		select {
		case <-releaseLoopback:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("X-Kandev-Desktop-Health-Token", "expected")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(loopback.Close)
	lan := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(lanResponded)
		w.Header().Set("X-Kandev-Desktop-Health-Token", "expected")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(lan.Close)

	type result struct {
		url string
		err error
	}
	done := make(chan result, 1)
	go func() {
		url, err := waitForHealth(context.Background(), backendEndpointSet{
			accessURL: loopback.URL,
			healthTargets: []string{
				loopback.URL + "/health",
				lan.URL + "/health",
			},
		}, fakeChild{}, time.Second, "expected", nil)
		done <- result{url: url, err: err}
	}()

	<-loopbackStarted
	<-lanResponded
	select {
	case got := <-done:
		t.Fatalf("waitForHealth returned %q before higher-priority target responded (err=%v)", got.url, got.err)
	case <-time.After(100 * time.Millisecond):
	}
	release()

	got := <-done
	if got.err != nil {
		t.Fatalf("waitForHealth() error = %v, want loopback readiness", got.err)
	}
	if got.url != loopback.URL {
		t.Fatalf("ready URL = %q, want higher-priority loopback %q", got.url, loopback.URL)
	}
}

func TestWaitForHealthBypassesHTTPProxyForSpecificBind(t *testing.T) {
	hosts := listHostNetworkAddresses()
	if len(hosts) == 0 {
		t.Skip("no non-loopback interface available for proxy-bypass coverage")
	}

	var proxyRequests atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		proxyRequests.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(proxy.Close)
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")

	listener, err := net.Listen("tcp", net.JoinHostPort(hosts[0], "0"))
	if err != nil {
		t.Skipf("cannot bind a non-loopback test listener: %v", err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Kandev-Desktop-Health-Token", "expected")
		w.WriteHeader(http.StatusOK)
	}))
	srv.Listener = listener
	srv.Start()
	t.Cleanup(srv.Close)

	readyURL, err := waitForHealth(context.Background(), singleHealthTarget(srv.URL), fakeChild{}, time.Second, "expected", nil)
	if err != nil {
		t.Fatalf("waitForHealth() = %v, want proxy-free readiness", err)
	}
	if readyURL != srv.URL {
		t.Fatalf("ready URL = %q, want %q", readyURL, srv.URL)
	}
	if got := proxyRequests.Load(); got != 0 {
		t.Fatalf("proxy requests = %d, want no proxy request for local readiness", got)
	}
}

func TestWaitForHealthRetainsLastSafeObservationPerTarget(t *testing.T) {
	status := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(status.Close)
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Kandev-Desktop-Health-Token", "wrong")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(foreign.Close)

	_, err := waitForHealth(context.Background(), backendEndpointSet{
		accessURL: status.URL,
		healthTargets: []string{
			status.URL + "/health",
			foreign.URL + "/health",
			"http://127.0.0.1:1/health",
		},
	}, fakeChild{}, 350*time.Millisecond, "expected", nil)
	if err == nil {
		t.Fatal("waitForHealth() = nil, want typed startup failure")
	}
	var healthErr *backendHealthError
	if !errors.As(err, &healthErr) {
		t.Fatalf("error type = %T, want *backendHealthError", err)
	}
	if healthErr.Class != healthFailureForeign {
		t.Fatalf("failure class = %q, want %q", healthErr.Class, healthFailureForeign)
	}
	if len(healthErr.Observations) != 3 {
		t.Fatalf("observations = %#v, want one observation per target", healthErr.Observations)
	}
	byURL := make(map[string]healthObservation, len(healthErr.Observations))
	for _, observation := range healthErr.Observations {
		byURL[observation.URL] = observation
	}
	if got := byURL[status.URL+"/health"]; got.Outcome != healthOutcomeHTTPStatus || got.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status observation = %#v, want HTTP 503", got)
	}
	if got := byURL[foreign.URL+"/health"]; got.Outcome != healthOutcomeForeignProcess || got.SafeDetail != "missing or mismatched launcher token" {
		t.Fatalf("foreign observation = %#v, want token mismatch without token value", got)
	}
	if got := byURL["http://127.0.0.1:1/health"]; got.Outcome != healthOutcomeConnectionError || got.SafeDetail == "" {
		t.Fatalf("connection observation = %#v, want bounded safe detail", got)
	}
}

func TestWaitForHealthClassifiesUnreachableBackend(t *testing.T) {
	_, err := waitForHealth(context.Background(), singleHealthTarget("http://127.0.0.1:1"), fakeChild{}, 50*time.Millisecond, "expected", nil)
	if err == nil {
		t.Fatal("waitForHealth() = nil, want timeout error")
	}
	var healthErr *backendHealthError
	if !errors.As(err, &healthErr) {
		t.Fatalf("error type = %T, want *backendHealthError", err)
	}
	if healthErr.Class != healthFailureUnreachable {
		t.Fatalf("failure class = %q, want %q", healthErr.Class, healthFailureUnreachable)
	}
}

func TestWaitForHealthIgnoresHealthyResponseWithoutMatchingToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Kandev-Desktop-Health-Token", "wrong")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	_, err := waitForHealth(context.Background(), singleHealthTarget(srv.URL), fakeChild{}, 400*time.Millisecond, "expected", nil)
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
	_, err := waitForHealth(context.Background(), singleHealthTarget(srv.URL), fakeChild{}, 400*time.Millisecond, "", func() { failures++ })
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
		_, err := waitForHealth(context.Background(), singleHealthTarget(srv.URL), fakeChild{}, 300*time.Millisecond, "", nil)
		done <- err
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
		_, err := waitForHealth(ctx, singleHealthTarget(srv.URL), fakeChild{}, time.Minute, "", nil)
		done <- err
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
	_, err := waitForHealth(ctx, singleHealthTarget("http://127.0.0.1:1"), fakeChild{}, time.Minute, "", func() {
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
	_, err := waitForHealth(
		context.Background(),
		singleHealthTarget("http://127.0.0.1:1"),
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
	var healthErr *backendHealthError
	if !errors.As(err, &healthErr) {
		t.Fatalf("error type = %T, want *backendHealthError", err)
	}
	if healthErr.Class != healthFailureEarlyExit || !healthErr.ChildExited || healthErr.ChildExitCode != 3 {
		t.Fatalf("health error = %#v, want early exit code 3", healthErr)
	}
	if failures != 1 {
		t.Fatalf("onFailure called %d times, want 1", failures)
	}
}
