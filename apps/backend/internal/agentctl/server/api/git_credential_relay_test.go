package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
)

type relayUpstream struct {
	*httptest.Server
	calls   atomic.Int32
	method  atomic.Value
	path    atomic.Value
	query   atomic.Value
	headers atomic.Value
	body    atomic.Value
}

func newRelayUpstream(t *testing.T, handler http.HandlerFunc) *relayUpstream {
	u := &relayUpstream{}
	u.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.calls.Add(1)
		u.method.Store(r.Method)
		u.path.Store(r.URL.Path)
		u.query.Store(r.URL.RawQuery)
		u.headers.Store(r.Header.Clone())
		raw, _ := io.ReadAll(r.Body)
		u.body.Store(string(raw))
		handler(w, r)
	}))
	t.Cleanup(u.Close)
	return u
}

func relayServer(t *testing.T, upstream string) *Server {
	t.Helper()
	log := newTestLogger()
	cfg := &config.InstanceConfig{WorkDir: t.TempDir(), BrokerUpstreamURL: upstream, SessionMCP: config.NewSessionMCPToken("t")}
	return NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)
}

func doRelay(s *Server, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	s.Router().ServeHTTP(rec, req)
	return rec
}

func TestGitCredentialRelayForwardsResolveAndReissue(t *testing.T) {
	up := newRelayUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Secret", "leak")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	s := relayServer(t, up.URL+"/api/v1/git/credentials/resolve")

	rec := doRelay(s, http.MethodPost, "/git-credentials/resolve?evil=1", `{"lease":"L"}`, map[string]string{
		"Content-Type": "application/json", "Accept": "application/json", "Authorization": "Bearer caller", "X-Forwarded-For": "1.2.3.4",
	})
	if rec.Code != http.StatusAccepted || rec.Body.String() != `{"ok":true}` || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("resolve = %d %q %v", rec.Code, rec.Body.String(), rec.Header())
	}
	if rec.Header().Get("X-Secret") != "" {
		t.Fatal("only Content-Type may be returned")
	}
	if up.path.Load() != "/api/v1/git/credentials/resolve" || up.query.Load() != "" || up.body.Load() != `{"lease":"L"}` {
		t.Fatalf("upstream saw path=%v query=%v body=%v", up.path.Load(), up.query.Load(), up.body.Load())
	}
	h := up.headers.Load().(http.Header)
	if h.Get("Authorization") != "" || h.Get("X-Forwarded-For") != "" || h.Get("Content-Type") != "application/json" || h.Get("Accept") != "application/json" {
		t.Fatalf("forwarded headers = %v", h)
	}

	rec = doRelay(s, http.MethodPost, "/git-credentials/reissue", `{}`, nil)
	if rec.Code != http.StatusAccepted || up.path.Load() != "/api/v1/git/credentials/reissue" {
		t.Fatalf("reissue = %d path=%v", rec.Code, up.path.Load())
	}
}

func TestGitCredentialRelayGETResolveIsForwarded(t *testing.T) {
	up := newRelayUpstream(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusMethodNotAllowed) })
	s := relayServer(t, up.URL+"/x/resolve")
	rec := doRelay(s, http.MethodGet, "/git-credentials/resolve", "", nil)
	if rec.Code != http.StatusMethodNotAllowed || up.method.Load() != http.MethodGet {
		t.Fatalf("code=%d method=%v", rec.Code, up.method.Load())
	}
}

func TestGitCredentialRelayRequestOrder(t *testing.T) {
	up := newRelayUpstream(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	s := relayServer(t, up.URL+"/x/resolve")
	none := relayServer(t, "")
	big := strings.Repeat("a", gitCredentialMaxRequest+1)
	cases := []struct {
		name   string
		srv    *Server
		method string
		path   string
		body   string
		want   int
	}{
		{"unknown subpath", s, http.MethodPost, "/git-credentials/other", "", 404},
		{"prefix only", s, http.MethodPost, "/git-credentials/", "", 404},
		{"trailing slash", s, http.MethodPost, "/git-credentials/resolve/", "", 404},
		{"resolve PUT", s, http.MethodPut, "/git-credentials/resolve", "", 405},
		{"reissue GET", s, http.MethodGet, "/git-credentials/reissue", "", 405},
		{"unknown path beats method", s, http.MethodPut, "/git-credentials/nope", "", 404},
		{"no upstream", none, http.MethodPost, "/git-credentials/resolve", "{}", 404},
		{"no upstream beats size", none, http.MethodPost, "/git-credentials/resolve", big, 404},
		{"method beats upstream", none, http.MethodPut, "/git-credentials/resolve", "", 405},
		{"too large", s, http.MethodPost, "/git-credentials/resolve", big, 413},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := up.calls.Load()
			rec := doRelay(tc.srv, tc.method, tc.path, tc.body, nil)
			if rec.Code != tc.want {
				t.Fatalf("code = %d, want %d (%s)", rec.Code, tc.want, rec.Body.String())
			}
			if up.calls.Load() != before {
				t.Fatal("nothing may be forwarded")
			}
		})
	}
}

func TestGitCredentialRelayNeedsNoControlCredential(t *testing.T) {
	up := newRelayUpstream(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	log := newTestLogger()
	cfg := &config.InstanceConfig{WorkDir: t.TempDir(), BrokerUpstreamURL: up.URL + "/x/resolve", AuthToken: "control", SessionMCP: config.NewSessionMCPToken("t")}
	s := NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)
	if rec := doRelay(s, http.MethodPost, "/git-credentials/resolve", "{}", nil); rec.Code != http.StatusOK {
		t.Fatalf("relay must be served without credentials, got %d", rec.Code)
	}
	if rec := doRelay(s, http.MethodGet, "/api/v1/status", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("other routes keep the control credential, got %d", rec.Code)
	}
}

func TestGitCredentialRelayUpstreamFailures(t *testing.T) {
	t.Run("unreachable", func(t *testing.T) {
		up := newRelayUpstream(t, func(http.ResponseWriter, *http.Request) {})
		url := up.URL + "/x/resolve"
		up.Close()
		rec := doRelay(relayServer(t, url), http.MethodPost, "/git-credentials/resolve", "{}", nil)
		if rec.Code != http.StatusBadGateway || strings.Contains(rec.Body.String(), "127.0.0.1") {
			t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
		}
	})
	t.Run("response over limit", func(t *testing.T) {
		up := newRelayUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(strings.Repeat("a", gitCredentialMaxResponse+1)))
		})
		rec := doRelay(relayServer(t, up.URL+"/x/resolve"), http.MethodPost, "/git-credentials/resolve", "{}", nil)
		if rec.Code != http.StatusBadGateway || rec.Body.Len() > 200 {
			t.Fatalf("code=%d len=%d", rec.Code, rec.Body.Len())
		}
	})
	t.Run("redirect is returned unchanged and not followed", func(t *testing.T) {
		up := newRelayUpstream(t, func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/elsewhere", http.StatusFound)
		})
		rec := doRelay(relayServer(t, up.URL+"/x/resolve"), http.MethodPost, "/git-credentials/resolve", "{}", nil)
		if rec.Code != http.StatusFound || up.calls.Load() != 1 {
			t.Fatalf("code=%d calls=%d", rec.Code, up.calls.Load())
		}
	})
	t.Run("upstream error status passes through once", func(t *testing.T) {
		up := newRelayUpstream(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) })
		rec := doRelay(relayServer(t, up.URL+"/x/resolve"), http.MethodPost, "/git-credentials/resolve", "{}", nil)
		if rec.Code != http.StatusServiceUnavailable || up.calls.Load() != 1 {
			t.Fatalf("code=%d calls=%d (no retry)", rec.Code, up.calls.Load())
		}
	})
}

func TestGitCredentialRelayCallerCancelStopsUpstream(t *testing.T) {
	started := make(chan struct{})
	released := make(chan struct{})
	up := newRelayUpstream(t, func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(released)
	})
	s := relayServer(t, up.URL+"/x/resolve")
	ctx, cancel := context.WithCancel(t.Context())
	req := httptest.NewRequest(http.MethodPost, "/git-credentials/resolve", strings.NewReader("{}")).WithContext(ctx)
	done := make(chan struct{})
	go func() {
		s.Router().ServeHTTP(httptest.NewRecorder(), req)
		close(done)
	}()
	<-started
	cancel()
	select {
	case <-released:
	case <-time.After(5 * time.Second):
		t.Fatal("caller cancel must cancel the upstream request")
	}
	<-done
}

func TestRelayUpstreamURL(t *testing.T) {
	got, ok := relayUpstreamURL("http://10.0.0.1:8080/api/v1/git/resolve", gitCredentialReissuePath)
	if !ok || got != "http://10.0.0.1:8080/api/v1/git/reissue" {
		t.Fatalf("got %q %v", got, ok)
	}
	if _, ok := relayUpstreamURL("", gitCredentialResolvePath); ok {
		t.Fatal("empty upstream is not configured")
	}
}
