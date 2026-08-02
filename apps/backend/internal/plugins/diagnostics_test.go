package plugins

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestNormalizePluginErrorCollapsesWhitespace(t *testing.T) {
	got := normalizePluginError(errors.New("  handshake\nfailed\twhile starting  "))
	if got != "handshake failed while starting" {
		t.Fatalf("normalizePluginError() = %q, want %q", got, "handshake failed while starting")
	}
}

func TestNormalizePluginErrorBoundsUTF8Message(t *testing.T) {
	got := normalizePluginError(errors.New(strings.Repeat("é", maxPluginErrorBytes)))
	if len([]byte(got)) > maxPluginErrorBytes {
		t.Fatalf("normalizePluginError() returned %d bytes, want <= %d", len([]byte(got)), maxPluginErrorBytes)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("normalizePluginError() = %q, want a truncation marker", got)
	}
}

func TestNormalizePluginErrorNilIsEmpty(t *testing.T) {
	if got := normalizePluginError(nil); got != "" {
		t.Fatalf("normalizePluginError(nil) = %q, want empty", got)
	}
}

func TestNormalizePluginErrorRedactsCredentialsAndHomePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() error: %v", err)
	}

	const (
		classicPAT   = "ghp_abcdefghijklmnopqrstuvwxyz1234567890AB"
		bearer       = "eyJhbGciOiJIUzI1NiJ9.super-secret-signature"
		password     = "hunter2-rocks"
		token        = "plugin-token-value"
		secret       = "s3cret-value"
		apiKey       = "key-value-123"
		apiToken     = "api-token-value"
		clientSecret = "client-secret-value"
		urlPassword  = "url-password-value"
	)
	cases := []struct {
		name   string
		input  string
		secret []string
		want   string
	}{
		{
			name:   "personal access token",
			input:  "plugin stdout included PAT " + classicPAT,
			secret: []string{classicPAT},
			want:   "[REDACTED]",
		},
		{
			name:   "bearer authorization",
			input:  "connect failed: Authorization: Bearer " + bearer,
			secret: []string{bearer},
			want:   "Bearer [REDACTED]",
		},
		{
			name:   "labeled secrets",
			input:  "password=" + password + " token: " + token + " secret='" + secret + "' api_key=\"" + apiKey + "\" api_token=" + apiToken,
			secret: []string{password, token, secret, apiKey, apiToken},
			want:   "[REDACTED]",
		},
		{
			name:   "compound labeled secret",
			input:  "plugin failed with client_secret=" + clientSecret,
			secret: []string{clientSecret},
			want:   "client_secret=[REDACTED]",
		},
		{
			name:   "URL credentials",
			input:  "dial failed: https://admin:" + urlPassword + "@internal.example/api",
			secret: []string{urlPassword},
			want:   "https://[REDACTED]@internal.example/api",
		},
		{
			name:   "home path",
			input:  home + "/.kandev/plugins/acme/server: handshake failed",
			secret: []string{home + "/"},
			want:   "~/.kandev/plugins/acme/server",
		},
		{
			name:   "windows home path",
			input:  `plugin failed at C:\Users\alice\.kandev\plugins\acme\server: handshake failed`,
			secret: []string{`C:\Users\alice\`},
			want:   `~\.kandev\plugins\acme\server`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizePluginError(errors.New(tc.input))
			for _, value := range tc.secret {
				if strings.Contains(got, value) {
					t.Fatalf("normalizePluginError() leaked %q in %q", value, got)
				}
			}
			if !strings.Contains(got, tc.want) {
				t.Fatalf("normalizePluginError() = %q, want it to contain %q", got, tc.want)
			}
		})
	}
}
