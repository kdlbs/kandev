package websocket

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
)

// Port-proxy capability credential.
//
// With authentication enforced, every request to /port-proxy/:sessionId/:port/*
// requires a credential (session cookie, PAT, or ?token=). The preview document
// carries one, but the browser's subresource fetches do not always: <link
// rel="manifest"> fetches are made with credentials omitted, so they never send
// the session cookie, and headerless contexts (sandboxed iframes, ?token= URLs)
// drop it from other requests too. A document that authenticated would 200
// while every asset 401s — the preview appears broken.
//
// The handler therefore mints a short-lived, path-scoped capability after the
// document authenticates and propagates it two ways: as a cookie scoped to the
// proxy subtree (covers cookie-sending subresource fetches) and appended to the
// rewritten asset URLs in proxied HTML/CSS (covers the credential-less manifest
// fetch and cookie-less contexts). requireConnectionAuth accepts either form
// for the matching /port-proxy/ subtree and reconstructs the identity, so
// CheckSessionAccess still gates on the real owner.
const (
	// proxyCapabilityCookieName is the cookie carrying the subtree capability.
	proxyCapabilityCookieName = "kandev_port_proxy"
	// proxyCapabilityQueryParam is the query parameter appended to rewritten
	// asset URLs by rewriteProxyResponse.
	proxyCapabilityQueryParam = "kandev_cap"
	// proxyCapabilityTTL is how long a minted capability stays valid. Sliding:
	// every proxied response mints a fresh one. Previews opened with a durable
	// session cookie re-authenticate indefinitely; a preview that relies
	// solely on the capability (no cookie storage) is deliberately short-lived
	// — after an idle proxyCapabilityTTL the capability expires and the
	// subtree requires a fresh credential.
	proxyCapabilityTTL = 15 * time.Minute
)

// proxyCapabilityContextKey carries the capability for the current request from
// the handler into the cached proxy's ModifyResponse, which appends it to
// rewritten asset URLs.
type proxyCapabilityContextKey struct{}

// proxyCSSBaseContextKey carries the PUBLIC directory (proxy prefix + app
// path directory, trailing slash) that a standalone CSS response resolves
// relative references against. The request path is rewritten to the
// agentctl form before the proxy runs, so the public path must be captured
// at the handler.
type proxyCSSBaseContextKey struct{}

// publicCSSBase returns the public directory (trailing slash) that a
// standalone CSS file at the given public path resolves relative references
// against: proxyPrefix + the directory of the app-relative path.
func publicCSSBase(proxyPrefix, remaining string) string {
	if remaining == "" {
		return ""
	}
	dir := path.Dir(remaining)
	if dir == "." || dir == "/" {
		return proxyPrefix + "/"
	}
	return proxyPrefix + "/" + strings.TrimPrefix(dir, "/") + "/"
}

// proxyTokenConsumedKey marks that requireConnectionAuth authenticated this
// request via ?token=<PAT>, so the port proxy strips the token from the
// forwarded query. When absent, a ?token= parameter belongs to the app and is
// forwarded untouched.
type proxyTokenConsumedKey struct{}

// portProxyEntry caches a reverse proxy and its target for a session:port pair.
type portProxyEntry struct {
	proxy  *httputil.ReverseProxy
	target string
}

// PortProxyHandler reverse-proxies HTTP and WebSocket traffic to arbitrary
// localhost ports inside a remote executor via agentctl.
type PortProxyHandler struct {
	lifecycleMgr *lifecycle.Manager
	logger       *logger.Logger

	// capabilitySecret signs the subtree capability cookie/query credentials.
	// Per-process random: capabilities are short-lived, so a backend restart
	// simply invalidates them.
	capabilitySecret [32]byte

	mu      sync.Mutex
	proxies map[string]*portProxyEntry // key: "sessionId:port"
}

// NewPortProxyHandler creates a new port proxy handler.
func NewPortProxyHandler(lifecycleMgr *lifecycle.Manager, log *logger.Logger) *PortProxyHandler {
	secret := [32]byte{}
	if _, err := rand.Read(secret[:]); err != nil {
		// crypto/rand never fails on supported platforms; a deterministic
		// fallback would make capabilities forgeable, so fail loudly instead
		// of serving an auth bypass.
		panic(fmt.Sprintf("port proxy: generate capability secret: %v", err))
	}
	return &PortProxyHandler{
		lifecycleMgr:     lifecycleMgr,
		logger:           log.WithFields(zap.String("component", "port-proxy")),
		capabilitySecret: secret,
		proxies:          make(map[string]*portProxyEntry),
	}
}

// HandlePortProxy handles all HTTP/WS requests to /port-proxy/:sessionId/:port/*path.
func (h *PortProxyHandler) HandlePortProxy(c *gin.Context) {
	sessionID := c.Param("sessionId")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sessionId is required"})
		return
	}

	// Per-user scoping (opt-in auth): authorize session ownership before the
	// bare execution lookup / per-session proxy cache (see vscode_proxy.go).
	if err := h.lifecycleMgr.CheckSessionAccess(c.Request.Context(), sessionID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	portStr := c.Param("port")
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1024 || port > 65535 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid port: must be 1024-65535"})
		return
	}

	// After the request authenticated (session cookie, PAT, ?token=, or a
	// previously issued subtree capability), mint a fresh short-lived
	// capability for this session:port subtree. It authorizes the browser's
	// subresource fetches — which cannot always carry the session credential —
	// for the next proxyCapabilityTTL, sliding with every proxied response.
	// Synthetic identities (auth disabled) skip it: with no auth anywhere the
	// subtree needs no credential, and the proxied bodies stay byte-identical
	// to the pre-auth behavior.
	capability := ""
	if identity, ok := authn.FromGin(c); ok && !identity.Synthetic {
		capability = h.issueCapability(sessionID, port, identity)
		h.setCapabilityCookie(c, sessionID, port, capability)
		ctx := context.WithValue(c.Request.Context(), proxyCapabilityContextKey{}, capability)
		c.Request = c.Request.WithContext(ctx)
	}

	proxy, err := h.resolveProxy(c, sessionID, port)
	if err != nil {
		return // error already written to response
	}

	// Rewrite path: strip /port-proxy/:sessionId/:port prefix,
	// forward as /api/v1/port-proxy/:port/{remainingPath} to agentctl.
	originalPath := c.Request.URL.Path
	prefix := "/port-proxy/" + sessionID + "/" + portStr
	remaining := strings.TrimPrefix(originalPath, prefix)
	if remaining == "" {
		remaining = "/"
	}

	// The runtime shim is served by the gateway itself (same-origin, so an
	// app Content-Security-Policy that allows 'self' scripts lets it load),
	// not by the proxied app.
	if remaining == "/"+runtimeShimPath {
		h.serveRuntimeShim(c, prefix, capability)
		return
	}

	c.Request.URL.Path = "/api/v1/port-proxy/" + portStr + remaining
	c.Request.URL.RawPath = ""

	// The request path is rewritten to the agentctl form above, so the
	// public path is unrecoverable at ModifyResponse time. Stash the PUBLIC
	// stylesheet directory for standalone CSS rewriting: relative CSS
	// references resolve against the stylesheet file's public directory.
	if dir := publicCSSBase(prefix, remaining); dir != "" {
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), proxyCSSBaseContextKey{}, dir))
	}

	h.logger.Debug("port proxy forwarding",
		zap.String("session_id", sessionID),
		zap.Int("port", port),
		zap.String("original_path", originalPath),
		zap.String("rewritten_path", c.Request.URL.Path))

	defer func() {
		if r := recover(); r != nil {
			if r == http.ErrAbortHandler {
				h.logger.Debug("port proxy: client disconnected",
					zap.String("session_id", sessionID),
					zap.Int("port", port))
				return
			}
			panic(r)
		}
	}()

	proxy.ServeHTTP(c.Writer, c.Request)
}

func (h *PortProxyHandler) resolveProxy(c *gin.Context, sessionID string, port int) (*httputil.ReverseProxy, error) {
	cacheKey := sessionID + ":" + strconv.Itoa(port)

	h.mu.Lock()
	defer h.mu.Unlock()

	if entry, ok := h.proxies[cacheKey]; ok {
		return entry.proxy, nil
	}

	execution, ok := h.lifecycleMgr.GetExecutionBySessionID(sessionID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found or no active execution"})
		return nil, fmt.Errorf("session not found")
	}

	agentctlClient, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if agentctlClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "agentctl client not available"})
		return nil, fmt.Errorf("agentctl client not available")
	}

	baseURL := agentctlClient.BaseURL()
	target, err := url.Parse(baseURL)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to resolve agentctl target"})
		return nil, err
	}

	authToken := agentctlClient.AuthToken()
	// Public path that fronts this proxy on the gateway. Used by `ModifyResponse`
	// to rewrite root-absolute URLs in proxied HTML/CSS so iframe asset requests
	// stay on the same proxy chain instead of escaping to the host origin.
	proxyPrefix := "/port-proxy/" + sessionID + "/" + strconv.Itoa(port)
	proxy := h.createProxy(cacheKey, target, authToken, proxyPrefix)
	h.proxies[cacheKey] = &portProxyEntry{proxy: proxy, target: baseURL}

	h.logger.Info("created port proxy",
		zap.String("session_id", sessionID),
		zap.Int("port", port),
		zap.String("execution_id", execution.ID),
		zap.String("target", baseURL))
	return proxy, nil
}

func (h *PortProxyHandler) createProxy(cacheKey string, target *url.URL, authToken, proxyPrefix string) *httputil.ReverseProxy {
	proxy := &httputil.ReverseProxy{}
	proxy.Rewrite = func(r *httputil.ProxyRequest) {
		r.SetURL(target)
		r.Out.URL.Path = r.In.URL.Path
		r.Out.URL.RawPath = ""
		// Never forward the browser's Accept-Encoding: Go's Transport adds
		// its own and transparently decompresses, so ModifyResponse (and the
		// HTML/CSS rewriter) always sees identity bytes. Forwarding gzip/br
		// would deliver compressed bytes to the rewriter, which then strips
		// Content-Encoding while keeping the compressed body.
		r.Out.Header.Del("Accept-Encoding")
		// The subtree capability is consumed by requireConnectionAuth at the
		// gateway; strip it (in any encoding) before forwarding so it never
		// lands in agentctl or application logs, redirects, or analytics. Only
		// the reserved pairs are removed — every other parameter keeps its
		// original bytes and order. The ?token= PAT is stripped only when the
		// gateway actually consumed it for authentication; a cookie-authenticated
		// app's own token parameter passes through.
		if consumed, _ := r.In.Context().Value(proxyTokenConsumedKey{}).(bool); consumed {
			r.Out.URL.RawQuery = stripReservedProxyParams(r.Out.URL.RawQuery)
		} else {
			r.Out.URL.RawQuery = stripCapabilityParam(r.Out.URL.RawQuery)
		}
		// The browser may send a rewritten asset URL (which embeds the
		// capability, possibly percent-encoded) as the Referer of a later
		// request; sanitize every Referer. Capability-free Referers are left
		// byte-identical; malformed ones are dropped rather than forwarded
		// with their query intact.
		if ref := r.Out.Header.Get("Referer"); ref != "" {
			if parsed, err := url.Parse(ref); err == nil {
				cleaned := stripReservedProxyParams(parsed.RawQuery)
				if cleaned != parsed.RawQuery {
					parsed.RawQuery = cleaned
					r.Out.Header.Set("Referer", parsed.String())
				}
			} else {
				r.Out.Header.Del("Referer")
			}
		}
		// Inject agentctl auth token
		if authToken != "" {
			r.Out.Header.Set("Authorization", "Bearer "+authToken)
		}
		if r.Out.Header.Get("Upgrade") != "" {
			r.Out.Header.Set("Connection", "Upgrade")
		}
	}

	proxy.ModifyResponse = func(resp *http.Response) error {
		filterReservedCookies(resp.Header)
		capability, _ := resp.Request.Context().Value(proxyCapabilityContextKey{}).(string)
		if capability != "" {
			// Every response in an authenticated proxy subtree carries the
			// per-user capability cookie, and rewritten bodies embed the
			// capability: a shared intermediary must not retain or replay them
			// for another user, and the browser must not leak the embedded
			// capability to external origins through the Referer header. This
			// applies to every response class (JS/JSON/redirects/upgrades
			// included), not just the HTML/CSS bodies the rewriter touches.
			resp.Header.Set("Cache-Control", "private, no-store")
			resp.Header.Set("Referrer-Policy", "no-referrer")
		}
		// Redirect Location headers are root-relative to the proxied app; the
		// browser resolves them against the gateway origin, so they must be
		// re-anchored on the proxy subtree (navigation: no capability).
		if loc := resp.Header.Get("Location"); loc != "" {
			resp.Header.Set("Location", rewriteURLReference(loc, proxyPrefix, ""))
		}
		if resp.StatusCode == http.StatusSwitchingProtocols {
			resp.Header.Set("Connection", "Upgrade")
			return nil
		}
		if resp.Request.Method == http.MethodHead {
			// HEAD responses carry no body; rewriting would synthesize a
			// misleading Content-Length for the would-be GET body.
			return nil
		}
		return rewriteProxyResponse(resp, proxyPrefix, capability)
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		h.logger.Error("port proxy error",
			zap.String("cache_key", cacheKey),
			zap.String("request_path", r.URL.Path),
			zap.Error(err))
		h.invalidateProxy(cacheKey)
		http.Error(w, "port proxy error", http.StatusBadGateway)
	}

	// Flush immediately for SSE/streaming responses.
	proxy.FlushInterval = -1

	return proxy
}

func (h *PortProxyHandler) invalidateProxy(cacheKey string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.proxies, cacheKey)
}

// InvalidateSession removes all cached proxies for a session.
func (h *PortProxyHandler) InvalidateSession(sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	prefix := sessionID + ":"
	for key := range h.proxies {
		if strings.HasPrefix(key, prefix) {
			delete(h.proxies, key)
		}
	}
}

// capabilityPayload is the signed, tamper-evident body of a subtree capability.
// The identity travels inside the payload so requireConnectionAuth can restore
// it and CheckSessionAccess still runs as the real owner.
type capabilityPayload struct {
	UserID    string `json:"u"`
	Role      string `json:"r"`
	SessionID string `json:"s"`
	Port      int    `json:"p"`
	ExpiresAt int64  `json:"e"` // unix seconds
}

// issueCapability mints a signed capability for the session:port subtree with
// the default TTL.
func (h *PortProxyHandler) issueCapability(sessionID string, port int, identity authn.Identity) string {
	return h.issueCapabilityWithTTL(sessionID, port, identity, proxyCapabilityTTL)
}

// issueCapabilityWithTTL mints a signed capability with an explicit TTL (test
// seam for expiry coverage).
func (h *PortProxyHandler) issueCapabilityWithTTL(sessionID string, port int, identity authn.Identity, ttl time.Duration) string {
	payload := capabilityPayload{
		UserID:    identity.UserID,
		Role:      string(identity.Role),
		SessionID: sessionID,
		Port:      port,
		ExpiresAt: time.Now().Add(ttl).Unix(),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		// The payload is a fixed shape; marshal cannot fail.
		return ""
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	mac := h.signCapability(raw)
	return encoded + "." + mac
}

// validateCapability verifies the signature, expiry, and session:port target of
// a capability and returns the identity it was issued to.
func (h *PortProxyHandler) validateCapability(raw, sessionID string, port int) (authn.Identity, bool) {
	dot := strings.IndexByte(raw, '.')
	if dot <= 0 || dot == len(raw)-1 {
		return authn.Identity{}, false
	}
	encoded, mac := raw[:dot], raw[dot+1:]
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return authn.Identity{}, false
	}
	if !hmac.Equal([]byte(h.signCapability(payload)), []byte(mac)) {
		return authn.Identity{}, false
	}
	var parsed capabilityPayload
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return authn.Identity{}, false
	}
	if parsed.SessionID != sessionID || parsed.Port != port {
		return authn.Identity{}, false
	}
	// Standard expiry semantics: the capability is valid strictly before
	// ExpiresAt; at the exact boundary (now == ExpiresAt) it is expired.
	if time.Now().Unix() >= parsed.ExpiresAt {
		return authn.Identity{}, false
	}
	return authn.Identity{UserID: parsed.UserID, Role: authn.Role(parsed.Role)}, true
}

// signCapability computes the HMAC-SHA256 of the capability payload.
func (h *PortProxyHandler) signCapability(payload []byte) string {
	mac := hmac.New(sha256.New, h.capabilitySecret[:])
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// cookieName extracts the cookie name from a Set-Cookie header value (the
// token before the first '='), trimming leading/trailing whitespace. An
// empty or malformed value yields "".
func cookieName(setCookie string) string {
	name, _, _ := strings.Cut(setCookie, "=")
	return strings.TrimSpace(name)
}

// filterReservedCookies removes every upstream Set-Cookie whose name is the
// reserved capability cookie: a proxied app must never control
// kandev_port_proxy, since an app value with the same or a narrower Path
// could override the gateway's capability and 401 the authenticated subtree.
func filterReservedCookies(h http.Header) {
	cookies := h["Set-Cookie"]
	if len(cookies) == 0 {
		return
	}
	kept := cookies[:0]
	for _, c := range cookies {
		if cookieName(c) == proxyCapabilityCookieName {
			continue
		}
		kept = append(kept, c)
	}
	if len(kept) == 0 {
		h.Del("Set-Cookie")
		return
	}
	h["Set-Cookie"] = kept
}

// setCapabilityCookie sets the subtree-scoped capability cookie on the response
// so the browser's cookie-sending subresource fetches stay authorized. The
// cookie's Path is exactly the proxy subtree, so it is never presented to (or
// accepted for) any other path.
func (h *PortProxyHandler) setCapabilityCookie(c *gin.Context, sessionID string, port int, capability string) {
	if capability == "" {
		return
	}
	path := "/port-proxy/" + sessionID + "/" + strconv.Itoa(port)
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(proxyCapabilityCookieName, capability, int(proxyCapabilityTTL.Seconds()), path, "", requestIsTLS(c), true)
}

// requestIsTLS reports whether the request arrived over TLS, directly or via a
// reverse proxy setting X-Forwarded-Proto. Mirrors the auth cookie helper so
// the Secure flag tracks the session cookie.
func requestIsTLS(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	return strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
}

// proxyTokenQueryParam is the ?token=<PAT> query credential requireConnectionAuth
// accepts on proxy routes. Like the capability, it is consumed at the gateway
// and must never be forwarded to the proxied app.
const proxyTokenQueryParam = "token"

// stripCapabilityParam removes the capability query parameter from a raw query
// string. See stripReservedProxyParams.
func stripCapabilityParam(rawQuery string) string {
	return stripQueryParam(rawQuery, proxyCapabilityQueryParam)
}

// stripQueryParam removes the named query parameter from a raw query string,
// matching the decoded key so percent-encoded spellings are caught too.
func stripQueryParam(rawQuery, name string) string {
	if rawQuery == "" {
		return rawQuery
	}
	parts := strings.Split(rawQuery, "&")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		key, _, _ := strings.Cut(part, "=")
		if decoded, err := url.QueryUnescape(key); err == nil {
			key = decoded
		}
		if key != name {
			out = append(out, part)
		}
	}
	return strings.Join(out, "&")
}

// stripReservedProxyParams removes every reserved gateway credential pair
// (kandev_cap, token) from a raw query string, matching the decoded key so
// percent-encoded spellings (%6bandev_cap=) are caught too. Every other
// parameter keeps its original bytes and order — nothing is re-encoded.
// Returns the input unchanged when no reserved pair is present.
//
// The grammar is "&"-separated pairs, which is the only form the gateway ever
// issues (the rewriter and runtime shim append with "?"/"&") and the only form
// the gateway's own query parser accepts as a credential; semicolon- or
// space-separated spellings cannot carry a gateway-issued capability, so they
// are out of scope here.
func stripReservedProxyParams(rawQuery string) string {
	if rawQuery == "" {
		return rawQuery
	}
	parts := strings.Split(rawQuery, "&")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		key, _, _ := strings.Cut(part, "=")
		if decoded, err := url.QueryUnescape(key); err == nil {
			key = decoded
		}
		if key != proxyCapabilityQueryParam && key != proxyTokenQueryParam {
			out = append(out, part)
		}
	}
	return strings.Join(out, "&")
}

// portProxyTarget extracts the session:port subtree from a request handled by
// the /port-proxy/:sessionId/:port(/...) routes.
func portProxyTarget(c *gin.Context) (sessionID string, port int, ok bool) {
	sessionID = c.Param("sessionId")
	portStr := c.Param("port")
	if sessionID == "" || portStr == "" {
		return "", 0, false
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1024 || port > 65535 {
		return "", 0, false
	}
	return sessionID, port, true
}

// serveRuntimeShim answers the reserved per-subtree runtime shim path with the
// shim JavaScript, prefix and capability baked in. Served same-origin by the
// gateway so an app Content-Security-Policy allowing 'self' scripts does not
// block the proxy's runtime fixes; never cached (per-user body).
func (h *PortProxyHandler) serveRuntimeShim(c *gin.Context, prefix, capability string) {
	js := runtimeShim(prefix, capability)
	c.Header("Content-Type", "application/javascript; charset=utf-8")
	c.Header("Cache-Control", "private, no-store")
	c.Header("Referrer-Policy", "no-referrer")
	c.Data(http.StatusOK, "application/javascript; charset=utf-8", []byte(js))
}
