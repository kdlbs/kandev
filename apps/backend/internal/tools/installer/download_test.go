package installer

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

// swapDownloadClient installs c as the package download client for the test.
func swapDownloadClient(t *testing.T, c *http.Client) {
	t.Helper()
	original := downloadClient
	downloadClient = c
	t.Cleanup(func() { downloadClient = original })
}

// swapStallTimeout shrinks the body stall window for the test.
func swapStallTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	original := downloadStallTimeout
	downloadStallTimeout = d
	t.Cleanup(func() { downloadStallTimeout = original })
}

// testDownloadClient builds the real download client with a shortened response
// header timeout, so tests exercise the production transport configuration.
func testDownloadClient(responseHeaderTimeout time.Duration) *http.Client {
	c := newDownloadClient()
	c.Transport.(*http.Transport).ResponseHeaderTimeout = responseHeaderTimeout
	return c
}

// serverTransport routes every request to srv regardless of the URL under test,
// letting strategy-level tests exercise real network behavior against a server
// that misbehaves on purpose.
type serverTransport struct {
	base http.RoundTripper
	addr *url.URL
}

func (t serverTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = t.addr.Scheme
	clone.URL.Host = t.addr.Host
	clone.Host = ""
	return t.base.RoundTrip(clone)
}

func redirectToServer(t *testing.T, c *http.Client, srv *httptest.Server) *http.Client {
	t.Helper()
	addr, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	return &http.Client{Transport: serverTransport{base: c.Transport, addr: addr}}
}

// stallingServer serves a handler that writes prelude (if any), flushes, and
// then goes silent for the rest of the test — the stalled-mirror case.
func stallingServer(t *testing.T, prelude []byte) *httptest.Server {
	t.Helper()

	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(prelude) > 0 {
			w.Header().Set("Content-Length", "1048576")
			_, _ = w.Write(prelude)
			w.(http.Flusher).Flush()
		}
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	// Cleanups run LIFO: unblock the handler before the server waits on it.
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	return srv
}

func TestGetDownloadFailsWhenResponseHeadersStall(t *testing.T) {
	srv := stallingServer(t, nil)
	swapDownloadClient(t, testDownloadClient(150*time.Millisecond))

	start := time.Now()
	resp, err := getDownload(t.Context(), srv.URL)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("getDownload() error = nil, want response header timeout")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("getDownload() blocked for %s, want prompt failure", elapsed)
	}
	if !strings.Contains(err.Error(), "download failed") {
		t.Fatalf("getDownload() error = %v, want download failure", err)
	}
}

func TestGetDownloadFailsWhenBodyStalls(t *testing.T) {
	srv := stallingServer(t, []byte("partial payload"))
	swapDownloadClient(t, testDownloadClient(5*time.Second))
	swapStallTimeout(t, 150*time.Millisecond)

	resp, err := getDownload(t.Context(), srv.URL)
	if err != nil {
		t.Fatalf("getDownload() error = %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	start := time.Now()
	_, err = io.ReadAll(resp.Body)
	if err == nil {
		t.Fatal("body read error = nil, want stall error")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("body read blocked for %s, want prompt failure", elapsed)
	}
	if !strings.Contains(err.Error(), "download stalled") {
		t.Fatalf("body read error = %v, want stall error", err)
	}
}

func TestGetDownloadAllowsSlowButProgressingBody(t *testing.T) {
	const chunks = 6
	payload := strings.Repeat("x", 64)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for range chunks {
			_, _ = io.WriteString(w, payload)
			w.(http.Flusher).Flush()
			select {
			case <-time.After(40 * time.Millisecond):
			case <-r.Context().Done():
				return
			}
		}
	}))
	defer srv.Close()

	swapDownloadClient(t, testDownloadClient(5*time.Second))
	// Total transfer time far exceeds the stall window; each gap stays under it.
	swapStallTimeout(t, 120*time.Millisecond)

	resp, err := getDownload(t.Context(), srv.URL)
	if err != nil {
		t.Fatalf("getDownload() error = %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("slow but progressing download failed: %v", err)
	}
	if want := chunks * len(payload); len(body) != want {
		t.Fatalf("body length = %d, want %d", len(body), want)
	}
}

// TestStallGuardIgnoresTimeSpentDownstream pins the distinction the guard
// exists to make: it must bound how long a read blocks, not how long the caller
// takes to digest what the read returned. Expanding one read's worth of a
// highly compressed archive onto a slow disk can outlast the stall window
// without the mirror having gone quiet at all.
func TestStallGuardIgnoresTimeSpentDownstream(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const timeout = 50 * time.Millisecond

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		guard := newStallGuard(io.NopCloser(strings.NewReader("abcdefghij")), cancel, timeout)
		buf := make([]byte, 5)

		if _, err := guard.Read(buf); err != nil {
			t.Fatalf("first read error = %v", err)
		}

		// The caller is busy downstream, far longer than the stall window.
		time.Sleep(10 * timeout)

		if _, err := guard.Read(buf); err != nil {
			t.Fatalf("read after slow downstream processing error = %v, want success", err)
		}
		if ctx.Err() != nil {
			t.Fatalf("guard cancelled a progressing download: %v", ctx.Err())
		}
	})
}

func TestGetDownloadCancelsWithParentContext(t *testing.T) {
	srv := stallingServer(t, []byte("partial payload"))
	swapDownloadClient(t, testDownloadClient(5*time.Second))
	swapStallTimeout(t, time.Minute)

	ctx, cancel := context.WithCancel(t.Context())
	resp, err := getDownload(ctx, srv.URL)
	if err != nil {
		t.Fatalf("getDownload() error = %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	time.AfterFunc(50*time.Millisecond, cancel)
	if _, err := io.ReadAll(resp.Body); err == nil {
		t.Fatal("body read error = nil, want context cancellation")
	}
}

func TestGithubTarballStrategyInstallFailsOnStalledServer(t *testing.T) {
	target := runtime.GOOS + "-" + runtime.GOARCH
	archive := tarGzWithFiles(t, map[string]string{
		"tool-1.0.0-" + target + "/bin/tool": "complete",
	})

	// Enough bytes for the gzip header to parse, then silence mid-stream.
	srv := stallingServer(t, archive[:16])
	swapDownloadClient(t, redirectToServer(t, testDownloadClient(5*time.Second), srv))
	swapStallTimeout(t, 150*time.Millisecond)

	strategy := NewGithubTarballStrategy(t.TempDir(), "tool", GithubTarballConfig{
		Owner:        "owner",
		Repo:         "repo",
		Version:      "1.0.0",
		AssetPattern: "tool-{version}-{os}-{arch}.tar.gz",
		BinaryPath:   "tool-{version}-{os}-{arch}/bin/tool",
		Targets: map[string]string{
			runtime.GOOS + "/" + runtime.GOARCH: target,
		},
	}, testLogger())

	err := installWithinDeadline(t, strategy)
	if err == nil || !strings.Contains(err.Error(), "download stalled") {
		t.Fatalf("Install() error = %v, want stall error", err)
	}
}

func TestGithubReleaseStrategyInstallFailsOnStalledServer(t *testing.T) {
	srv := stallingServer(t, []byte("partial binary"))
	swapDownloadClient(t, redirectToServer(t, testDownloadClient(5*time.Second), srv))
	swapStallTimeout(t, 150*time.Millisecond)

	strategy := NewGithubReleaseStrategy(t.TempDir(), "tool", GithubReleaseConfig{
		Owner:        "owner",
		Repo:         "repo",
		AssetPattern: "tool-{target}",
		Targets: map[string]string{
			runtime.GOOS + "/" + runtime.GOARCH: "target",
		},
	}, testLogger())

	err := installWithinDeadline(t, strategy)
	if err == nil || !strings.Contains(err.Error(), "download stalled") {
		t.Fatalf("Install() error = %v, want stall error", err)
	}
}

func TestGithubReleaseStrategyInstallFailsOnStalledHeaders(t *testing.T) {
	srv := stallingServer(t, nil)
	swapDownloadClient(t, redirectToServer(t, testDownloadClient(150*time.Millisecond), srv))

	strategy := NewGithubReleaseStrategy(t.TempDir(), "tool", GithubReleaseConfig{
		Owner:        "owner",
		Repo:         "repo",
		AssetPattern: "tool-{target}",
		Targets: map[string]string{
			runtime.GOOS + "/" + runtime.GOARCH: "target",
		},
	}, testLogger())

	err := installWithinDeadline(t, strategy)
	if err == nil || !strings.Contains(err.Error(), "download failed") {
		t.Fatalf("Install() error = %v, want download failure", err)
	}
}

// installWithinDeadline fails the test if Install blocks instead of erroring
// out, which is the regression these tests guard against.
func installWithinDeadline(t *testing.T, strategy Strategy) error {
	t.Helper()

	type result struct{ err error }
	done := make(chan result, 1)
	go func() {
		_, err := strategy.Install(t.Context())
		done <- result{err: err}
	}()

	select {
	case r := <-done:
		return r.err
	case <-time.After(30 * time.Second):
		t.Fatal("Install() blocked on a stalled server instead of failing")
		return nil
	}
}
