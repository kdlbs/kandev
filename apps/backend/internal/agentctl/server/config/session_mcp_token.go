package config

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"sync/atomic"
)

// SessionMCPToken holds the per-instance MCP bearer token in memory only. The
// zero value holds no token. The value never appears in JSON, String() or log
// fields.
type SessionMCPToken struct {
	value atomic.Pointer[string]
	// header is the Authorization value minted with the token. It outlives a
	// revoke so the managed-tool policy keeps recognizing the entry it injected.
	header string
}

// NewSessionMCPToken returns a holder for token, or a holder with no token when
// token is empty.
func NewSessionMCPToken(token string) *SessionMCPToken {
	t := &SessionMCPToken{}
	if token != "" {
		t.value.Store(&token)
		t.header = "Bearer " + token
	}
	return t
}

// Value returns the current token, or "" when none is held or it was revoked.
func (t *SessionMCPToken) Value() string {
	if t == nil {
		return ""
	}
	if p := t.value.Load(); p != nil {
		return *p
	}
	return ""
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

// Revoke clears the token so the instance answers MCP routes as unconfigured.
func (t *SessionMCPToken) Revoke() {
	if t != nil {
		t.value.Store(nil)
	}
}

// Fingerprint returns the one-way fingerprint of the held token, or "" when none.
func (t *SessionMCPToken) Fingerprint() string {
	return TokenFingerprint(t.Value())
}

// String and GoString keep the token out of formatted output.
func (t *SessionMCPToken) String() string   { return "[redacted]" }
func (t *SessionMCPToken) GoString() string { return "[redacted]" }

// TokenFingerprint is base64url(SHA-256("kandev-mcp-fp-v1:" + token)), unpadded,
// or "" for an empty token.
func TokenFingerprint(token string) string {
	if token == "" {
		return ""
	}
	return fingerprint("kandev-mcp-fp-v1:" + token)
}

// BrokerUpstreamFingerprint is the same construction over the upstream broker
// URL, "" when none.
func BrokerUpstreamFingerprint(upstream string) string {
	if upstream == "" {
		return ""
	}
	return fingerprint("kandev-broker-fp-v1:" + upstream)
}

func fingerprint(s string) string {
	sum := sha256.Sum256([]byte(s))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
