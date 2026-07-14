package websocket

import (
	"net/http"
	"testing"
)

// The exhaustive origin policy table lives in internal/common/httpmw; this
// covers the upgrade-specific wrapper semantics.
func TestCheckWebSocketOrigin(t *testing.T) {
	tests := []struct {
		name   string
		origin string
		host   string
		want   bool
	}{
		{"no origin allows non-browser clients", "", "example.com", true},
		{"same hostname", "https://example.com", "example.com:8080", true},
		{"loopback cross port", "http://localhost:3000", "127.0.0.1:8080", true},
		{"foreign origin", "https://evil.com", "example.com", false},
		{"malformed origin", "not-a-url", "example.com", false},
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

			if got := checkWebSocketOrigin(r); got != tt.want {
				t.Errorf("checkWebSocketOrigin(origin=%q, host=%q) = %v, want %v",
					tt.origin, tt.host, got, tt.want)
			}
		})
	}
}
