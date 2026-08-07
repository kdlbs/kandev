package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestGitHubCredentialHelperHandlesMultibyteTruncationBoundary guards against
// a byte-boundary truncation splitting a multi-byte UTF-8 rune in the broker
// error body. formatBrokerErrorBody truncates by raw bytes before rune
// iteration, so a cut mid-rune must not expand the bounded body or produce
// replacement runes in the surfaced error message.
func TestGitHubCredentialHelperHandlesMultibyteTruncationBoundary(t *testing.T) {
	// Build a body whose byte 512 boundary lands mid-rune: pad with ASCII to
	// 510 bytes, then place a 3-byte rune (e.g. '中') straddling the cut.
	prefix := strings.Repeat("a", 510)
	rawBody := prefix + "中文emoji😀tail"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, rawBody)
	}))
	t.Cleanup(server.Close)
	env := githubCredentialTestEnv(server.URL)

	err := runGitHubCredentialHelper(
		context.Background(), []string{"get"},
		strings.NewReader("protocol=https\nhost=github.com\npath=acme/widgets.git\n\n"),
		io.Discard, lookupEnv(env), server.Client(),
	)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !utf8.ValidString(err.Error()) {
		t.Fatalf("error message is not valid UTF-8: %q", err.Error())
	}
	const errorPrefix = "resolve GitHub credential: broker returned HTTP 401"
	appended := strings.TrimPrefix(err.Error(), errorPrefix)
	if !strings.HasPrefix(appended, ": ") {
		t.Fatalf("error = %q, want appended broker body", err.Error())
	}
	appended = strings.TrimPrefix(appended, ": ")
	if len(appended) > maxBrokerErrorBodyBytes {
		t.Fatalf("appended body length = %d, want <= %d", len(appended), maxBrokerErrorBodyBytes)
	}
	if strings.ContainsRune(appended, utf8.RuneError) {
		t.Fatalf("appended body contains replacement rune: %q", appended)
	}
	if strings.Contains(appended, "中") {
		t.Fatalf("appended body contains the split rune: %q", appended)
	}
}
