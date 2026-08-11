package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestStartShellTerminal_PostsTerminalGeometry(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{}`))

	if err := newHTTPOnlyClient(srv.URL).StartShellTerminal(
		context.Background(), "term-1", 120, 40); err != nil {
		t.Fatalf("StartShellTerminal: %v", err)
	}

	if got.Method != http.MethodPost || got.Path != "/api/v1/shell/terminal/start" {
		t.Errorf("request = %s %s, want POST /api/v1/shell/terminal/start", got.Method, got.Path)
	}
	if got.ContentType != "application/json" {
		t.Errorf("content-type = %q, want application/json", got.ContentType)
	}

	var sent struct {
		TerminalID string `json:"terminal_id"`
		Cols       int    `json:"cols"`
		Rows       int    `json:"rows"`
	}
	if err := json.Unmarshal(got.Body, &sent); err != nil {
		t.Fatalf("decode sent body: %v", err)
	}
	if sent.TerminalID != "term-1" {
		t.Errorf("terminal_id = %q, want term-1", sent.TerminalID)
	}
	if sent.Cols != 120 || sent.Rows != 40 {
		t.Errorf("cols/rows = %d / %d, want 120 / 40", sent.Cols, sent.Rows)
	}
}

func TestStartShellTerminal_FailsOnNonOKStatusWithoutRetrying(t *testing.T) {
	// The handler runs on the server's goroutine while the test goroutine reads
	// the counter, so the count is atomic rather than a plain int.
	var attempts atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	err := newHTTPOnlyClient(srv.URL).StartShellTerminal(context.Background(), "term-1", 80, 24)
	if err == nil || !strings.Contains(err.Error(), "start shell terminal failed: 500") {
		t.Fatalf("error = %v, want status 500 named", err)
	}
	// A status error means agentctl answered — retrying would just repeat a
	// deterministic rejection, so the client must give up immediately.
	if n := attempts.Load(); n != 1 {
		t.Errorf("attempts = %d, want 1 — status errors must not be retried", n)
	}
}

// flakyTransport fails the first `failures` round trips with `err`, then
// delegates to base. It stands in for the Sprites proxy tunnel dropping the
// first connection attempt.
//
// Note: the retry tests install this as the client's whole Transport, which
// replaces anything WithAuthToken/WithExecutionID would have chained on.
// newHTTPOnlyClient sets no auth options, so these tests are correctly
// isolated — but if a future test builds its client through NewClient with auth
// options, it must wrap the existing c.httpClient.Transport as `base` instead
// of overwriting it, or the retried request will silently lose its headers.
type flakyTransport struct {
	mu       sync.Mutex
	failures int
	attempts int
	err      error
	base     http.RoundTripper
}

func (t *flakyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	t.attempts++
	shouldFail := t.attempts <= t.failures
	t.mu.Unlock()
	if shouldFail {
		return nil, t.err
	}
	return t.base.RoundTrip(req)
}

func (t *flakyTransport) attemptCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.attempts
}

func TestStartShellTerminal_RetriesTransientConnectionErrors(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{}`))

	c := newHTTPOnlyClient(srv.URL)
	transport := &flakyTransport{
		failures: 1,
		err:      errors.New("dial tcp 127.0.0.1:1: connect: connection refused"),
		base:     http.DefaultTransport,
	}
	c.httpClient.Transport = transport

	if err := c.StartShellTerminal(context.Background(), "term-1", 80, 24); err != nil {
		t.Fatalf("StartShellTerminal should have recovered on retry: %v", err)
	}
	if transport.attemptCount() != 2 {
		t.Errorf("attempts = %d, want 2 (one transient failure then success)", transport.attemptCount())
	}
	// The retry must rebuild the body — a consumed reader would post an empty
	// payload on the second attempt.
	var sent struct {
		TerminalID string `json:"terminal_id"`
	}
	if err := json.Unmarshal(got.Body, &sent); err != nil {
		t.Fatalf("decode retried body: %v", err)
	}
	if sent.TerminalID != "term-1" {
		t.Errorf("retried terminal_id = %q, want term-1 — the retry lost the body", sent.TerminalID)
	}
}

func TestStartShellTerminal_DoesNotRetryNonTransientErrors(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusOK, `{}`))

	c := newHTTPOnlyClient(srv.URL)
	transport := &flakyTransport{
		failures: 1,
		err:      errors.New("x509: certificate signed by unknown authority"),
		base:     http.DefaultTransport,
	}
	c.httpClient.Transport = transport

	err := c.StartShellTerminal(context.Background(), "term-1", 80, 24)
	if err == nil || !strings.Contains(err.Error(), "certificate signed by unknown authority") {
		t.Fatalf("error = %v, want the transport error surfaced", err)
	}
	if transport.attemptCount() != 1 {
		t.Errorf("attempts = %d, want 1 — a TLS failure is not transient", transport.attemptCount())
	}
}

// signallingTransport fails its first round trip with a transient error and
// announces it on `first`, then delegates every later attempt to base so the
// real transport can observe the request context.
type signallingTransport struct {
	mu       sync.Mutex
	attempts int
	first    chan struct{}
	once     sync.Once
	base     http.RoundTripper
}

func (t *signallingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	t.attempts++
	isFirst := t.attempts == 1
	t.mu.Unlock()
	if isFirst {
		t.once.Do(func() { close(t.first) })
		return nil, errors.New("read tcp 127.0.0.1:1: connection reset by peer")
	}
	return t.base.RoundTrip(req)
}

// StartShellTerminal's retry loop sleeps with time.Sleep rather than a
// context-aware timer, so cancelling mid-backoff does not shorten the wait —
// the call still burns the full 200ms before the next attempt notices. This
// pins that observable behaviour; if the production loop is changed to
// `time.NewTimer` + `select` on ctx.Done() (the convention in
// apps/backend/CLAUDE.md), this test should be inverted to assert a prompt
// return instead.
func TestStartShellTerminal_RetryBackoffIsNotInterruptedByCancellation(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusOK, `{}`))

	c := newHTTPOnlyClient(srv.URL)
	transport := &signallingTransport{first: make(chan struct{}), base: http.DefaultTransport}
	c.httpClient.Transport = transport

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Cancel as soon as the first attempt has failed, i.e. while the client is
	// inside its 200ms backoff sleep.
	go func() {
		<-transport.first
		cancel()
	}()

	start := time.Now()
	err := c.StartShellTerminal(ctx, "term-1", 80, 24)
	elapsed := time.Since(start)

	// The second attempt reaches the real transport with a cancelled context.
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled once the retry re-dials", err)
	}
	// A context-aware backoff would return in ~0ms; the fixed sleep means the
	// caller waits the whole 200ms after cancellation.
	if elapsed < 150*time.Millisecond {
		t.Errorf("elapsed = %v, want >=150ms — the backoff is a plain time.Sleep, "+
			"so cancellation cannot cut it short", elapsed)
	}
	transport.mu.Lock()
	attempts := transport.attempts
	transport.mu.Unlock()
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2 — one transient failure then a cancelled re-dial", attempts)
	}
}

func TestIsTransientConnError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"connection reset", errors.New("read tcp: connection reset by peer"), true},
		{"connection refused", errors.New("dial tcp: connect: connection refused"), true},
		{"io.EOF sentinel", io.EOF, true},
		{"unexpected EOF text", fmt.Errorf("proxy: %w", io.ErrUnexpectedEOF), true},
		{"wrapped connection reset", fmt.Errorf("post: %w", errors.New("connection reset")), true},
		{"plain EOF text is not matched", errors.New("EOF"), false},
		{"unrelated", errors.New("permission denied"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTransientConnError(tc.err); got != tc.want {
				t.Errorf("isTransientConnError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestShellTerminalBuffer_ReturnsDecodedData(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK,
		`{"terminal_id":"term-1","data":"$ echo hi\r\nhi\r\n"}`))

	data, err := newHTTPOnlyClient(srv.URL).ShellTerminalBuffer(context.Background(), "term-1")
	if err != nil {
		t.Fatalf("ShellTerminalBuffer: %v", err)
	}

	if got.Method != http.MethodGet {
		t.Errorf("method = %q, want GET", got.Method)
	}
	if got.Path != "/api/v1/shell/terminal/term-1/buffer" {
		t.Errorf("path = %q, want the terminal ID between /terminal/ and /buffer", got.Path)
	}
	if data != "$ echo hi\r\nhi\r\n" {
		t.Errorf("data = %q, want the buffered terminal output", data)
	}
}

func TestShellTerminalBuffer_FailureModes(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"non-OK status", http.StatusNotFound, `{}`, "shell terminal buffer failed"},
		{"malformed body", http.StatusOK, `{"data":`, "unexpected EOF"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := captureServer(t, jsonResponder(tc.status, tc.body))

			data, err := newHTTPOnlyClient(srv.URL).ShellTerminalBuffer(context.Background(), "term-1")
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
			if data != "" {
				t.Errorf("data = %q, want empty on failure", data)
			}
		})
	}
}

func TestStopShellTerminal_UsesDeleteVerbOnTerminalPath(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{}`))

	if err := newHTTPOnlyClient(srv.URL).StopShellTerminal(context.Background(), "term-1"); err != nil {
		t.Fatalf("StopShellTerminal: %v", err)
	}

	if got.Method != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", got.Method)
	}
	if got.Path != "/api/v1/shell/terminal/term-1" {
		t.Errorf("path = %q, want /api/v1/shell/terminal/term-1", got.Path)
	}
}

func TestStopShellTerminal_FailsOnNonOKStatus(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusConflict, `{}`))

	err := newHTTPOnlyClient(srv.URL).StopShellTerminal(context.Background(), "term-1")
	if err == nil || !strings.Contains(err.Error(), "stop shell terminal failed") {
		t.Fatalf("error = %v, want the stop failure named", err)
	}
	if !strings.Contains(err.Error(), "409") {
		t.Errorf("error = %v, want the HTTP status carried through", err)
	}
}

func TestStreamShellTerminal_DialsBinaryWebSocketWithAuthHeaders(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	var gotPath, gotAuth, gotInstance string
	serverDone := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotInstance = r.Header.Get("X-Instance-ID")
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			close(serverDone)
			return
		}
		defer func() {
			_ = conn.Close()
			close(serverDone)
		}()
		_ = conn.WriteMessage(websocket.BinaryMessage, []byte("hello from pty"))
		// Block until the client hangs up so the handler owns its own teardown.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)

	c := newHTTPOnlyClient(srv.URL)
	c.authToken = "term-token"
	c.executionID = "exec-9"

	conn, err := c.StreamShellTerminal(context.Background(), "term-1")
	if err != nil {
		t.Fatalf("StreamShellTerminal: %v", err)
	}

	msgType, payload, err := conn.ReadMessage()
	if err != nil {
		_ = conn.Close()
		t.Fatalf("read from terminal stream: %v", err)
	}
	if msgType != websocket.BinaryMessage {
		t.Errorf("message type = %d, want BinaryMessage", msgType)
	}
	if string(payload) != "hello from pty" {
		t.Errorf("payload = %q, want the pty bytes", payload)
	}

	// The caller owns the connection; closing it drains the server handler.
	if err := conn.Close(); err != nil {
		t.Errorf("close terminal stream: %v", err)
	}
	<-serverDone

	if gotPath != "/api/v1/shell/terminal/term-1/stream" {
		t.Errorf("path = %q, want /api/v1/shell/terminal/term-1/stream", gotPath)
	}
	if gotAuth != "Bearer term-token" {
		t.Errorf("Authorization = %q, want Bearer term-token", gotAuth)
	}
	if gotInstance != "exec-9" {
		t.Errorf("X-Instance-ID = %q, want exec-9 — a recycled port would go undetected", gotInstance)
	}
}

func TestStreamShellTerminal_ReturnsErrorWhenUpgradeIsRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no such terminal", http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	conn, err := newHTTPOnlyClient(srv.URL).StreamShellTerminal(context.Background(), "gone")
	if err == nil {
		_ = conn.Close()
		t.Fatal("StreamShellTerminal returned nil error for a refused upgrade")
	}
	if !strings.Contains(err.Error(), "failed to connect to shell terminal stream") {
		t.Errorf("error = %v, want the dial failure wrapped", err)
	}
	if conn != nil {
		t.Error("conn non-nil alongside an error")
	}
}
