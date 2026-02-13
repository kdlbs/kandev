package codex

import (
	"strings"
	"testing"
)

func TestParseCodexStderrError(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		wantNil         bool
		wantHTTPError   string
		wantErrorType   string
		wantMsgContains string
		wantRawJSON     bool
	}{
		{
			name:    "no match - empty string",
			input:   "",
			wantNil: true,
		},
		{
			name:    "no match - regular log line",
			input:   "2026-01-23T22:57:08.953223Z INFO some_module: doing something",
			wantNil: true,
		},
		{
			name:    "no match - error without Some pattern",
			input:   "2026-01-23T22:57:08.953223Z ERROR some_module: error=something went wrong",
			wantNil: true,
		},
		{
			name:            "rate limit error - full format",
			input:           `2026-01-23T22:57:08.953223Z ERROR codex_api::endpoint::responses: error=http 429 Too Many Requests: Some("{\"error\":{\"type\":\"usage_limit_reached\",\"message\":\"The usage limit has been reached\",\"resets_in_seconds\":57600}}")`,
			wantNil:         false,
			wantHTTPError:   "http 429 Too Many Requests",
			wantErrorType:   "usage_limit_reached",
			wantMsgContains: "The usage limit has been reached",
			wantRawJSON:     true,
		},
		{
			name:            "rate limit error - with reset time in hours",
			input:           `error=http 429 Too Many Requests: Some("{\"error\":{\"type\":\"usage_limit_reached\",\"message\":\"Limit reached\",\"resets_in_seconds\":7200}}")`,
			wantNil:         false,
			wantHTTPError:   "http 429 Too Many Requests",
			wantErrorType:   "usage_limit_reached",
			wantMsgContains: "resets in 2 hours",
			wantRawJSON:     true,
		},
		{
			name:            "rate limit error - with reset time in minutes",
			input:           `error=http 429 Too Many Requests: Some("{\"error\":{\"type\":\"usage_limit_reached\",\"message\":\"Limit reached\",\"resets_in_seconds\":300}}")`,
			wantNil:         false,
			wantHTTPError:   "http 429 Too Many Requests",
			wantMsgContains: "resets in 5 minutes",
			wantRawJSON:     true,
		},
		{
			name:            "rate limit error - with reset time in seconds",
			input:           `error=http 429 Too Many Requests: Some("{\"error\":{\"type\":\"usage_limit_reached\",\"message\":\"Limit reached\",\"resets_in_seconds\":45}}")`,
			wantNil:         false,
			wantHTTPError:   "http 429 Too Many Requests",
			wantMsgContains: "resets in 45 seconds",
			wantRawJSON:     true,
		},
		{
			name:            "auth error",
			input:           `error=http 401 Unauthorized: Some("{\"error\":{\"type\":\"invalid_api_key\",\"message\":\"Invalid API key provided\"}}")`,
			wantNil:         false,
			wantHTTPError:   "http 401 Unauthorized",
			wantErrorType:   "invalid_api_key",
			wantMsgContains: "Invalid API key provided",
			wantRawJSON:     true,
		},
		{
			name:            "error with type but no message",
			input:           `error=http 400 Bad Request: Some("{\"error\":{\"type\":\"invalid_request\"}}")`,
			wantNil:         false,
			wantHTTPError:   "http 400 Bad Request",
			wantErrorType:   "invalid_request",
			wantMsgContains: "Error: invalid_request",
			wantRawJSON:     true,
		},
		{
			name:            "flat error structure - message at top level",
			input:           `error=http 500 Internal Server Error: Some("{\"message\":\"Something went wrong\",\"code\":500}")`,
			wantNil:         false,
			wantHTTPError:   "http 500 Internal Server Error",
			wantMsgContains: "Something went wrong",
			wantRawJSON:     true,
		},
		{
			name:            "unknown JSON structure",
			input:           `error=http 502 Bad Gateway: Some("{\"status\":\"error\",\"details\":{\"reason\":\"upstream timeout\"}}")`,
			wantNil:         false,
			wantHTTPError:   "http 502 Bad Gateway",
			wantMsgContains: "http 502 Bad Gateway",
			wantRawJSON:     true,
		},
		{
			name:            "invalid JSON - still returns HTTP error",
			input:           `error=http 500 Internal Server Error: Some("not valid json")`,
			wantNil:         false,
			wantHTTPError:   "http 500 Internal Server Error",
			wantMsgContains: "http 500 Internal Server Error",
			wantRawJSON:     false,
		},
		{
			name:            "empty JSON object",
			input:           `error=http 500 Internal Server Error: Some("{}")`,
			wantNil:         false,
			wantHTTPError:   "http 500 Internal Server Error",
			wantMsgContains: "http 500 Internal Server Error",
			wantRawJSON:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseCodexStderrError(tt.input)

			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
				return
			}

			if result == nil {
				t.Fatal("expected non-nil result, got nil")
			} else {
				if result.HTTPError != tt.wantHTTPError {
					t.Errorf("HTTPError = %q, want %q", result.HTTPError, tt.wantHTTPError)
				}

				if tt.wantErrorType != "" && result.ErrorType != tt.wantErrorType {
					t.Errorf("ErrorType = %q, want %q", result.ErrorType, tt.wantErrorType)
				}

				if !strings.Contains(result.Message, tt.wantMsgContains) {
					t.Errorf("Message = %q, want to contain %q", result.Message, tt.wantMsgContains)
				}
			}

			if tt.wantRawJSON && result.RawJSON == nil {
				t.Error("expected RawJSON to be non-nil")
			}
			if !tt.wantRawJSON && result.RawJSON != nil {
				t.Error("expected RawJSON to be nil")
			}
		})
	}
}

func TestParseCodexStderrLines(t *testing.T) {
	tests := []struct {
		name            string
		lines           []string
		wantNil         bool
		wantMsgContains string
	}{
		{
			name:    "empty lines",
			lines:   []string{},
			wantNil: true,
		},
		{
			name:    "nil lines",
			lines:   nil,
			wantNil: true,
		},
		{
			name: "no error lines",
			lines: []string{
				"2026-01-23T22:57:08Z INFO starting up",
				"2026-01-23T22:57:09Z DEBUG processing request",
			},
			wantNil: true,
		},
		{
			name: "error at end - returns most recent",
			lines: []string{
				"2026-01-23T22:57:08Z INFO starting up",
				`error=http 429 Too Many Requests: Some("{\"error\":{\"message\":\"Rate limited\"}}")`,
			},
			wantNil:         false,
			wantMsgContains: "Rate limited",
		},
		{
			name: "error in middle - returns it",
			lines: []string{
				`error=http 500 Server Error: Some("{\"error\":{\"message\":\"Server error\"}}")`,
				"2026-01-23T22:57:09Z INFO recovered",
			},
			wantNil:         false,
			wantMsgContains: "Server error",
		},
		{
			name: "multiple errors - returns most recent (last)",
			lines: []string{
				`error=http 500 Server Error: Some("{\"error\":{\"message\":\"First error\"}}")`,
				"2026-01-23T22:57:09Z INFO retrying",
				`error=http 429 Rate Limited: Some("{\"error\":{\"message\":\"Second error\"}}")`,
			},
			wantNil:         false,
			wantMsgContains: "Second error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseCodexStderrLines(tt.lines)

			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
				return
			}

			if result == nil {
				t.Fatal("expected non-nil result, got nil")
			} else if !strings.Contains(result.Message, tt.wantMsgContains) {
				t.Errorf("Message = %q, want to contain %q", result.Message, tt.wantMsgContains)
			}
		})
	}
}

func TestParseCodexStderrError_ResetsInSeconds(t *testing.T) {
	// Test that ResetsInSeconds is correctly extracted
	input := `error=http 429 Too Many Requests: Some("{\"error\":{\"type\":\"usage_limit_reached\",\"resets_in_seconds\":3600}}")`

	result := ParseCodexStderrError(input)
	if result == nil {
		t.Fatal("expected non-nil result")
	} else if result.ResetsInSeconds != 3600 {
		t.Errorf("ResetsInSeconds = %d, want 3600", result.ResetsInSeconds)
	}
}
