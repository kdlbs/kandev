package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// startCapturingHTMLServer starts an httptest server that captures the
// Accept-Encoding header it receives and replies with HTML plus a couple
// of iframe-blocking security headers.
func startCapturingHTMLServer(t *testing.T, captured *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*captured = r.Header.Get("Accept-Encoding")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
		w.Header().Set("X-Frame-Options", "DENY")
		_, _ = io.WriteString(w, "<html><body><p>Hi</p></body></html>")
	}))
	t.Cleanup(srv.Close)
	return srv
}

// portOf extracts the numeric port from an httptest.Server URL.
func portOf(t *testing.T, rawURL string) int {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("parse port from %q: %v", u.Port(), err)
	}
	return port
}

func TestCreatePortProxy_StripsAcceptEncodingAndInjectsHTML(t *testing.T) {
	var capturedAE string
	upstream := startCapturingHTMLServer(t, &capturedAE)
	port := portOf(t, upstream.URL)

	srv := newTestServer(t)
	proxy := srv.createPortProxy(port)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	resp := rec.Result()
	defer func() { _ = resp.Body.Close() }()

	// The proxy deletes the client's Accept-Encoding header. The stdlib
	// Transport may re-add "gzip" on its own (and transparently decode),
	// but the original client value ("gzip, deflate") must not pass through.
	if capturedAE == "gzip, deflate" {
		t.Errorf("expected client Accept-Encoding to be stripped, got %q", capturedAE)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "<script>") {
		t.Errorf("expected injected <script> tag in body, got: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "kandev-inspector") {
		t.Errorf("expected kandev-inspector marker in body, got: %s", bodyStr)
	}

	if got := resp.Header.Get("Content-Security-Policy"); got != "" {
		t.Errorf("expected CSP header to be stripped, got %q", got)
	}
	if got := resp.Header.Get("X-Frame-Options"); got != "" {
		t.Errorf("expected X-Frame-Options to be stripped, got %q", got)
	}
	if got := resp.Header.Get("Content-Encoding"); got != "" {
		t.Errorf("expected no Content-Encoding header, got %q", got)
	}
}

func TestCreatePortProxy_SwitchingProtocolsPassesThrough(t *testing.T) {
	srv := newTestServer(t)
	proxy := srv.createPortProxy(9999)

	resp := &http.Response{
		StatusCode: http.StatusSwitchingProtocols,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader("")),
	}

	if err := proxy.ModifyResponse(resp); err != nil {
		t.Fatalf("ModifyResponse returned error: %v", err)
	}
	if got := resp.Header.Get("Connection"); got != "Upgrade" {
		t.Errorf("expected Connection: Upgrade, got %q", got)
	}
}

func TestCreatePortProxy_NonHTMLPassesThrough(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(upstream.Close)
	port := portOf(t, upstream.URL)

	srv := newTestServer(t)
	proxy := srv.createPortProxy(port)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	resp := rec.Result()
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != `{"ok":true}` {
		t.Errorf("expected JSON body unchanged, got %q", string(body))
	}
	if strings.Contains(string(body), "<script>") {
		t.Errorf("expected no <script> injection for non-HTML, got %q", string(body))
	}
}
