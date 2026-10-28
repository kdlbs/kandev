package config

import "testing"

// An empty token in the create request yields a holder that carries nothing:
// the instance stays legacy (unguarded) rather than enforcing an empty bearer.
func TestNewSessionMCPTokenEmptyIsLegacy(t *testing.T) {
	tok := NewSessionMCPToken("")
	if tok.Value() != "" || tok.Headers() != nil || tok.Fingerprint() != "" {
		t.Fatalf("empty token must hold nothing: value=%q headers=%v fingerprint=%q", tok.Value(), tok.Headers(), tok.Fingerprint())
	}
	if tok.MatchesHeaders(map[string]string{"Authorization": "Bearer "}) {
		t.Fatal("an empty holder must not match an empty bearer header")
	}
}

func TestNewSessionMCPTokenCarriesBearerHeader(t *testing.T) {
	tok := NewSessionMCPToken("abc")
	if got := tok.Headers()["Authorization"]; got != "Bearer abc" {
		t.Fatalf("Authorization = %q", got)
	}
	tok.Revoke()
	if tok.Value() != "" {
		t.Fatal("a revoked token must not report a value")
	}
}

func TestSessionMCPTokenModes(t *testing.T) {
	legacy := NewSessionMCPToken("")
	if legacy.Enforcing() || legacy.Revoked() {
		t.Fatal("empty token must be legacy")
	}
	legacy.Revoke()
	if !legacy.Revoked() || legacy.Enforcing() {
		t.Fatal("revoking a legacy holder must mark it revoked")
	}
	enforcing := NewSessionMCPToken("abc")
	if !enforcing.Enforcing() || enforcing.Revoked() {
		t.Fatal("a held token must be enforcing")
	}
	enforcing.Revoke()
	if !enforcing.Revoked() || enforcing.Enforcing() {
		t.Fatal("revoking an enforcing holder must mark it revoked and stop enforcing")
	}
}
