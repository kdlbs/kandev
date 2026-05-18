package updates

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchLatestRelease_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Errorf("missing Accept header, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.4","html_url":"https://github.com/kdlbs/kandev/releases/tag/v1.2.4"}`))
	}))
	defer srv.Close()

	tag, url, err := FetchLatestReleaseFrom(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tag != "v1.2.4" {
		t.Errorf("tag=%q want v1.2.4", tag)
	}
	if url != "https://github.com/kdlbs/kandev/releases/tag/v1.2.4" {
		t.Errorf("url=%q", url)
	}
}

func TestFetchLatestRelease_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	_, _, err := FetchLatestReleaseFrom(context.Background(), srv.Client(), srv.URL)
	if err == nil {
		t.Fatalf("expected error on 404")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected error message to mention 404, got %v", err)
	}
}

func TestFetchLatestRelease_RateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	_, _, err := FetchLatestReleaseFrom(context.Background(), srv.Client(), srv.URL)
	if !errors.Is(err, ErrGitHubRateLimited) {
		t.Fatalf("expected ErrGitHubRateLimited, got %v", err)
	}
}

func TestFetchLatestRelease_MissingTag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"html_url":"x"}`))
	}))
	defer srv.Close()

	_, _, err := FetchLatestReleaseFrom(context.Background(), srv.Client(), srv.URL)
	if err == nil {
		t.Fatalf("expected error for missing tag_name")
	}
}
