package orchestrator

import "testing"

func TestIsAuthError(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		want bool
	}{
		{"empty", "", false},
		{"generic error", "connection timeout", false},
		{"authentication_error type", `{"type":"authentication_error","message":"bad token"}`, true},
		{"expired token", `OAuth token has expired. Please obtain a new token.`, true},
		{"failed to authenticate", `Internal error: Failed to authenticate. API Error: 401`, true},
		{"rate limit", `rate_limit_error: too many requests`, false},
		{"mixed case", `AUTHENTICATION_ERROR occurred`, true},
		{"authentication required JSON-RPC", `{"code":-32000,"message":"Authentication required"}`, true},
		{"authentication required plain", `Authentication required`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAuthError(tt.msg); got != tt.want {
				t.Errorf("isAuthError(%q) = %v, want %v", tt.msg, got, tt.want)
			}
		})
	}
}

func TestExtractReadableAuthError(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "nested JSON-RPC with API error",
			raw:  `{"code":-32603,"message":"Internal error: Failed to authenticate. API Error: 401 {\"type\":\"error\",\"error\":{\"type\":\"authentication_error\",\"message\":\"OAuth token has expired. Please obtain a new token or refresh your existing token.\"},\"request_id\":\"req_abc\"}"}`,
			want: "OAuth token has expired. Please obtain a new token or refresh your existing token.",
		},
		{
			name: "plain message",
			raw:  "token expired",
			want: "token expired",
		},
		{
			name: "JSON-RPC without inner JSON",
			raw:  `{"code":-32603,"message":"Failed to authenticate"}`,
			want: "Failed to authenticate",
		},
		{
			name: "JSON-RPC -32000 authentication required",
			raw:  `{"code":-32000,"message":"Authentication required"}`,
			want: "Authentication required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractReadableAuthError(tt.raw); got != tt.want {
				t.Errorf("extractReadableAuthError() = %q, want %q", got, tt.want)
			}
		})
	}
}
