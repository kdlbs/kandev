package installer

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

// Timeouts bounding a release download. There is deliberately no overall
// http.Client.Timeout: these are tarballs, and a single deadline for the whole
// transfer would kill a large-but-healthy download on a slow link. Instead each
// phase that can stall silently gets its own bound — connect, TLS handshake,
// response headers — and the body stream is watched for lack of progress by
// stallGuard. http.DefaultClient has none of these, so a stalled mirror hangs
// the install forever with no way out.
const (
	downloadDialTimeout           = 30 * time.Second
	downloadTLSHandshakeTimeout   = 30 * time.Second
	downloadResponseHeaderTimeout = 60 * time.Second
)

// downloadStallTimeout is the longest gap tolerated between two successful
// reads from a response body before the download is abandoned. It is a var so
// tests can shrink it.
var downloadStallTimeout = 2 * time.Minute

// downloadClient is the shared client for tool downloads. Swapped in tests.
var downloadClient = newDownloadClient()

func newDownloadClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   downloadDialTimeout,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          10,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   downloadTLSHandshakeTimeout,
			ExpectContinueTimeout: 1 * time.Second,
			ResponseHeaderTimeout: downloadResponseHeaderTimeout,
		},
	}
}

// getDownload issues a GET for url and returns a response whose body fails
// instead of blocking when the transfer stops making progress. The caller owns
// resp.Body and must close it — closing also releases the stall watchdog and
// the derived request context.
func getDownload(ctx context.Context, url string) (*http.Response, error) {
	ctx, cancel := context.WithCancel(ctx)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := downloadClient.Do(req)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("download failed: %w", err)
	}

	resp.Body = newStallGuard(resp.Body, cancel, downloadStallTimeout)
	return resp, nil
}

// stallGuard aborts a response body that stops delivering data.
//
// ResponseHeaderTimeout only covers the wait for the status line; once headers
// are in, a mirror that goes quiet mid-body would leave the read blocked
// indefinitely. The guard arms a timer at construction and rearms it on every
// read that returns bytes, so a download is only killed for lack of progress —
// never for taking a long time while still moving.
type stallGuard struct {
	body    io.ReadCloser
	cancel  context.CancelFunc
	timer   *time.Timer
	timeout time.Duration
	stalled atomic.Bool
}

func newStallGuard(body io.ReadCloser, cancel context.CancelFunc, timeout time.Duration) *stallGuard {
	g := &stallGuard{body: body, cancel: cancel, timeout: timeout}
	g.timer = time.AfterFunc(timeout, func() {
		g.stalled.Store(true)
		// Cancelling the request context unblocks the in-flight Read.
		cancel()
	})
	return g
}

func (g *stallGuard) Read(p []byte) (int, error) {
	n, err := g.body.Read(p)
	if n > 0 {
		g.timer.Reset(g.timeout)
	}
	if err != nil && g.stalled.Load() {
		return n, fmt.Errorf("download stalled: no data received for %s: %w", g.timeout, err)
	}
	return n, err
}

func (g *stallGuard) Close() error {
	g.timer.Stop()
	err := g.body.Close()
	g.cancel()
	return err
}
