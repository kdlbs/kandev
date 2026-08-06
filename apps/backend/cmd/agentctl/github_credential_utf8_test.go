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
// iteration, so a cut mid-rune must not panic or produce invalid UTF-8 in the
// surfaced error message.
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
	t.Logf("error = %q (len=%d)", err.Error(), len(err.Error()))
}
