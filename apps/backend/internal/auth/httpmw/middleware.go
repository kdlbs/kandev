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
	"net"
	"net/http"
	"net/netip"
	"strings"

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
// diverge; an untrusted value is dropped (with a warning) and the resolver
// falls back to the request Host. backendapp passes the list returned by its
// configureTrustedProxies, so the two uses share one configuration.
func StripUntrustedForwardedHost(trusted []string, log *logger.Logger) gin.HandlerFunc {
	matcher := newTrustedProxyMatcher(trusted)
	return func(c *gin.Context) {
		if c.GetHeader("X-Forwarded-Host") == "" {
			return
		}
		peer := remoteAddrHost(c.Request.RemoteAddr)
		if matcher.contains(peer) {
			return
		}
		log.Warn("ignoring X-Forwarded-Host from untrusted peer",
			zap.String("peer", peer),
			zap.String("forwarded_host", c.GetHeader("X-Forwarded-Host")))
		c.Request.Header.Del("X-Forwarded-Host")
	}
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
// bearer. Shared with the WS gateway's upgrade-time check.
func ResolveRequest(c *gin.Context, svc *auth.Service) (authn.Identity, bool) {
	ctx := c.Request.Context()
	if cookie, err := c.Cookie(svc.CookieNameForRequest(c.Request)); err == nil && cookie != "" {
		if identity, ok := svc.ResolveSessionToken(ctx, cookie); ok {
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
func isPublicPath(method, path string) bool {
	switch path {
	case "/health":
		// CLI + desktop readiness probes poll before any session can exist.
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
	case strings.HasPrefix(path, "/api/plugins/") && strings.Contains(path, "/webhooks/"):
		// Relayed to the plugin subprocess, which owns signature validation.
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
