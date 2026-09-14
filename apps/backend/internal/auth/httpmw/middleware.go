// Package httpmw is the global HTTP enforcement middleware for opt-in
// authentication. It runs after CORS on every request (see
// backendapp.buildHTTPServer) and implements the allowlist policy from
// docs/specs/auth: identity injection in disabled mode, credential resolution
// (session cookie, then PAT bearer), self-authenticating callback and webhook
// passthrough, the office agent-JWT deferral, and SPA-shell availability for
// the login page.
//
// CSRF note: cross-origin browser requests are already rejected by
// backendapp.corsMiddleware (httpmw.AllowedOrigin) before this middleware
// runs, and the session cookie is SameSite=Lax — no separate origin check is
// required here.
package httpmw

import (
	"crypto/sha256"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/auth"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	userstore "github.com/kandev/kandev/internal/user/store"
)

// Middleware returns the global auth gin middleware.
func Middleware(svc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		mode := svc.Mode()
		if mode == auth.ModeDisabled {
			// Opt-out path: inject the synthetic single-user admin identity so
			// downstream code is identity-aware with unchanged behavior.
			authn.SetOnGin(c, SyntheticIdentity())
			c.Next()
			return
		}
		if identity, ok := ResolveRequest(c, svc); ok {
			authn.SetOnGin(c, identity)
			c.Next()
			return
		}
		path := c.Request.URL.Path
		if isPublicPath(c.Request.Method, path) || isDeferredPath(c, path) {
			c.Next()
			return
		}
		c.Header("WWW-Authenticate", "Bearer")
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
	}
}

// StripUntrustedForwardedHost removes X-Forwarded-Host from requests whose
// immediate peer is not a trusted proxy. It is a pure header-modifier: it
// never aborts, so gin continues to the next handler automatically once the
// function returns (no c.Next() call needed). Browsers never send
// X-Forwarded-Host; only a reverse proxy in front of the backend should, and
// only to carry the browser's original host:port through a port-rewrite to
// the port-scoped cookie-name resolver (httpcookie.PortSuffix). Without this
// gate a non-browser client could pick its own forwarded host, and with it
// its own port-scoped cookie name, by setting the header directly. The same
// trusted list that gates X-Forwarded-For (gin's ClientIP via
// SetTrustedProxies) gates this header so the two trust decisions cannot
// diverge; an untrusted value is dropped (warned once per distinct peer and
// host, see forwardedHostWarnSet) and the resolver falls back to the request
// Host. backendapp passes the list returned by its
// configureTrustedProxies, so the two uses share one configuration.
func StripUntrustedForwardedHost(trusted []string, log *logger.Logger) gin.HandlerFunc {
	matcher := newTrustedProxyMatcher(trusted)
	warned := newForwardedHostWarnSet()
	return func(c *gin.Context) {
		forwardedHost := c.GetHeader("X-Forwarded-Host")
		if forwardedHost == "" {
			return
		}
		peer := remoteAddrHost(c.Request.RemoteAddr)
		if matcher.contains(peer) {
			return
		}
		if warned.first(peer, forwardedHost) {
			log.Warn("ignoring X-Forwarded-Host from untrusted peer",
				zap.String("peer", peer),
				zap.String("forwarded_host", truncateForLog(forwardedHost)),
				zap.String("hint", forwardedHostWarnHint))
		}
		c.Request.Header.Del("X-Forwarded-Host")
	}
}

// forwardedHostWarnHint tells the operator how to fix the common cause: a real
// reverse proxy missing from the trusted list. The warning is a configuration
// signal, not a per-request event, so the fix travels with it.
const forwardedHostWarnHint = "set KANDEV_TRUSTED_PROXIES to this peer's IP or CIDR if it is your reverse proxy"

// forwardedHostWarnLimit bounds the distinct (peer, forwarded host) pairs
// remembered for warn deduplication. A misconfigured proxy produces one pair;
// the cap keeps a client that varies either value from growing the set without
// bound, at the cost of only losing warnings once it is reached.
const forwardedHostWarnLimit = 64

// forwardedHostLogMaxLen caps the forwarded-host bytes written to the log. The
// header is unauthenticated and may run to the server's whole header budget
// (backendapp leaves MaxHeaderBytes at net/http's 1 MiB default), and dumping a
// megabyte per line would recreate, in one line, the flooding this
// deduplication exists to stop. The prefix is enough to recognize a hostname.
const forwardedHostLogMaxLen = 256

// truncateForLog shortens an attacker-controlled value to a bounded, readable
// prefix, cutting on a rune boundary so the log field stays valid UTF-8.
func truncateForLog(value string) string {
	if len(value) <= forwardedHostLogMaxLen {
		return value
	}
	cut := forwardedHostLogMaxLen
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return value[:cut] + "...(truncated)"
}

// forwardedHostWarnSet deduplicates the strip warning. The stripped header is
// a static property of the deployment (an untrusted proxy keeps forwarding on
// every request), so warning per request buries the rest of the log in
// thousands of identical lines while adding nothing after the first. One
// warning per distinct (peer, forwarded host) pair keeps the operator signal
// and still surfaces a genuinely new peer or host.
//
// Keys are SHA-256 digests rather than the pair itself: the forwarded host is
// unauthenticated and can approach the server's whole header budget, so
// retaining the raw values would let 64 oversized requests pin tens of MiB for
// the process lifetime despite the entry cap. A fixed-size digest bounds
// retention at forwardedHostWarnLimit * 32 bytes whatever the header size.
type forwardedHostWarnSet struct {
	mu   sync.RWMutex
	seen map[[sha256.Size]byte]struct{}
}

func newForwardedHostWarnSet() *forwardedHostWarnSet {
	return &forwardedHostWarnSet{seen: make(map[[sha256.Size]byte]struct{})}
}

// forwardedHostWarnKey digests a (peer, forwarded host) pair into a fixed-size
// dedup key. The NUL separator cannot appear in either value, so no pair can
// forge another pair's digest by moving the boundary.
func forwardedHostWarnKey(peer, forwardedHost string) [sha256.Size]byte {
	digest := sha256.New()
	digest.Write([]byte(peer))
	digest.Write([]byte{0})
	digest.Write([]byte(forwardedHost))
	var key [sha256.Size]byte
	copy(key[:], digest.Sum(nil))
	return key
}

// first reports whether this (peer, forwarded host) pair has not been warned
// about yet, recording it when so. It returns false once the pair is known or
// the set is full.
func (s *forwardedHostWarnSet) first(peer, forwardedHost string) bool {
	key := forwardedHostWarnKey(peer, forwardedHost)
	s.mu.RLock()
	_, seen := s.seen[key]
	atLimit := len(s.seen) >= forwardedHostWarnLimit
	s.mu.RUnlock()
	if seen || atLimit {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.seen[key]; ok {
		return false
	}
	if len(s.seen) >= forwardedHostWarnLimit {
		return false
	}
	s.seen[key] = struct{}{}
	return true
}

// trustedProxyMatcher matches a peer IP against the trusted-proxy list (bare
// IPs and CIDRs, the same entries configureTrustedProxies handed to gin).
type trustedProxyMatcher struct {
	addresses []netip.Addr
	prefixes  []netip.Prefix
}

// newTrustedProxyMatcher indexes the trusted-proxy list (bare IPs and CIDRs)
// for O(1) peer matching. IPv4-mapped entries are normalized to their IPv4
// form so the matcher agrees with gin's trust decision (see contains).
func newTrustedProxyMatcher(trusted []string) *trustedProxyMatcher {
	m := &trustedProxyMatcher{}
	for _, entry := range trusted {
		if strings.Contains(entry, "/") {
			prefix, err := netip.ParsePrefix(entry)
			if err != nil {
				continue
			}
			if prefix.Addr().Is4In6() {
				// gin normalizes IPv4-mapped CIDRs to their IPv4 form
				// (::ffff:10.0.0.0/120 → 10.0.0.0/24, mask sliced 96 bits
				// shorter); mirror it so the two trust decisions cannot
				// diverge. A mapped-form CIDR with exactly 96 bits
				// degenerates to 0.0.0.0/0 in gin (trusts every IPv4 peer)
				// and is mirrored as such; with fewer than 96 bits gin's
				// entry matches no IPv4-form peer and the matcher skips it,
				// failing closed (unsupported configuration).
				prefix = netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()-96)
			}
			if prefix.IsValid() {
				m.prefixes = append(m.prefixes, prefix)
			}
			continue
		}
		if addr, err := netip.ParseAddr(entry); err == nil {
			m.addresses = append(m.addresses, addr.Unmap())
		}
	}
	return m
}

// contains reports whether host is one of the trusted addresses or inside one
// of the trusted prefixes. Unparsable peers (Unix sockets, empty RemoteAddr)
// are never trusted. IPv4-mapped IPv6 peers are unmapped so the matcher
// agrees with gin's trust decision (which compares the IPv4 form): if the two
// ever diverged, the strip would fail CLOSED (dropping a header gin trusts),
// breaking the port-rewrite cookie flow rather than security.
func (m *trustedProxyMatcher) contains(host string) bool {
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	for _, a := range m.addresses {
		if a == addr {
			return true
		}
	}
	for _, p := range m.prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// remoteAddrHost returns the host part of a RemoteAddr ("IP:port"), or the
// whole value when it has no port.
// remoteAddrHost returns the host part of a RemoteAddr ("IP:port"), or the
// whole value when it has no port.
func remoteAddrHost(remoteAddr string) string {
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	return remoteAddr
}

// SyntheticIdentity is the implicit identity used while auth is disabled.
func SyntheticIdentity() authn.Identity {
	return authn.Identity{UserID: userstore.DefaultUserID, Role: authn.RoleAdmin, Synthetic: true}
}

// ResolveRequest authenticates a request from its session cookie or PAT
// bearer. The WS gateway consumes the middleware-resolved identity on
// upgraded connections (it never calls this function); PAT ?token= upgrades
// go through AuthPolicy.ResolveToken instead. The resolved client IP rides
// the session touch so the stored IP tracks the client's current address.
func ResolveRequest(c *gin.Context, svc *auth.Service) (authn.Identity, bool) {
	ctx := c.Request.Context()
	if cookie, err := c.Cookie(svc.CookieNameForRequest(c.Request)); err == nil && cookie != "" {
		if identity, ok := svc.ResolveSessionToken(ctx, cookie, c.ClientIP()); ok {
			return identity, true
		}
	}
	if bearer := BearerToken(c.Request); bearer != "" {
		if identity, ok := svc.ResolveBearer(ctx, bearer); ok {
			return identity, true
		}
	}
	return authn.Identity{}, false
}

// BearerToken extracts an Authorization bearer credential.
func BearerToken(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if strings.HasPrefix(header, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	}
	return ""
}

// isPublicPath is the unauthenticated allowlist. Everything here is either
// pre-session bootstrap, a credential-issuing endpoint, or a
// self-authenticating webhook (own secret/HMAC validated by its handler).
// Plugin webhooks are NOT in this allowlist — whether a given plugin webhook
// is anonymous-callable depends on its manifest (webhooks[].public), which
// only plugins.Controller.webhook can read; see isPluginWebhookPath below.
func isPublicPath(method, path string) bool {
	switch path {
	case "/health":
		// CLI + desktop liveness probes poll before any session can exist.
		return method == http.MethodGet
	case "/ready":
		// Kubernetes readinessProbe and the e2e fixture poll before any
		// session can exist, same as /health.
		return method == http.MethodGet
	case "/api/v1/features", "/api/v1/app-state":
		// SPA bootstrap reads; app-state returns the auth-aware boot payload
		// (empty initialState + auth block when unauthenticated).
		return method == http.MethodGet
	case "/api/v1/auth/login", "/api/v1/auth/setup", "/api/v1/auth/invites/accept":
		return method == http.MethodPost
	case "/api/v1/auth/invites/preview":
		return method == http.MethodGet
	case "/api/v1/auth/me":
		// Returns {authenticated:false, mode} for anonymous visitors.
		return method == http.MethodGet
	case "/api/v1/github/credentials/resolve":
		// Opaque, task-scoped broker lease (SHA-256-hashed at rest, TTL'd,
		// scope-matched on redeem) is the credential — containers and remote
		// executors hold no session cookie or PAT by design. GET is the
		// readiness probe, POST redeems the lease; both self-authenticate
		// inside the handler, never off request identity.
		return method == http.MethodGet || method == http.MethodPost
	case "/api/v1/github/credentials/reissue":
		// The encrypted execution capability is the credential. The handler
		// checks its expiry and exact task/session/repository scope.
		return method == http.MethodPost
	case "/api/v1/git/credentials/resolve":
		return method == http.MethodGet || method == http.MethodPost
	case "/api/v1/git/credentials/reissue":
		return method == http.MethodPost
	}
	switch {
	case strings.HasPrefix(path, "/api/v1/automations/webhook/"):
		// X-Webhook-Secret, constant-time compared by the handler.
		return true
	case strings.HasPrefix(path, "/api/v1/office/channels/") && strings.HasSuffix(path, "/inbound"):
		// HMAC-SHA256 / provider token verified by the channel handler.
		return true
	case strings.HasPrefix(path, "/api/v1/github/app/registrations/") &&
		(strings.HasSuffix(path, "/manifest/callback") ||
			strings.HasSuffix(path, "/install/callback") ||
			strings.HasSuffix(path, "/personal/callback")):
		// GitHub redirects through a public hostname that cannot carry the
		// Kandev session cookie. The handlers validate expiring, single-use state.
		return method == http.MethodGet
	case strings.HasPrefix(path, "/api/v1/github/app/registrations/") && strings.HasSuffix(path, "/webhook"):
		// GitHub App webhook delivery; HMAC (X-Hub-Signature-256) verified by
		// the handler, not request identity.
		return true
	case strings.HasPrefix(path, "/api/v1/e2e") || strings.HasPrefix(path, "/api/v1/_test/"):
		// Test-harness routes; only mounted under KANDEV_E2E_MOCK. Never
		// registered on production binaries.
		return true
	}
	return false
}

// isDeferredPath lets requests through for a downstream authenticator or for
// surfaces that cannot be challenged here.
func isDeferredPath(c *gin.Context, path string) bool {
	switch {
	case path == "/ws",
		strings.HasPrefix(path, "/api/v1/plugins/web-apps/runtime/"),
		strings.HasPrefix(path, "/terminal/"),
		strings.HasPrefix(path, "/lsp/"),
		strings.HasPrefix(path, "/vscode/"),
		strings.HasPrefix(path, "/port-proxy/"):
		// WebSocket upgrades and iframe-embedded proxies authenticate inside
		// the gateway handlers (cookie or ?token=PAT) — a JSON 401 mid-upgrade
		// would surface as an opaque connection error.
		return true
	case strings.HasPrefix(path, "/mcp"):
		// External MCP enforces PAT auth in its own group middleware
		// (externalMCPAuthMiddleware) so agent clients get MCP-shaped errors.
		return true
	case isPluginWebhookRelayMethod(c.Request.Method) && isPluginWebhookPath(path):
		// Whether this specific webhook is anonymous-callable depends on the
		// plugin's manifest (webhooks[].public), which only
		// plugins.Controller.webhook can read — it enforces the auth gate
		// itself (webhookCallerAuthorized), mirroring the /mcp precedent above.
		return true
	case strings.HasPrefix(path, "/api/v1/office/") && BearerToken(c.Request) != "":
		// Sandbox office agents call back with an agent JWT (KANDEV_API_KEY);
		// officeagents.AgentAuthMiddleware validates it. Bearer-less office
		// requests do NOT defer — they need a session like any other API call.
		return true
	case !strings.HasPrefix(path, "/api/") && !strings.HasPrefix(path, "/debug/"):
		// SPA shell + static assets (NoRoute handler): must stay reachable so
		// the login page can render. The boot payload carries no data for
		// unauthenticated visitors.
		return true
	}
	return false
}

// isPluginWebhookRelayMethod reports whether method is registered on the plugin
// webhook relay route. Other methods should not bypass the global auth
// challenge just because the path has the relay shape.
func isPluginWebhookRelayMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodPost
}

// isPluginWebhookPath structurally matches /api/plugins/<id>/webhooks/<key>
// with both dynamic segments non-empty — precisely what plugins.RegisterRoutes
// registers for the relay endpoint (POST/GET /:id/webhooks/:key). Splitting
// on "/" (rather than the old strings.Contains(path, "/webhooks/")) avoids
// over-matching unrelated routes that merely contain the substring, such as
// /api/plugins/p1/user-state/task/t1/note/webhooks/k or /api/plugins/webhooks/x.
func isPluginWebhookPath(path string) bool {
	parts := strings.Split(path, "/")
	return len(parts) == 6 &&
		parts[0] == "" && parts[1] == "api" && parts[2] == "plugins" &&
		parts[3] != "" && parts[4] == "webhooks" && parts[5] != ""
}
