package plugins

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// authLoginHeader is the reserved header an auth-capable plugin sets on its
// webhook response to assert a validated external identity (OIDC/SAML SSO).
// Its value is a JSON externalLoginAssertion. The host always consumes and
// removes it before relaying the response, so the plugin-controlled value can
// never reach the browser, and only interprets it when the plugin declares the
// `auth` capability.
const authLoginHeader = "X-Kandev-Auth-Login"

// externalLoginAssertion is the JSON payload carried by authLoginHeader: the
// identity a plugin validated against its IdP. Provider is a non-"local"
// provider key (e.g. an OIDC issuer or SAML entity ID); Subject is the stable
// IdP subject claim; Email/DisplayName come from the IdP for account linking
// and just-in-time provisioning.
type externalLoginAssertion struct {
	Provider    string `json:"provider"`
	Subject     string `json:"subject"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

// AuthLoginBridge establishes an authenticated kandev browser session for an
// external identity that an auth-capable plugin has validated against an
// external IdP (OIDC/SAML SSO). The host mints and sets the session cookie
// (name derived from the request host) on the response; the plugin never
// receives the raw session token — its only channel is the reserved login
// directive header on its webhook response (see handlers.go). Implemented in
// internal/backendapp over internal/auth so this package need not depend on
// the auth service.
type AuthLoginBridge interface {
	// LoginExternal maps the asserted external identity to a kandev user
	// (linking or just-in-time provisioning as needed) and sets the session
	// cookie on c. It returns an error when authentication is not enabled or
	// the identity cannot be established.
	LoginExternal(c *gin.Context, provider, subject, email, displayName string) error

	// SessionCookieName returns the name of Kandev's own session cookie, so
	// the webhook relay (flattenHeaders) can strip it from the headers
	// forwarded to a plugin subprocess.
	SessionCookieName() string
}

// takeAuthLoginDirective removes the reserved SSO login directive header from a
// plugin webhook response's headers and returns its value. It strips *every*
// case variant (e.g. both `X-Kandev-Auth-Login` and `x-kandev-auth-login`,
// which are distinct Go map keys) so no variant can be relayed to the browser,
// regardless of whether the plugin holds the `auth` capability. If a plugin
// sends more than one variant, ambiguous is true and the caller must reject:
// map iteration order is nondeterministic, so we must never pick one arbitrarily
// and authenticate a possibly-different asserted identity.
func takeAuthLoginDirective(headers map[string]string) (raw string, present, ambiguous bool) {
	var matched []string
	for k := range headers {
		if strings.EqualFold(k, authLoginHeader) {
			matched = append(matched, k)
		}
	}
	for _, k := range matched {
		raw = headers[k]
		delete(headers, k)
	}
	switch len(matched) {
	case 0:
		return "", false, false
	case 1:
		return raw, true, false
	default:
		return "", true, true
	}
}
