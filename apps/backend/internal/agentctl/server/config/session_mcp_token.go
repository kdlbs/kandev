package config

import (
	"crypto/subtle"
	"fmt"
	"sync/atomic"

	"github.com/kandev/kandev/internal/common/sessionmcp"
	"github.com/kandev/kandev/internal/githubauth"
)

// sessionMCPState is the guard mode and expected token, replaced as one value.
type sessionMCPState struct {
	revoked bool
	token   string
}

// SessionMCPToken holds the per-instance MCP bearer token in memory only. An
// instance created with a token is enforcing, one created without is legacy
// (MCP routes unguarded), and a revoke moves either to revoked. The value
// never appears in JSON, String() or log fields.
type SessionMCPToken struct {
	state atomic.Pointer[sessionMCPState]
	// header is the Authorization value minted with the token. It outlives a
	// revoke so the managed-tool policy keeps recognizing the entry it injected.
	header string
}

// NewSessionMCPToken returns an enforcing holder for token, or a legacy holder
// when token is empty.
func NewSessionMCPToken(token string) *SessionMCPToken {
	t := &SessionMCPToken{}
	t.state.Store(&sessionMCPState{token: token})
	if token != "" {
		t.header = "Bearer " + token
	}
	return t
}

// Value returns the current token, or "" when none is held or it was revoked.
func (t *SessionMCPToken) Value() string {
	if t == nil {
		return ""
	}
	if st := t.state.Load(); st != nil {
		return st.token
	}
	return ""
}

// Enforcing reports whether MCP requests must present the held token.
func (t *SessionMCPToken) Enforcing() bool { return t.Value() != "" }

// Revoked reports whether the holder was revoked.
func (t *SessionMCPToken) Revoked() bool {
	if t == nil {
		return false
	}
	st := t.state.Load()
	return st != nil && st.revoked
}

// Headers returns the single Authorization header carrying the token, or nil
// when the instance was created without one.
func (t *SessionMCPToken) Headers() map[string]string {
	if t == nil || t.header == "" {
		return nil
	}
	return map[string]string{"Authorization": t.header}
}

// MatchesHeaders reports whether headers is exactly the one token Authorization
// header (or empty, for an instance without a token).
func (t *SessionMCPToken) MatchesHeaders(headers map[string]string) bool {
	if len(headers) == 0 {
		return true
	}
	return t != nil && t.header != "" && len(headers) == 1 &&
		subtle.ConstantTimeCompare([]byte(headers["Authorization"]), []byte(t.header)) == 1
}

// Revoke clears the token so the instance answers MCP routes as unconfigured,
// whether it was enforcing or legacy.
func (t *SessionMCPToken) Revoke() {
	if t != nil {
		t.state.Store(&sessionMCPState{revoked: true})
	}
}

// Fingerprint returns the one-way fingerprint of the held token, or "" when none.
func (t *SessionMCPToken) Fingerprint() string {
	return TokenFingerprint(t.Value())
}

// String and GoString keep the token out of formatted output.
func (t *SessionMCPToken) String() string   { return "[redacted]" }
func (t *SessionMCPToken) GoString() string { return "[redacted]" }

// TokenFingerprint fingerprints a session token; see sessionmcp.Fingerprint.
func TokenFingerprint(token string) string { return sessionmcp.Fingerprint(token) }

// BrokerUpstreamFingerprint fingerprints the upstream broker URL.
func BrokerUpstreamFingerprint(upstream string) string { return sessionmcp.BrokerFingerprint(upstream) }

// BrokerRelayPath is the instance listener path of the Git credential relay.
const BrokerRelayPath = "/git-credentials/resolve"

// BrokerRelayURL is the broker URL agent processes of an instance that relays
// to an upstream broker are given: its own loopback listener.
func BrokerRelayURL(port int) string {
	return fmt.Sprintf("http://127.0.0.1:%d%s", port, BrokerRelayPath)
}

// EffectiveBrokerURLEnv returns env with the broker URL replaced by the relay
// URL when the instance relays to an upstream broker and env carries a broker
// URL. The input is not modified.
func (c *InstanceConfig) EffectiveBrokerURLEnv(env map[string]string) map[string]string {
	if c == nil || c.BrokerUpstreamURL == "" || c.Port <= 0 || env[githubauth.CredentialBrokerURLEnv] == "" {
		return env
	}
	out := make(map[string]string, len(env))
	for k, v := range env {
		out[k] = v
	}
	out[githubauth.CredentialBrokerURLEnv] = BrokerRelayURL(c.Port)
	return out
}
