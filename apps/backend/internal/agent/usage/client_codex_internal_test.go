package usage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeCodexAuth(t *testing.T, dir string, withTokens bool) string {
	t.Helper()
	root := map[string]any{"OPENAI_API_KEY": nil, "last_refresh": "2026-07-01T00:00:00Z"}
	if withTokens {
		root["tokens"] = map[string]any{
			"id_token":      "id-token",
			"access_token":  "access-token",
			"refresh_token": "refresh-token",
			"account_id":    "acct-123",
		}
	}
	path := filepath.Join(dir, "auth.json")
	data, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const codexUsageBody = `{
	"plan_type": "plus",
	"rate_limit": {
		"allowed": true,
		"limit_reached": false,
		"primary_window": {"used_percent": 42.5, "limit_window_seconds": 18000, "reset_after_seconds": 9000, "reset_at": 1786644793},
		"secondary_window": {"used_percent": 7, "limit_window_seconds": 604800, "reset_after_seconds": 302400, "reset_at": 1786944793}
	},
	"credits": {"has_credits": false}
}`

func TestCodexFetchUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("chatgpt-account-id"); got != "acct-123" {
			t.Errorf("chatgpt-account-id = %q", got)
		}
		_, _ = w.Write([]byte(codexUsageBody))
	}))
	defer srv.Close()

	client := NewCodexUsageClientWithPath(writeCodexAuth(t, t.TempDir(), true))
	client.usageURL = srv.URL

	got, err := client.FetchUsage(context.Background())
	if err != nil {
		t.Fatalf("FetchUsage: %v", err)
	}
	if got.Provider != "openai" {
		t.Errorf("Provider = %q", got.Provider)
	}
	if got.Plan != "plus" {
		t.Errorf("Plan = %q, want plus", got.Plan)
	}
	if len(got.Windows) != 2 {
		t.Fatalf("Windows = %+v, want 2", got.Windows)
	}
	if got.Windows[0].Label != "5-hour" || got.Windows[0].UtilizationPct != 42.5 {
		t.Errorf("window[0] = %+v", got.Windows[0])
	}
	if !got.Windows[0].ResetAt.Equal(time.Unix(1786644793, 0)) {
		t.Errorf("window[0].ResetAt = %v", got.Windows[0].ResetAt)
	}
	if got.Windows[1].Label != "7-day" || got.Windows[1].UtilizationPct != 7 {
		t.Errorf("window[1] = %+v", got.Windows[1])
	}
}

func TestCodexFetchUsage_NullSecondaryWindow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"plan_type": "free",
			"rate_limit": {
				"primary_window": {"used_percent": 5, "limit_window_seconds": 2592000, "reset_after_seconds": 2592000, "reset_at": 1786644793},
				"secondary_window": null
			}
		}`))
	}))
	defer srv.Close()

	client := NewCodexUsageClientWithPath(writeCodexAuth(t, t.TempDir(), true))
	client.usageURL = srv.URL

	got, err := client.FetchUsage(context.Background())
	if err != nil {
		t.Fatalf("FetchUsage: %v", err)
	}
	if len(got.Windows) != 1 {
		t.Fatalf("Windows = %+v, want 1", got.Windows)
	}
	if got.Windows[0].Label != "30-day" {
		t.Errorf("label = %q, want 30-day", got.Windows[0].Label)
	}
}

func TestCodexFetchUsage_RefreshOn401PreservesUnknownFields(t *testing.T) {
	var usageCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		usageCalls++
		if usageCalls == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer new-access" {
			t.Errorf("Authorization after refresh = %q", got)
		}
		_, _ = w.Write([]byte(codexUsageBody))
	}))
	defer srv.Close()
	refreshSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req codexRefreshRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode refresh request: %v", err)
		}
		if req.ClientID != codexOAuthClientID || req.GrantType != "refresh_token" || req.RefreshToken != "refresh-token" {
			t.Errorf("refresh request = %+v", req)
		}
		_, _ = w.Write([]byte(`{"id_token": "new-id", "access_token": "new-access", "refresh_token": "new-refresh"}`))
	}))
	defer refreshSrv.Close()

	path := writeCodexAuth(t, t.TempDir(), true)
	client := NewCodexUsageClientWithPath(path)
	client.usageURL = srv.URL
	client.refreshURL = refreshSrv.URL

	got, err := client.FetchUsage(context.Background())
	if err != nil {
		t.Fatalf("FetchUsage: %v", err)
	}
	if got.Plan != "plus" {
		t.Errorf("Plan = %q", got.Plan)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	if _, ok := root["OPENAI_API_KEY"]; !ok {
		t.Errorf("OPENAI_API_KEY dropped on refresh: %+v", root)
	}
	toks, _ := root["tokens"].(map[string]any)
	if toks["access_token"] != "new-access" || toks["refresh_token"] != "new-refresh" {
		t.Errorf("tokens not persisted: %+v", toks)
	}
	if toks["account_id"] != "acct-123" {
		t.Errorf("account_id dropped on refresh: %+v", toks)
	}
}

func TestCodexRefreshRejectsEmptyAccessToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	refreshSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id_token": "new-id", "access_token": "", "refresh_token": "new-refresh"}`))
	}))
	defer refreshSrv.Close()

	path := writeCodexAuth(t, t.TempDir(), true)
	client := NewCodexUsageClientWithPath(path)
	client.usageURL = srv.URL
	client.refreshURL = refreshSrv.URL

	if _, err := client.FetchUsage(context.Background()); err == nil {
		t.Fatal("expected error for empty access_token in refresh response")
	}

	// The stored credentials must be untouched.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	toks, _ := root["tokens"].(map[string]any)
	if toks["access_token"] != "access-token" || toks["refresh_token"] != "refresh-token" {
		t.Errorf("credentials were modified: %+v", toks)
	}
}

func TestCodexHasSubscriptionCredentials(t *testing.T) {
	missing := NewCodexUsageClientWithPath(filepath.Join(t.TempDir(), "nope.json"))
	if missing.HasSubscriptionCredentials() {
		t.Error("expected false for missing file")
	}

	apiKeyOnly := writeCodexAuth(t, t.TempDir(), false)
	if NewCodexUsageClientWithPath(apiKeyOnly).HasSubscriptionCredentials() {
		t.Error("expected false for API-key-only auth.json")
	}

	withTokens := writeCodexAuth(t, t.TempDir(), true)
	if !NewCodexUsageClientWithPath(withTokens).HasSubscriptionCredentials() {
		t.Error("expected true with OAuth tokens")
	}
}

func TestCodexWindowLabel(t *testing.T) {
	cases := map[int64]string{
		0:       "current",
		18000:   "5-hour",
		86400:   "1-day",
		604800:  "7-day",
		2592000: "30-day",
	}
	for seconds, want := range cases {
		if got := codexWindowLabel(seconds); got != want {
			t.Errorf("codexWindowLabel(%d) = %q, want %q", seconds, got, want)
		}
	}
}
