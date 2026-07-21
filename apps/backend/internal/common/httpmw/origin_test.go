package httpmw

import "testing"

func TestAllowedOrigin(t *testing.T) {
	tests := []struct {
		name   string
		origin string
		host   string
		want   bool
	}{
		// Loopback origin ↔ loopback host (dev servers, desktop shell)
		{"localhost", "http://localhost", "localhost", true},
		{"localhost different ports", "http://localhost:3000", "localhost:8080", true},
		{"https localhost", "https://localhost", "localhost", true},
		{"127.0.0.1", "http://127.0.0.1", "127.0.0.1", true},
		{"127.0.0.1 different ports", "http://127.0.0.1:3000", "127.0.0.1:8080", true},
		{"localhost origin to 127.0.0.1 host", "http://localhost:3000", "127.0.0.1:8080", true},
		{"ipv6 loopback origin to ipv4 loopback host", "http://[::1]:3000", "127.0.0.1:8080", true},
		{"ipv6 loopback host", "http://localhost:3000", "[::1]:8080", true},

		// Same hostname (ports ignored — see AllowedOrigin doc)
		{"same origin", "https://example.com", "example.com", true},
		{"same origin with port", "https://example.com:443", "example.com:8080", true},
		{"same origin case insensitive", "https://Example.COM", "example.com", true},
		{"uppercase scheme", "HTTPS://example.com", "example.com", true},

		// Cross-origin — reject
		{"cross origin", "https://evil.com", "example.com", false},
		{"cross origin similar", "https://notexample.com", "example.com", false},
		{"ipv6 loopback origin to public host", "http://[::1]:3000", "example.com:8080", false},
		{"loopback origin to public host", "http://localhost:3000", "example.com", false},

		// Loopback prefix-match bypass attempts — hostname must match exactly
		{"localhost subdomain bypass", "http://localhost.attacker.tld", "localhost:8080", false},
		{"127.0.0.1 subdomain bypass", "http://127.0.0.1.attacker.tld:80", "127.0.0.1:8080", false},

		// Malformed / non-http origins
		{"empty origin", "", "example.com", false},
		{"malformed origin", "not-a-url", "example.com", false},
		{"null origin", "null", "localhost:8080", false},
		{"file scheme", "file:///etc/passwd", "localhost:8080", false},
		{"ws scheme", "ws://example.com", "example.com", false},

		// Components that never appear in a browser Origin header — reject
		{"userinfo", "https://user@example.com", "example.com", false},
		{"userinfo with password", "https://user:pass@example.com", "example.com", false},
		{"path", "https://example.com/path", "example.com", false},
		{"trailing slash", "https://example.com/", "example.com", false},
		{"query", "https://example.com?x=1", "example.com", false},
		{"empty query", "https://example.com?", "example.com", false},
		{"fragment", "https://example.com#frag", "example.com", false},
		{"opaque", "https:example.com", "example.com", false},

		// Empty host — no match possible
		{"empty request host", "https://example.com", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AllowedOrigin(tt.origin, tt.host); got != tt.want {
				t.Errorf("AllowedOrigin(%q, %q) = %v, want %v", tt.origin, tt.host, got, tt.want)
			}
		})
	}
}
