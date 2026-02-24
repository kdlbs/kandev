package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/process"
)

func TestHandleVscodeProxy_ProxiesRequest(t *testing.T) {
	var mu sync.Mutex
	var receivedPath, receivedMethod string
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedPath = r.URL.Path
		receivedMethod = r.Method
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer mock.Close()

	port := mustParsePort(t, mock.URL)

	s := newTestServer(t)
	s.procMgr.SetVscodeForTest(process.VscodeStatusRunning, port)

	// Use a real HTTP server so Gin's responseWriter gets a real http.ResponseWriter
	ts := httptest.NewServer(s.router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/vscode/proxy/some/path")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	mu.Lock()
	defer mu.Unlock()
	if receivedPath != "/some/path" {
		t.Errorf("expected path /some/path, got %q", receivedPath)
	}
	if receivedMethod != http.MethodGet {
		t.Errorf("expected GET, got %q", receivedMethod)
	}
}

func TestHandleVscodeProxy_RootPath(t *testing.T) {
	var mu sync.Mutex
	var receivedPath string
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedPath = r.URL.Path
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer mock.Close()

	port := mustParsePort(t, mock.URL)

	s := newTestServer(t)
	s.procMgr.SetVscodeForTest(process.VscodeStatusRunning, port)

	ts := httptest.NewServer(s.router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/vscode/proxy")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	mu.Lock()
	defer mu.Unlock()
	if receivedPath != "/" {
		t.Errorf("expected root path /, got %q", receivedPath)
	}
}

func TestHandleVscodeProxy_OriginHeader(t *testing.T) {
	var mu sync.Mutex
	var receivedOrigin string
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedOrigin = r.Header.Get("Origin")
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer mock.Close()

	port := mustParsePort(t, mock.URL)

	s := newTestServer(t)
	s.procMgr.SetVscodeForTest(process.VscodeStatusRunning, port)

	ts := httptest.NewServer(s.router)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/vscode/proxy/test", nil)
	req.Header.Set("Origin", "http://different-host:9999")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	_, _ = io.Copy(io.Discard, resp.Body)

	mu.Lock()
	defer mu.Unlock()
	expected := "http://127.0.0.1:" + strconv.Itoa(port)
	if receivedOrigin != expected {
		t.Errorf("expected Origin %q, got %q", expected, receivedOrigin)
	}
}

func TestHandleVscodeProxy_NotRunning(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/vscode/proxy/foo", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func mustParsePort(t *testing.T, rawURL string) int {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("failed to parse URL: %v", err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("failed to parse port: %v", err)
	}
	return port
}
