package websocket

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/auth"
	"github.com/kandev/kandev/internal/auth/authn"
	authhttpmw "github.com/kandev/kandev/internal/auth/httpmw"
	"github.com/kandev/kandev/internal/auth/store"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	userstore "github.com/kandev/kandev/internal/user/store"
)

// newProxyAuthService builds an auth service with authentication enabled and an
// admin user whose session-cookie token is returned. Mirrors the httpmw test
// harness so the full production chain (httpmw.Middleware → requireConnectionAuth
// → HandlePortProxy) is exercised against a real credential store.
func newProxyAuthService(t *testing.T) (*auth.Service, string) {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	users, cleanup, err := userstore.Provide(conn, conn)
	if err != nil {
		t.Fatalf("user store: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	authStore, err := store.New(conn, conn)
	if err != nil {
		t.Fatalf("auth store: %v", err)
	}
	cfg := &config.Config{}
	cfg.Features.Auth = true
	cfg.Auth.SessionTTLHours = 720
	svc, err := auth.NewService(context.Background(), auth.Deps{
		Cfg: cfg, Store: authStore, Users: users,
	})
	if err != nil {
		t.Fatalf("auth service: %v", err)
	}
	_, token, err := svc.Setup(context.Background(), "admin@x.dev", "adminpass123", "Admin", "", "")
	if err != nil {
		t.Fatalf("setup admin: %v", err)
	}
	return svc, token
}

// newAuthEnforcingFakeAgentctl stands in for the agentctl inside an executor: it
// rejects requests without the injected Bearer token (like the real agentctl's
// bearerTokenAuth) and forwards /api/v1/port-proxy/<port>/* to the fake app.
// It records every forwarded request so tests can assert the gateway stripped
// the capability from the query before it reaches the app.
func newAuthEnforcingFakeAgentctl(t *testing.T, appURL, expectedToken string) (*httptest.Server, *fakeAgentctlRecordings) {
	t.Helper()
	target, err := url.Parse(appURL)
	if err != nil {
		t.Fatalf("parse app URL: %v", err)
	}
	var mu sync.Mutex
	var forwarded []string // "path?query" as received by the fake agentctl
	var referers []string
	record := func(rawQuery string, path, referer string) {
		mu.Lock()
		defer mu.Unlock()
		if rawQuery != "" {
			forwarded = append(forwarded, path+"?"+rawQuery)
		} else {
			forwarded = append(forwarded, path)
		}
		referers = append(referers, referer)
	}
	proxy := &httputil.ReverseProxy{}
	proxy.Rewrite = func(r *httputil.ProxyRequest) {
		record(r.Out.URL.RawQuery, r.Out.URL.Path, r.Out.Header.Get("Referer"))
		r.SetURL(target)
		// /api/v1/port-proxy/<port>/<path> → /<path>
		rest, ok := strings.CutPrefix(r.Out.URL.Path, "/api/v1/port-proxy/")
		if ok {
			if _, path, found := strings.Cut(rest, "/"); found {
				r.Out.URL.Path = "/" + path
			} else {
				r.Out.URL.Path = "/"
			}
		}
		r.Out.URL.RawPath = ""
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if expectedToken != "" && r.Header.Get("Authorization") != "Bearer "+expectedToken {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":"missing or invalid Authorization header"}`)
			return
		}
		proxy.ServeHTTP(w, r)
	}))
	srv.Config.SetKeepAlivesEnabled(false)
	t.Cleanup(srv.Close)
	recordings := &fakeAgentctlRecordings{forwarded: &forwarded, referers: &referers, mu: &mu}
	return srv, recordings
}

// fakeAgentctlRecordings captures what the fake agentctl received, with a
// race-safe snapshot for assertions.
type fakeAgentctlRecordings struct {
	mu        *sync.Mutex
	forwarded *[]string // "path?query" as received by the fake agentctl
	referers  *[]string
}

func (r *fakeAgentctlRecordings) snapshot() (forwarded, referers []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	forwarded = append([]string(nil), (*r.forwarded)...)
	referers = append([]string(nil), (*r.referers)...)
	return forwarded, referers
}

// newFakeProxyApp serves a Vite-style single page: an HTML document referencing
// a manifest and a module script, plus the manifest itself. The HTML mirrors
// the index.html shape that triggers the bug: the manifest <link> is fetched by
// the browser with credentials omitted, so it never carries the session cookie.
func newFakeProxyApp(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/manifest.webmanifest", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/manifest+json")
		_, _ = io.WriteString(w, `{"name":"proxied-app"}`)
	})
	mux.HandleFunc("/src/main.tsx", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/javascript")
		_, _ = io.WriteString(w, `console.log("main");`)
	})
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/landing")
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc("/landing", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<!doctype html><html><head><title>landed</title></head><body>ok</body></html>`)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<!doctype html><html><head>`+
			`<link rel="manifest" href="/manifest.webmanifest">`+
			`<script type="module" src="/src/main.tsx"></script>`+
			`</head><body>proxied app</body></html>`)
	})
	srv := httptest.NewServer(mux)
	srv.Config.SetKeepAlivesEnabled(false)
	t.Cleanup(srv.Close)
	return srv
}

// proxyAuthGatewayResponse is one response from the gateway under test.
type proxyAuthGatewayResponse struct {
	status  int
	body    string
	headers http.Header
}

// proxyAuthGateway issues requests against the full production chain: CORS,
// auth middleware, gateway routes (requireConnectionAuth + port proxy), a
// token-enforcing fake agentctl, and the fake app. It returns the request
// closure plus the fixture server URL so callers can derive the port-scoped
// session cookie name for its Host.
func proxyAuthGateway(t *testing.T, handler http.Handler) (func(method, path string, headers map[string]string) proxyAuthGatewayResponse, string) {
	t.Helper()
	srv := httptest.NewUnstartedServer(handler)
	srv.Config.SetKeepAlivesEnabled(false)
	srv.Start()
	t.Cleanup(srv.Close)

	transport := &http.Transport{DisableKeepAlives: true}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	return func(method, path string, headers map[string]string) proxyAuthGatewayResponse {
		t.Helper()
		req, err := http.NewRequest(method, srv.URL+path, nil)
		if err != nil {
			t.Fatalf("build request %s: %v", path, err)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request %s: %v", path, err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read body for %s: %v", path, err)
		}
		return proxyAuthGatewayResponse{status: resp.StatusCode, body: string(body), headers: resp.Header}
	}, srv.URL
}

// scopedSessionCookieHeader builds the Cookie header for the gateway fixture's
// Host (127.0.0.1:<random port>): the session cookie must carry the
// port-scoped name the auth middleware derives from that request Host.
func scopedSessionCookieHeader(t *testing.T, srvURL, sessionToken string) map[string]string {
	t.Helper()
	u, err := url.Parse(srvURL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	port := u.Port()
	if port == "" {
		t.Fatal("fixture server URL has no port")
	}
	return map[string]string{"Cookie": "kandev_session_" + port + "=" + sessionToken}
}

var manifestLinkPattern = regexp.MustCompile(`href="([^"]*manifest\.webmanifest[^"]*)"`)

// TestPortProxyServesCredentiallessSubresourcesAfterDocumentAuth pins the
// regression from the field report: with auth enforced, the preview document
// loads (it carries the session cookie) but the browser's manifest fetch —
// which never sends cookies — 401s on every subresource request. Once the
// document authenticates, the proxy must mint a short-lived, path-scoped
// capability and propagate it (cookie + rewritten asset URLs) so the whole
// proxy subtree stays authorized without requiring per-request credentials.
func TestPortProxyServesCredentiallessSubresourcesAfterDocumentAuth(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	svc, sessionToken := newProxyAuthService(t)
	app := newFakeProxyApp(t)
	agentctl, recordings := newAuthEnforcingFakeAgentctl(t, app.URL, "agentctl-token")

	manager := newProxyTestManager(t, log)
	// Wire the production ownership boundary: the session-ownership check runs
	// with whatever identity requireConnectionAuth restored (session cookie or
	// capability), so a capability minted for user A must not unlock user B's
	// session. Records the identities the check actually saw.
	var checkerMu sync.Mutex
	var seenOwners []string
	manager.SetSessionAccessChecker(func(ctx context.Context, sessionID string) error {
		identity, ok := authn.IdentityFromContext(ctx)
		checkerMu.Lock()
		defer checkerMu.Unlock()
		if !ok {
			seenOwners = append(seenOwners, "<none>")
			return errors.New("no identity")
		}
		seenOwners = append(seenOwners, identity.UserID)
		if identity.UserID != "default-user" {
			return errors.New("foreign session")
		}
		return nil
	})
	addExecutionForURL(t, manager, "sess-auth", agentctl.URL, "agentctl-token", log)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(authhttpmw.Middleware(svc))
	gateway := NewGateway(log)
	gateway.SetAuthPolicy(AuthPolicy{
		Enforced:     func() bool { return svc.Mode() != auth.ModeDisabled },
		ResolveToken: svc.ResolveBearer,
	})
	gateway.SetPortProxy(manager)
	gateway.SetupRoutes(router)

	request, srvURL := proxyAuthGateway(t, router)
	cookieHeader := scopedSessionCookieHeader(t, srvURL, sessionToken)

	// 1. The document request authenticates with the session cookie.
	doc := request(http.MethodGet, "/port-proxy/sess-auth/5173/", cookieHeader)
	if doc.status != http.StatusOK {
		t.Fatalf("document status = %d, want %d (body=%s)", doc.status, http.StatusOK, doc.body)
	}
	if !strings.Contains(doc.body, `/port-proxy/sess-auth/5173/manifest.webmanifest`) {
		t.Fatalf("document HTML does not reference the proxied manifest:\n%s", doc.body)
	}
	capSetCookie := doc.headers.Values("Set-Cookie")
	var capabilityCookie string
	for _, sc := range capSetCookie {
		if strings.HasPrefix(sc, proxyCapabilityCookieName+"=") {
			capabilityCookie = sc
		}
	}
	if capabilityCookie == "" {
		t.Fatal("document response does not set the path-scoped capability cookie")
	}
	if !strings.Contains(capabilityCookie, "Path=/port-proxy/sess-auth/5173") {
		t.Fatalf("capability cookie %q not scoped to the proxy subtree", capabilityCookie)
	}
	// Capability-bearing responses are uncacheable and referrer-safe.
	if got := doc.headers.Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("document Cache-Control = %q, want %q", got, "private, no-store")
	}
	if got := doc.headers.Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("document Referrer-Policy = %q, want %q", got, "no-referrer")
	}

	// 2. The rewritten manifest link carries the capability query parameter, so
	// the browser's credential-less manifest fetch authenticates without cookies.
	match := manifestLinkPattern.FindStringSubmatch(doc.body)
	if len(match) != 2 {
		t.Fatalf("no manifest link in rewritten HTML:\n%s", doc.body)
	}
	manifestURL := match[1]
	if !strings.Contains(manifestURL, proxyCapabilityQueryParam+"=") {
		t.Fatalf("manifest link %q missing the capability query parameter", manifestURL)
	}
	manifestReq := request(http.MethodGet, manifestURL, nil) // no cookie, like Chrome's manifest fetch
	if manifestReq.status != http.StatusOK {
		t.Fatalf("manifest (capability query, no cookie) status = %d, want %d (body=%s)",
			manifestReq.status, http.StatusOK, manifestReq.body)
	}

	// 3. The capability never reaches the proxied app: the gateway strips the
	// reserved parameter from the forwarded query (any encoding) and from any
	// Referer header that would embed a rewritten asset URL.
	forwarded, _ := recordings.snapshot()
	for _, seen := range forwarded {
		if strings.Contains(seen, proxyCapabilityQueryParam) {
			t.Fatalf("capability leaked to agentctl in forwarded request %q", seen)
		}
	}
	// A later request whose Referer embeds a rewritten asset URL (which carries
	// the capability) must be forwarded with the capability stripped. The
	// request itself authenticates with the capability, like a browser
	// subresource would.
	_, capQuery, _ := strings.Cut(manifestURL, proxyCapabilityQueryParam+"=")
	warmURL := "/port-proxy/sess-auth/5173/@react-refresh?" + proxyCapabilityQueryParam + "=" + capQuery
	warm := request(http.MethodGet, warmURL,
		map[string]string{"Referer": "http://gateway.test" + manifestURL})
	if warm.status != http.StatusOK {
		t.Fatalf("warm-up request status = %d, want %d", warm.status, http.StatusOK)
	}
	_, referers := recordings.snapshot()
	if len(referers) > 0 {
		last := referers[len(referers)-1]
		if strings.Contains(last, proxyCapabilityQueryParam) {
			t.Fatalf("capability leaked to agentctl in Referer %q", last)
		}
		if !strings.Contains(last, "manifest.webmanifest") {
			t.Fatalf("expected the manifest Referer to survive sanitization, got %q", last)
		}
	}

	// A Referer whose capability key is percent-encoded must be sanitized too —
	// the sanitizer cannot rely on the literal spelling.
	encodedRef := "http://gateway.test/port-proxy/sess-auth/5173/x?%6bandev_cap=stale&a=1"
	_ = request(http.MethodGet, "/port-proxy/sess-auth/5173/@vite/client?"+proxyCapabilityQueryParam+"="+capQuery,
		map[string]string{"Referer": encodedRef})
	_, referers = recordings.snapshot()
	if len(referers) > 0 {
		last := referers[len(referers)-1]
		if strings.Contains(last, "kandev_cap") || strings.Contains(last, "%6bandev_cap") {
			t.Fatalf("encoded capability leaked to agentctl in Referer %q", last)
		}
		if !strings.Contains(last, "a=1") {
			t.Fatalf("encoded-Referer sanitization dropped unrelated params, got %q", last)
		}
	}

	// 3b. A reload carrying BOTH the capability and ?token=<PAT> authenticates
	// via the capability, but the valid PAT is still a gateway credential and
	// must be stripped from the forwarded query (never reach the app), while
	// the capability itself is also stripped.
	// A real gateway PAT (svc.ResolveBearer only accepts stored API tokens,
	// not the Setup bootstrap token).
	_, pat, patErr := svc.MintToken(context.Background(), userstore.DefaultUserID, "ci", 0)
	if patErr != nil {
		t.Fatalf("mint PAT: %v", patErr)
	}
	tokenReload := "/port-proxy/sess-auth/5173/@vite/client?" + proxyCapabilityQueryParam + "=" + capQuery + "&token=" + pat
	tok := request(http.MethodGet, tokenReload, nil)
	if tok.status != http.StatusOK {
		t.Fatalf("capability+token reload status = %d, want %d", tok.status, http.StatusOK)
	}
	forwarded, _ = recordings.snapshot()
	if len(forwarded) > 0 {
		last := forwarded[len(forwarded)-1]
		if strings.Contains(last, "token=") || strings.Contains(last, proxyCapabilityQueryParam) {
			t.Fatalf("gateway PAT/capability leaked to agentctl in forwarded request %q", last)
		}
	}

	// 3c. Duplicate kandev_port_proxy cookies (a stale value first, then the
	// valid subtree cookie) must not let the stale one shadow the valid one:
	// any cookie value that validates authorizes the request.
	staleCap := "kandev_port_proxy=stale-not-valid; Path=/port-proxy/sess-auth/5173"
	dup := request(http.MethodGet, "/port-proxy/sess-auth/5173/@vite/client?"+proxyCapabilityQueryParam+"="+capQuery,
		map[string]string{"Cookie": staleCap + "; " + capabilityCookie})
	if dup.status != http.StatusOK {
		t.Fatalf("duplicate-cookie request status = %d, want %d (stale cookie must not shadow the valid one)", dup.status, http.StatusOK)
	}

	// 4. Ownership restoration: every proxied request ran the session-ownership
	// check as the real owner (default-user), never as a synthetic or empty
	// identity.
	checkerMu.Lock()
	owners := append([]string(nil), seenOwners...)
	checkerMu.Unlock()
	if len(owners) == 0 {
		t.Fatal("session-ownership checker never ran")
	}
	for _, owner := range owners {
		if owner != "default-user" {
			t.Fatalf("ownership check ran as %q, want default-user", owner)
		}
	}

	// 5. A capability minted for another user is rejected at the ownership
	// boundary: requireConnectionAuth restores the foreign identity and the
	// checker denies it.
	intruder := gateway.PortProxyHandler.issueCapability("sess-auth", 5173,
		authn.Identity{UserID: "intruder", Role: authn.RoleAdmin})
	denied := request(http.MethodGet,
		"/port-proxy/sess-auth/5173/x?"+proxyCapabilityQueryParam+"="+intruder, nil)
	if denied.status != http.StatusNotFound {
		t.Fatalf("intruder capability status = %d, want %d (ownership denial)", denied.status, http.StatusNotFound)
	}
	checkerMu.Lock()
	lastOwner := seenOwners[len(seenOwners)-1]
	checkerMu.Unlock()
	if lastOwner != "intruder" {
		t.Fatalf("intruder request ran ownership check as %q, want intruder", lastOwner)
	}

	// 6. The same manifest without any credential still 401s — the capability
	// is not a blanket bypass.
	noCred := request(http.MethodGet, "/port-proxy/sess-auth/5173/manifest.webmanifest", nil)
	if noCred.status != http.StatusUnauthorized {
		t.Fatalf("manifest (no credential) status = %d, want %d", noCred.status, http.StatusUnauthorized)
	}

	// 7. A module script carrying only the capability cookie (Path-scoped, no
	// session cookie) also passes — covers cookie-sending subresource fetches
	// in contexts that lack the session cookie.
	capValue := capabilityCookie
	if idx := strings.Index(capValue, ";"); idx >= 0 {
		capValue = capValue[:idx]
	}
	_, capToken, _ := strings.Cut(capValue, "=")
	script := request(http.MethodGet, "/port-proxy/sess-auth/5173/src/main.tsx",
		map[string]string{"Cookie": proxyCapabilityCookieName + "=" + capToken})
	if script.status != http.StatusOK {
		t.Fatalf("module script (capability cookie, no session) status = %d, want %d (body=%s)",
			script.status, http.StatusOK, script.body)
	}

	// 8. The runtime shim is served by the gateway at its reserved path, with
	// the prefix and capability baked in, so an app CSP allowing 'self' scripts
	// does not block the proxy's runtime fixes.
	shim := request(http.MethodGet, "/port-proxy/sess-auth/5173/__kandev_runtime_shim.js", cookieHeader)
	if shim.status != http.StatusOK {
		t.Fatalf("runtime shim status = %d, want %d", shim.status, http.StatusOK)
	}
	if ct := shim.headers.Get("Content-Type"); !strings.Contains(ct, "application/javascript") {
		t.Fatalf("runtime shim Content-Type = %q, want application/javascript", ct)
	}
	if !strings.Contains(shim.body, `var P="/port-proxy/sess-auth/5173";`) {
		t.Fatalf("runtime shim missing the baked-in prefix:\n%.300s", shim.body)
	}
	if shim.headers.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("runtime shim Cache-Control = %q, want private, no-store", shim.headers.Get("Cache-Control"))
	}
	// The shim body is also the exact JS served for the injected <script src>.
	if !strings.Contains(doc.body, "/port-proxy/sess-auth/5173/__kandev_runtime_shim.js") {
		t.Fatalf("document does not inject the external runtime shim:\n%.500s", doc.body)
	}

	// 9. Redirect Location headers are re-anchored on the proxy subtree.
	redir := request(http.MethodGet, "/port-proxy/sess-auth/5173/redirect", cookieHeader)
	if redir.status != http.StatusFound {
		t.Fatalf("redirect status = %d, want %d", redir.status, http.StatusFound)
	}
	if got := redir.headers.Get("Location"); got != "/port-proxy/sess-auth/5173/landing" {
		t.Fatalf("redirect Location = %q, want the proxied landing path", got)
	}

	// 10. A cookie-authenticated request's own ?token= parameter is preserved
	// for the app — only the gateway-consumed credential is stripped.
	appToken := request(http.MethodGet,
		"/port-proxy/sess-auth/5173/src/main.tsx?token=app-own-token&x=1", cookieHeader)
	if appToken.status != http.StatusOK {
		t.Fatalf("app-token request status = %d, want %d", appToken.status, http.StatusOK)
	}
	forwarded, _ = recordings.snapshot()
	last := forwarded[len(forwarded)-1]
	if !strings.Contains(last, "token=app-own-token") {
		t.Fatalf("cookie-authenticated app token was stripped from forwarded request %q", last)
	}
	if !strings.Contains(last, "x=1") {
		t.Fatalf("unrelated query param lost in forwarded request %q", last)
	}
}

// A browser request carrying Accept-Encoding: gzip must not corrupt the
// proxied HTML: the proxy drops the outbound header so Go's Transport
// auto-decompresses, and the rewriter sees identity bytes. The response is
// the REWRITTEN uncompressed HTML with no Content-Encoding.
func TestPortProxyDecompressesAndRewritesCompressedHTML(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	html := `<!doctype html><html><head><link rel="manifest" href="/m.webmanifest"></head><body>gzip</body></html>`
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			w.Header().Set("Content-Encoding", "gzip")
			var buf bytes.Buffer
			gz := gzip.NewWriter(&buf)
			_, _ = gz.Write([]byte(html))
			_ = gz.Close()
			w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
			_, _ = w.Write(buf.Bytes())
			return
		}
		_, _ = io.WriteString(w, html)
	}))
	t.Cleanup(app.Close)

	svc, sessionToken := newProxyAuthService(t)
	agentctl, _ := newAuthEnforcingFakeAgentctl(t, app.URL, "agentctl-token")
	manager := newProxyTestManager(t, log)
	manager.SetSessionAccessChecker(func(ctx context.Context, sessionID string) error {
		identity, ok := authn.IdentityFromContext(ctx)
		if !ok || identity.UserID != "default-user" {
			return errors.New("foreign session")
		}
		return nil
	})
	addExecutionForURL(t, manager, "sess-gz", agentctl.URL, "agentctl-token", log)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(authhttpmw.Middleware(svc))
	gateway := NewGateway(log)
	gateway.SetAuthPolicy(AuthPolicy{
		Enforced:     func() bool { return svc.Mode() != auth.ModeDisabled },
		ResolveToken: svc.ResolveBearer,
	})
	gateway.SetPortProxy(manager)
	gateway.SetupRoutes(router)

	request, srvURL := proxyAuthGateway(t, router)
	headers := map[string]string{
		"Cookie":          scopedSessionCookieHeader(t, srvURL, sessionToken)["Cookie"],
		"Accept-Encoding": "gzip",
	}
	doc := request(http.MethodGet, "/port-proxy/sess-gz/5173/", headers)
	if doc.status != http.StatusOK {
		t.Fatalf("document status = %d, want %d (body=%s)", doc.status, http.StatusOK, doc.body)
	}
	if got := doc.headers.Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want none (body must be decompressed before rewriting)", got)
	}
	// The body must be the REWRITTEN uncompressed HTML, not gzip bytes.
	if strings.Contains(doc.body, "\x1f\x8b") {
		t.Fatalf("body still gzip-compressed after proxying:\n%q", doc.body[:min(len(doc.body), 40)])
	}
	mustContain(t, doc.body, `/port-proxy/sess-gz/5173/m.webmanifest?kandev_cap=`)
	mustContain(t, doc.body, `<body>gzip</body>`)
}

// Nested standalone CSS served through the real proxy wiring resolves ../ and
// relative references against the stylesheet's PUBLIC directory (stashed at
// the handler before the path is rewritten to the agentctl form).
func TestPortProxyRewritesNestedCSSWithPublicBase(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	css := `body{background:url(../img.png)}a{background:url(rel.png)}`
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/css/main.css":
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
			_, _ = io.WriteString(w, css)
		default:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, `<!doctype html><html><body>app</body></html>`)
		}
	}))
	t.Cleanup(app.Close)

	svc, sessionToken := newProxyAuthService(t)
	agentctl, _ := newAuthEnforcingFakeAgentctl(t, app.URL, "agentctl-token")
	manager := newProxyTestManager(t, log)
	manager.SetSessionAccessChecker(func(ctx context.Context, sessionID string) error {
		identity, ok := authn.IdentityFromContext(ctx)
		if !ok || identity.UserID != "default-user" {
			return errors.New("foreign session")
		}
		return nil
	})
	addExecutionForURL(t, manager, "sess-css", agentctl.URL, "agentctl-token", log)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(authhttpmw.Middleware(svc))
	gateway := NewGateway(log)
	gateway.SetAuthPolicy(AuthPolicy{
		Enforced:     func() bool { return svc.Mode() != auth.ModeDisabled },
		ResolveToken: svc.ResolveBearer,
	})
	gateway.SetPortProxy(manager)
	gateway.SetupRoutes(router)

	request, srvURL := proxyAuthGateway(t, router)
	headers := map[string]string{"Cookie": scopedSessionCookieHeader(t, srvURL, sessionToken)["Cookie"]}
	doc := request(http.MethodGet, "/port-proxy/sess-css/5173/css/main.css", headers)
	if doc.status != http.StatusOK {
		t.Fatalf("css status = %d, want %d (body=%s)", doc.status, http.StatusOK, doc.body)
	}
	// ../img.png resolves to the public P/img.png (in-subtree) and caps;
	// rel.png resolves inside the css directory and caps.
	mustContain(t, doc.body, `url(../img.png?kandev_cap=`)
	mustContain(t, doc.body, `url(rel.png?kandev_cap=`)
}

// TestPortProxyCapabilityRoundTrip covers the signed capability itself:
// valid capabilities validate, and tampering, expiry, or a mismatched
// session:port target are all rejected.
func TestPortProxyCapabilityRoundTrip(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	handler := NewPortProxyHandler(newProxyTestManager(t, log), log)

	identity := authn.Identity{UserID: "user-1", Role: authn.RoleAdmin}
	cap := handler.issueCapability("sess-1", 5173, identity)

	if got, ok := handler.validateCapability(cap, "sess-1", 5173); !ok {
		t.Fatal("fresh capability did not validate")
	} else if got.UserID != "user-1" {
		t.Fatalf("validated identity UserID = %q, want %q", got.UserID, "user-1")
	}

	// Wrong session or port: rejected (the credential is subtree-scoped).
	if _, ok := handler.validateCapability(cap, "sess-2", 5173); ok {
		t.Fatal("capability validated for the wrong session")
	}
	if _, ok := handler.validateCapability(cap, "sess-1", 5174); ok {
		t.Fatal("capability validated for the wrong port")
	}

	// Tampered payload: rejected. Flip the final byte to a value guaranteed to
	// differ from the original, rather than a fixed literal: the mac suffix is
	// hex, so appending a fixed "00" is a ~1/256 no-op whenever the genuine
	// suffix already ends that way, which flaked this test in CI.
	lastIdx := len(cap) - 1
	replacement := byte('0')
	if cap[lastIdx] == '0' {
		replacement = '1'
	}
	tampered := cap[:lastIdx] + string(replacement)
	if _, ok := handler.validateCapability(tampered, "sess-1", 5173); ok {
		t.Fatal("tampered capability validated")
	}

	// Expired: rejected.
	old := handler.issueCapabilityWithTTL("sess-1", 5173, identity, -time.Minute)
	if _, ok := handler.validateCapability(old, "sess-1", 5173); ok {
		t.Fatal("expired capability validated")
	}

	// Exact boundary (now == ExpiresAt): expired per standard semantics.
	atBoundary := handler.issueCapabilityWithTTL("sess-1", 5173, identity, 0)
	if _, ok := handler.validateCapability(atBoundary, "sess-1", 5173); ok {
		t.Fatal("capability at its expiry boundary validated")
	}

	// Garbage: rejected without panicking.
	if _, ok := handler.validateCapability("not-a-capability", "sess-1", 5173); ok {
		t.Fatal("garbage capability validated")
	}
}
