package websocket

import (
	"net/http"
	"testing"
)

func TestCheckWebSocketOrigin(t *testing.T) {
	tests := []struct {
		name   string
		origin string
		host   string
		want   bool
	}{
		// No origin — allow (non-browser client)
		{"no origin", "", "example.com", true},

		// Loopback origin ↔ loopback host (dev servers, desktop shell)
		{"localhost", "http://localhost", "localhost", true},
		{"localhost different ports", "http://localhost:3000", "localhost:8080", true},
		{"https localhost", "https://localhost", "localhost", true},
		{"127.0.0.1", "http://127.0.0.1", "127.0.0.1", true},
		{"127.0.0.1 different ports", "http://127.0.0.1:3000", "127.0.0.1:8080", true},
		{"localhost origin to 127.0.0.1 host", "http://localhost:3000", "127.0.0.1:8080", true},
		{"ipv6 loopback origin to ipv4 loopback host", "http://[::1]:3000", "127.0.0.1:8080", true},
		{"ipv6 loopback host", "http://localhost:3000", "[::1]:8080", true},

		// Same-origin (origin hostname matches request hostname, ports ignored)
		{"same origin", "https://example.com", "example.com", true},
		{"same origin with port", "https://example.com:443", "example.com:8080", true},
		{"same origin case insensitive", "https://Example.COM", "example.com", true},

		// Cross-origin — reject
		{"cross origin", "https://evil.com", "example.com", false},
		{"cross origin similar", "https://notexample.com", "example.com", false},
		{"ipv6 loopback origin to public host", "http://[::1]:3000", "example.com:8080", false},
		{"loopback origin to public host", "http://localhost:3000", "example.com", false},

		// Loopback prefix-match bypass attempts — hostname must match exactly
		{"localhost subdomain bypass", "http://localhost.attacker.tld", "localhost:8080", false},
		{"127.0.0.1 subdomain bypass", "http://127.0.0.1.attacker.tld:80", "127.0.0.1:8080", false},

		// Malformed / non-http origins
		{"malformed origin", "not-a-url", "example.com", false},
		{"null origin", "null", "localhost:8080", false},
		{"file scheme", "file:///etc/passwd", "localhost:8080", false},

		// Empty host — no match possible
		{"empty host rejects", "https://example.com", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{
				Header: http.Header{},
				Host:   tt.host,
			}
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}

			got := checkWebSocketOrigin(r)
			if got != tt.want {
				t.Errorf("checkWebSocketOrigin(origin=%q, host=%q) = %v, want %v",
					tt.origin, tt.host, got, tt.want)
			}
		})
	}
}
