package usage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTeamClaudeFetchUsageReportsUsablePoolCapacity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/teamclaude/status" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"accounts":[
			{"status":"active","quota":{"unified5h":0.80,"unified5hReset":1786644793000,"unified7d":0.70,"unified7dReset":1786944793000,"unified7dSonnet":0.90,"unified7dSonnetReset":1786944793000}},
			{"status":"active","quota":{"unified5h":0.20,"unified5hReset":1786648393000,"unified7d":0.30,"unified7dReset":1786958393000,"unified7dSonnet":0.10,"unified7dSonnetReset":1786958393000}},
			{"status":"throttled","unavailable":"rate-limited","quota":{"unified5h":0.01,"unified7d":0.01}},
			{"disabled":true,"status":"active","quota":{"unified5h":0.02,"unified7d":0.02}}
		]}`))
	}))
	defer srv.Close()

	got, err := NewTeamClaudeUsageClient(srv.URL + "/teamclaude/status").FetchUsage(context.Background())
	if err != nil {
		t.Fatalf("FetchUsage: %v", err)
	}
	if got.Provider != "teamclaude" || got.Plan != "pool (2 available of 4)" {
		t.Fatalf("provider/plan = %q/%q", got.Provider, got.Plan)
	}
	if len(got.Windows) != 3 {
		t.Fatalf("windows = %+v, want 3", got.Windows)
	}
	if got.Windows[0].Label != "5-hour (pool)" || got.Windows[0].UtilizationPct != 20 {
		t.Fatalf("5-hour window = %+v", got.Windows[0])
	}
	if !got.Windows[0].ResetAt.Equal(time.Unix(1786648393, 0).UTC()) {
		t.Fatalf("5-hour reset = %v", got.Windows[0].ResetAt)
	}
	if got.Windows[2].Label != "7-day Sonnet (pool)" || got.Windows[2].UtilizationPct != 10 {
		t.Fatalf("Sonnet window = %+v", got.Windows[2])
	}
}

func TestTeamClaudeFetchUsageRejectsNonSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	if _, err := NewTeamClaudeUsageClient(srv.URL).FetchUsage(context.Background()); err == nil {
		t.Fatal("FetchUsage succeeded for unauthorized status")
	}
}

func TestTeamClaudeFetchUsageRejectsNonStatusPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"not_accounts":[]}`))
	}))
	defer srv.Close()
	if _, err := NewTeamClaudeUsageClient(srv.URL).FetchUsage(context.Background()); err == nil {
		t.Fatal("FetchUsage succeeded for a non-TeamClaude payload")
	}
}

// TestTeamClaudeFetchUsageDoesNotFollowRedirect pins that discovery's
// loopback-only guarantee holds for the response as well as the request: the
// validated URL is loopback, but nothing revalidates a redirect target, so the
// client must treat a 3xx as a failure rather than follow it wherever it
// points.
func TestTeamClaudeFetchUsageDoesNotFollowRedirect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"accounts":[{"status":"active","quota":{"unified5h":0.5}}]}`))
	}))
	defer target.Close()

	redirecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirecting.Close()

	if _, err := NewTeamClaudeUsageClient(redirecting.URL).FetchUsage(context.Background()); err == nil {
		t.Fatal("FetchUsage followed a redirect instead of failing on the 3xx response")
	}
}
