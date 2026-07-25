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

func writeClaudeCreds(t *testing.T, dir string, expiresAt int64, extra map[string]any) string {
	t.Helper()
	oauth := map[string]any{
		"accessToken":      "live-token",
		"refreshToken":     "refresh-token",
		"expiresAt":        expiresAt,
		"subscriptionType": "max",
		"scopes":           []string{"user:inference", "user:profile"},
		"rateLimitTier":    "default_claude_max_20x",
	}
	for k, v := range extra {
		oauth[k] = v
	}
	path := filepath.Join(dir, ".credentials.json")
	data, err := json.Marshal(map[string]any{"claudeAiOauth": oauth})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func futureExpiryMillis() int64 {
	return time.Now().Add(time.Hour).UnixMilli()
}

// The live API returns float utilization values (e.g. 62.0) — the client must
// not parse them as int.
func TestClaudeFetchUsage_ParsesFloatsAndLimits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer live-token" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("anthropic-beta"); got != claudeBetaHeader {
			t.Errorf("anthropic-beta = %q", got)
		}
		_, _ = w.Write([]byte(`{
			"five_hour": {"utilization": 62.0, "resets_at": "2026-07-14T22:09:59.781046+00:00"},
			"seven_day": {"utilization": 15.0, "resets_at": "2026-07-15T00:59:59.781070+00:00"},
			"limits": [
				{"kind": "session", "group": "session", "percent": 62, "resets_at": "2026-07-14T22:09:59.781046+00:00", "scope": null},
				{"kind": "weekly_all", "group": "weekly", "percent": 15, "resets_at": "2026-07-15T00:59:59+00:00", "scope": null},
				{"kind": "weekly_scoped", "group": "weekly", "percent": 20.5, "resets_at": "2026-07-15T00:59:59+00:00",
					"scope": {"model": {"id": null, "display_name": "Opus"}}},
				{"kind": "mystery_future_kind", "percent": 99, "resets_at": "2026-07-15T00:59:59+00:00"}
			]
		}`))
	}))
	defer srv.Close()

	client := NewClaudeUsageClientWithPath(writeClaudeCreds(t, t.TempDir(), futureExpiryMillis(), nil))
	client.usageURL = srv.URL

	got, err := client.FetchUsage(context.Background())
	if err != nil {
		t.Fatalf("FetchUsage: %v", err)
	}
	if got.Provider != "anthropic" {
		t.Errorf("Provider = %q", got.Provider)
	}
	if got.Plan != "max" {
		t.Errorf("Plan = %q, want max", got.Plan)
	}
	wantLabels := []string{"5-hour", "7-day", "7-day (Opus)"}
	if len(got.Windows) != len(wantLabels) {
		t.Fatalf("Windows = %+v, want %d entries", got.Windows, len(wantLabels))
	}
	for i, label := range wantLabels {
		if got.Windows[i].Label != label {
			t.Errorf("Windows[%d].Label = %q, want %q", i, got.Windows[i].Label, label)
		}
	}
	if got.Windows[0].UtilizationPct != 62 {
		t.Errorf("session pct = %v", got.Windows[0].UtilizationPct)
	}
	if got.Windows[2].UtilizationPct != 20.5 {
		t.Errorf("scoped pct = %v", got.Windows[2].UtilizationPct)
	}
	// The live API emits sub-second fractional timestamps; time.Parse accepts
	// them with the RFC3339 layout even though the layout omits fractions.
	wantReset := time.Date(2026, 7, 14, 22, 9, 59, 781046000, time.UTC)
	if !got.Windows[0].ResetAt.Equal(wantReset) {
		t.Errorf("session ResetAt = %v, want %v", got.Windows[0].ResetAt, wantReset)
	}
}

func TestClaudeFetchUsage_FallbackWithoutLimits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"five_hour": {"utilization": 33.3, "resets_at": "2026-07-14T22:09:59+00:00"},
			"seven_day": {"utilization": 7.0, "resets_at": "2026-07-15T00:59:59+00:00"}
		}`))
	}))
	defer srv.Close()

	client := NewClaudeUsageClientWithPath(writeClaudeCreds(t, t.TempDir(), futureExpiryMillis(), nil))
	client.usageURL = srv.URL

	got, err := client.FetchUsage(context.Background())
	if err != nil {
		t.Fatalf("FetchUsage: %v", err)
	}
	if len(got.Windows) != 2 {
		t.Fatalf("Windows = %+v, want 2", got.Windows)
	}
	if got.Windows[0].Label != "5-hour" || got.Windows[0].UtilizationPct != 33.3 {
		t.Errorf("window[0] = %+v", got.Windows[0])
	}
	if got.Windows[1].Label != "7-day" || got.Windows[1].UtilizationPct != 7 {
		t.Errorf("window[1] = %+v", got.Windows[1])
	}
}

// Refreshing an expired token must not drop unrelated fields from the
// credentials file (scopes, subscriptionType, rateLimitTier, ...).
func TestClaudeFetchUsage_RefreshPreservesUnknownFields(t *testing.T) {
	usageSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer new-access" {
			t.Errorf("Authorization after refresh = %q", got)
		}
		_, _ = w.Write([]byte(`{"five_hour": {"utilization": 1.0}, "seven_day": {"utilization": 2.0}}`))
	}))
	defer usageSrv.Close()
	refreshSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token": "new-access", "refresh_token": "new-refresh", "expires_in": 3600}`))
	}))
	defer refreshSrv.Close()

	expired := time.Now().Add(-time.Hour).UnixMilli()
	path := writeClaudeCreds(t, t.TempDir(), expired, nil)
	client := NewClaudeUsageClientWithPath(path)
	client.usageURL = usageSrv.URL
	client.refreshURL = refreshSrv.URL

	if _, err := client.FetchUsage(context.Background()); err != nil {
		t.Fatalf("FetchUsage: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	oauth := root["claudeAiOauth"]
	if oauth["accessToken"] != "new-access" || oauth["refreshToken"] != "new-refresh" {
		t.Errorf("tokens not persisted: %+v", oauth)
	}
	if oauth["subscriptionType"] != "max" {
		t.Errorf("subscriptionType dropped on refresh: %+v", oauth)
	}
	if oauth["rateLimitTier"] != "default_claude_max_20x" {
		t.Errorf("rateLimitTier dropped on refresh: %+v", oauth)
	}
	if _, ok := oauth["scopes"]; !ok {
		t.Errorf("scopes dropped on refresh: %+v", oauth)
	}
}

func TestClaudeHasSubscriptionCredentials(t *testing.T) {
	missing := NewClaudeUsageClientWithPath(filepath.Join(t.TempDir(), "nope.json"))
	if missing.HasSubscriptionCredentials() {
		t.Error("expected false for missing file")
	}

	apiKeyOnly := filepath.Join(t.TempDir(), ".credentials.json")
	if err := os.WriteFile(apiKeyOnly, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if NewClaudeUsageClientWithPath(apiKeyOnly).HasSubscriptionCredentials() {
		t.Error("expected false without claudeAiOauth")
	}

	withOAuth := NewClaudeUsageClientWithPath(writeClaudeCreds(t, t.TempDir(), futureExpiryMillis(), nil))
	if !withOAuth.HasSubscriptionCredentials() {
		t.Error("expected true with OAuth credentials")
	}
}
