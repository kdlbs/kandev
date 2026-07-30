package usage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// stubKeychain swaps the keychain reader for the duration of one test. It
// honours the empty-service contract of the real reader, so a client
// constructed without a service name still bypasses the keychain entirely.
func stubKeychain(t *testing.T, blob []byte) {
	t.Helper()
	prev := readKeychainCredentials
	readKeychainCredentials = func(service string) []byte {
		if service == "" {
			return nil
		}
		return blob
	}
	t.Cleanup(func() { readKeychainCredentials = prev })
}

func keychainBlob(t *testing.T, accessToken string, expiresAt int64) []byte {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"claudeAiOauth": map[string]any{
			"accessToken":      accessToken,
			"refreshToken":     "keychain-refresh",
			"expiresAt":        expiresAt,
			"subscriptionType": "max",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// The regression this whole file exists for: on macOS the CLI refreshes the
// token in the keychain and never rewrites ~/.claude/.credentials.json, so a
// working install accumulates a file whose accessToken expired days ago and
// whose refreshToken has already been rotated away. Reading the file first
// makes every usage fetch fail with `invalid_request_error` on a machine that
// is otherwise perfectly authenticated.
func TestClaudeFetchUsage_PrefersKeychainOverStaleFile(t *testing.T) {
	staleExpiry := time.Now().Add(-9 * 24 * time.Hour).UnixMilli()
	path := writeClaudeCreds(t, t.TempDir(), staleExpiry, map[string]any{
		"accessToken": "stale-file-token",
	})
	stubKeychain(t, keychainBlob(t, "live-keychain-token", futureExpiryMillis()))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer live-keychain-token" {
			t.Errorf("Authorization = %q, want the keychain token", got)
		}
		_, _ = w.Write([]byte(`{"five_hour": {"utilization": 12.5}}`))
	}))
	defer srv.Close()

	client := NewClaudeUsageClientForProfile(path, ClaudeDefaultKeychainService)
	client.usageURL = srv.URL

	usage, err := client.FetchUsage(context.Background())
	if err != nil {
		t.Fatalf("FetchUsage: %v", err)
	}
	if len(usage.Windows) != 1 || usage.Windows[0].UtilizationPct != 12.5 {
		t.Fatalf("windows = %+v", usage.Windows)
	}
}

// A keychain item that is absent, unreadable, or malformed must not blind the
// panel — the file is still a legitimate source (and the only one off darwin).
func TestClaudeFetchUsage_FallsBackToFileWhenKeychainUnusable(t *testing.T) {
	for _, tc := range []struct {
		name string
		blob []byte
	}{
		{"absent", nil},
		{"malformed", []byte("not json")},
		{"no oauth entry", []byte(`{"somethingElse": {}}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeClaudeCreds(t, t.TempDir(), futureExpiryMillis(), nil)
			stubKeychain(t, tc.blob)

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("Authorization"); got != "Bearer live-token" {
					t.Errorf("Authorization = %q, want the file token", got)
				}
				_, _ = w.Write([]byte(`{"five_hour": {"utilization": 1}}`))
			}))
			defer srv.Close()

			client := NewClaudeUsageClientForProfile(path, ClaudeDefaultKeychainService)
			client.usageURL = srv.URL

			if _, err := client.FetchUsage(context.Background()); err != nil {
				t.Fatalf("FetchUsage: %v", err)
			}
		})
	}
}

// A test pointed at a temp file must never read the developer's real keychain.
func TestNewClaudeUsageClientWithPath_IsFileOnly(t *testing.T) {
	path := writeClaudeCreds(t, t.TempDir(), futureExpiryMillis(), nil)
	stubKeychain(t, keychainBlob(t, "should-not-be-used", futureExpiryMillis()))

	client := NewClaudeUsageClientWithPath(path)
	creds, fromKeychain, err := client.readCredentialsWithSource()
	if err != nil {
		t.Fatalf("readCredentialsWithSource: %v", err)
	}
	if fromKeychain {
		t.Error("WithPath consulted the keychain")
	}
	if creds.ClaudeAiOauth.AccessToken != "live-token" {
		t.Errorf("accessToken = %q", creds.ClaudeAiOauth.AccessToken)
	}
}

// When the credential the CLI owns is itself expired, the error should point at
// the CLI rather than surfacing a bare OAuth failure — and must not rewrite the
// file, which would fabricate a competing source of truth for a token we do not
// own.
func TestClaudeFetchUsage_ExpiredKeychainTokenPointsAtCLI(t *testing.T) {
	path := writeClaudeCreds(t, t.TempDir(), futureExpiryMillis(), nil)
	stubKeychain(t, keychainBlob(t, "expired", time.Now().Add(-time.Hour).UnixMilli()))

	refresh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error"}}`))
	}))
	defer refresh.Close()

	client := NewClaudeUsageClientForProfile(path, ClaudeDefaultKeychainService)
	client.refreshURL = refresh.URL

	_, err := client.FetchUsage(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "claude auth status") {
		t.Errorf("error should name the CLI remedy, got: %v", err)
	}

	// The file must be untouched.
	creds, _, err := NewClaudeUsageClientWithPath(path).readCredentialsWithSource()
	if err != nil {
		t.Fatal(err)
	}
	if creds.ClaudeAiOauth.AccessToken != "live-token" {
		t.Errorf("file was rewritten: accessToken = %q", creds.ClaudeAiOauth.AccessToken)
	}
}
