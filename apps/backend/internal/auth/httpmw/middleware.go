// Package httpmw is the global HTTP enforcement middleware for opt-in
// authentication. It runs after CORS on every request (see
// backendapp.buildHTTPServer) and implements the allowlist policy from
// docs/specs/auth: identity injection in disabled mode, credential resolution
// (session cookie, then PAT bearer), self-authenticating webhook passthrough,
// the office agent-JWT deferral, and SPA-shell availability for the login page.
//
// CSRF note: cross-origin browser requests are already rejected by
// backendapp.corsMiddleware (httpmw.AllowedOrigin) before this middleware
// runs, and the session cookie is SameSite=Lax — no separate origin check is
// required here.
package httpmw

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/auth"
	"github.com/kandev/kandev/internal/auth/authn"
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

// SyntheticIdentity is the implicit identity used while auth is disabled.
func SyntheticIdentity() authn.Identity {
	return authn.Identity{UserID: userstore.DefaultUserID, Role: authn.RoleAdmin, Synthetic: true}
}

// ResolveRequest authenticates a request from its session cookie or PAT
// bearer. Shared with the WS gateway's upgrade-time check.
func ResolveRequest(c *gin.Context, svc *auth.Service) (authn.Identity, bool) {
	ctx := c.Request.Context()
	if cookie, err := c.Cookie(svc.CookieName()); err == nil && cookie != "" {
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
	}
	switch {
	case strings.HasPrefix(path, "/api/v1/automations/webhook/"):
		// X-Webhook-Secret, constant-time compared by the handler.
		return true
	case strings.HasPrefix(path, "/api/v1/office/channels/") && strings.HasSuffix(path, "/inbound"):
		// HMAC-SHA256 / provider token verified by the channel handler.
		return true
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
