package websocket

import (
	"net/http"
	"net/url"
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

		// Localhost variants
		{"http://localhost", "http://localhost", "localhost", true},
		{"http://localhost:3000", "http://localhost:3000", "localhost:8080", true},
		{"https://localhost", "https://localhost", "localhost", true},
		{"http://127.0.0.1", "http://127.0.0.1", "127.0.0.1", true},
		{"http://127.0.0.1:3000", "http://127.0.0.1:3000", "127.0.0.1:8080", true},
		{"https://127.0.0.1", "https://127.0.0.1", "127.0.0.1", true},

		// Same-origin (origin host matches request host)
		{"same origin", "https://example.com", "example.com", true},
		{"same origin with port", "https://example.com:443", "example.com:8080", true},

		// Cross-origin — reject
		{"cross origin", "https://evil.com", "example.com", false},
		{"cross origin similar", "https://notexample.com", "example.com", false},

		// Malformed origin
		{"malformed origin", "not-a-url", "example.com", false},

		// IPv6 host
		{"ipv6 cross origin", "http://[::1]:3000", "example.com:8080", false},

		// Empty host — no match possible
		{"empty host rejects", "https://example.com", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{
				Header: http.Header{},
				Host:   tt.host,
				URL:    &url.URL{Host: tt.host},
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
