package websocket

import (
	"bytes"
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

func TestStripTerminalResponses(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  []byte
	}{
		{
			name:  "no sequences",
			input: []byte("hello world\r\n$ "),
			want:  []byte("hello world\r\n$ "),
		},
		{
			name:  "empty input",
			input: []byte{},
			want:  []byte{},
		},
		{
			name:  "OSC 11 response with ESC backslash",
			input: []byte("\x1b]11;rgb:1f1f/1f1f/1f1f\x1b\\"),
			want:  []byte{},
		},
		{
			name:  "OSC 11 response with BEL",
			input: []byte("\x1b]11;rgb:1f1f/1f1f/1f1f\x07"),
			want:  []byte{},
		},
		{
			name:  "DA1 response",
			input: []byte("\x1b[?1;2c"),
			want:  []byte{},
		},
		{
			name:  "DA1 response with multiple params",
			input: []byte("\x1b[?64;1;2;6;22c"),
			want:  []byte{},
		},
		{
			name:  "CPR response row;col",
			input: []byte("\x1b[5;1R"),
			want:  []byte{},
		},
		{
			name:  "CPR response row only",
			input: []byte("\x1b[1R"),
			want:  []byte{},
		},
		{
			name:  "only responses produces empty",
			input: []byte("\x1b]11;rgb:1f1f/1f1f/1f1f\x1b\\\x1b[?1;2c\x1b[5;1R"),
			want:  []byte{},
		},
		{
			name:  "mixed content preserves normal output",
			input: []byte("$ ls\r\nfile.txt\r\n\x1b]11;rgb:0000/0000/0000\x1b\\\x1b[?1;2c$ "),
			want:  []byte("$ ls\r\nfile.txt\r\n$ "),
		},
		{
			name:  "sequences between normal text",
			input: []byte("before\x1b[24;80Rafter"),
			want:  []byte("beforeafter"),
		},
		{
			name:  "multiple OSC 11 responses",
			input: []byte("\x1b]11;rgb:1f1f/1f1f/1f1f\x1b\\\x1b]11;rgb:ffff/ffff/ffff\x07"),
			want:  []byte{},
		},
		{
			name:  "preserves other escape sequences",
			input: []byte("\x1b[32mgreen\x1b[0m \x1b[?1;2c normal"),
			want:  []byte("\x1b[32mgreen\x1b[0m  normal"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripTerminalResponses(tt.input)
			if !bytes.Equal(got, tt.want) {
				t.Errorf("stripTerminalResponses(%q)\n got: %q\nwant: %q", tt.input, got, tt.want)
			}
		})
	}
}
