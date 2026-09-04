package routingerr

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestSanitize_RedactionsGolden(t *testing.T) {
	cases := []struct {
		name        string
		in          string
		mustNotHave []string
		mustHave    []string
	}{
		{
			name:        "anthropic-style key",
			in:          "use sk-abcdef1234567890QQQQ to call",
			mustNotHave: []string{"sk-abcdef1234567890"},
			mustHave:    []string{"sk-***"},
		},
		{
			name:        "github classic pat",
			in:          "token ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAA leaks",
			mustNotHave: []string{"ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},
			mustHave:    []string{"ghp_***"},
		},
		{
			name:        "github fine-grained pat",
			in:          "use github_pat_ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789ABCDEFGHIJKLMN here",
			mustNotHave: []string{"github_pat_ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789ABCDEFGHIJKLMN"},
			mustHave:    []string{"github_pat_***"},
		},
		{
			name:        "bearer token",
			in:          "header: Bearer abcdefghij1234567890XYZ",
			mustNotHave: []string{"abcdefghij1234567890XYZ"},
			mustHave:    []string{"Bearer ***"},
		},
		{
			name:        "authorization header",
			in:          "Authorization: token ZZZZZZZZZZZZ\nnext",
			mustNotHave: []string{"ZZZZZZZZZZZZ"},
			mustHave:    []string{"Authorization: ***"},
		},
		{
			name:        "api-key flag",
			in:          "--api-key=AAAAAAAAAAAAAAAA --other",
			mustNotHave: []string{"AAAAAAAAAAAAAAAA"},
			mustHave:    []string{"--api-key ***"},
		},
		{
			name:        "password=value",
			in:          "password=hunter2-rocks",
			mustNotHave: []string{"hunter2-rocks"},
			mustHave:    []string{"password: ***"},
		},
		{
			name:        "user home path",
			in:          "file at /Users/alice/work/repo/main.go failed",
			mustNotHave: []string{"/Users/alice/"},
			mustHave:    []string{"/Users/<redacted>/"},
		},
		{
			name:        "linux home path",
			in:          "file at /home/bob/work/repo/main.go failed",
			mustNotHave: []string{"/home/bob/"},
			mustHave:    []string{"/home/<redacted>/"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Sanitize(c.in)
			for _, bad := range c.mustNotHave {
				if strings.Contains(got, bad) {
					t.Fatalf("expected %q to be redacted, got %q", bad, got)
				}
			}
			for _, good := range c.mustHave {
				if !strings.Contains(got, good) {
					t.Fatalf("expected %q in output, got %q", good, got)
				}
			}
		})
	}
}

func TestSanitize_Idempotent(t *testing.T) {
	inputs := []string{
		"plain text",
		"Bearer abcdefghij1234567890XYZ tail",
		"sk-abcdefghijklmnop and ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"Authorization: foo bar baz qux\n",
		"--api-key=ABCDEFGHIJKLMNOPQRSTUV --rest",
		"password: hunter2 token: foobar secret=abc",
		"/Users/me/projects/x /home/me/x",
	}
	for _, in := range inputs {
		first := Sanitize(in)
		second := Sanitize(first)
		if first != second {
			t.Fatalf("sanitize not idempotent for %q: first=%q second=%q", in, first, second)
		}
	}
}

func TestSanitizeErrorRedactsMessageAndPreservesCause(t *testing.T) {
	sentinel := errors.New("credential rejected")
	raw := fmt.Errorf("token=ghp_abcdefghijklmnopqrstuvwxyz1234567890AB: %w", sentinel)

	got := SanitizeError(raw)

	if strings.Contains(got.Error(), "abcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("SanitizeError() exposed credential: %q", got)
	}
	if !errors.Is(got, sentinel) {
		t.Fatal("SanitizeError() did not preserve wrapped cause")
	}
	if SanitizeError(got) != got {
		t.Fatal("SanitizeError() is not idempotent")
	}
}

func TestSanitize_TruncationBoundary(t *testing.T) {
	long := strings.Repeat("a", MaxRawExcerptBytes+500)
	got := Sanitize(long)
	if len(got) > MaxRawExcerptBytes {
		t.Fatalf("expected ≤%d bytes, got %d", MaxRawExcerptBytes, len(got))
	}
}

func TestSanitizeCredentials_RedactsCredentialPatterns(t *testing.T) {
	cases := []struct {
		name        string
		in          string
		mustNotHave []string
		mustHave    []string
	}{
		{
			name:        "anthropic-style key",
			in:          "use sk-abcdef1234567890QQQQ to call",
			mustNotHave: []string{"sk-abcdef1234567890"},
			mustHave:    []string{"sk-***"},
		},
		{
			name:        "github classic pat",
			in:          "token ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAA leaks",
			mustNotHave: []string{"ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},
			mustHave:    []string{"ghp_***"},
		},
		{
			name:        "github fine-grained pat",
			in:          "use github_pat_ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789ABCDEFGHIJKLMN here",
			mustNotHave: []string{"github_pat_ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789ABCDEFGHIJKLMN"},
			mustHave:    []string{"github_pat_***"},
		},
		{
			name:        "kandev pat",
			in:          "auth via kandev_pat_ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 succeeded",
			mustNotHave: []string{"kandev_pat_ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"},
			mustHave:    []string{"kandev_pat_***"},
		},
		{
			name:        "bearer token",
			in:          "header: Bearer abcdefghij1234567890XYZ",
			mustNotHave: []string{"abcdefghij1234567890XYZ"},
			mustHave:    []string{"Bearer ***"},
		},
		{
			name:        "authorization header",
			in:          "Authorization: token ZZZZZZZZZZZZ\nnext",
			mustNotHave: []string{"ZZZZZZZZZZZZ"},
			mustHave:    []string{"Authorization: ***"},
		},
		{
			name:        "api-key flag",
			in:          "--api-key=AAAAAAAAAAAAAAAA --other",
			mustNotHave: []string{"AAAAAAAAAAAAAAAA"},
			mustHave:    []string{"--api-key ***"},
		},
		{
			name:        "password=value",
			in:          "password=hunter2-rocks",
			mustNotHave: []string{"hunter2-rocks"},
			mustHave:    []string{"password: ***"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SanitizeCredentials(c.in)
			for _, bad := range c.mustNotHave {
				if strings.Contains(got, bad) {
					t.Fatalf("expected %q to be redacted, got %q", bad, got)
				}
			}
			for _, good := range c.mustHave {
				if !strings.Contains(got, good) {
					t.Fatalf("expected %q in output, got %q", good, got)
				}
			}
		})
	}
}

// TestSanitizeCredentials_PreservesNonCredentialContent is the collateral-
// damage guard: SanitizeCredentials must not run the 32+ char catch-all, the
// URL path/query rewrite, or the home-path normalization, so commit SHAs,
// UUIDs, and URLs with paths survive intact.
func TestSanitizeCredentials_PreservesNonCredentialContent(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{name: "40-char commit sha", in: "fixed in 94e7b02458b6c1a2d3e4f5061728394a5b6c7d8"},
		{name: "uuid", in: "task id 3f9a1c2e-4b5d-4e6f-8a9b-0c1d2e3f4a5b"},
		{name: "base64 sample", in: "payload: aGVsbG8gd29ybGQgdGhpcyBpcyBhIHRlc3Q="},
		{name: "url with path and query", in: "see https://example.com/org/repo/pull/123?tab=files for details"},
		{name: "user home path", in: "file at /Users/alice/work/repo/main.go failed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SanitizeCredentials(c.in)
			if got != c.in {
				t.Fatalf("expected content to survive unchanged, got %q from input %q", got, c.in)
			}
		})
	}
}

func TestSanitizeCredentials_Idempotent(t *testing.T) {
	inputs := []string{
		"plain text",
		"Bearer abcdefghij1234567890XYZ tail",
		"sk-abcdefghijklmnop and ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"Authorization: foo bar baz qux\n",
		"--api-key=ABCDEFGHIJKLMNOPQRSTUV --rest",
		"password: hunter2 token: foobar secret=abc",
		"commit 94e7b02458b6c1a2d3e4f5061728394a5b6c7d8",
	}
	for _, in := range inputs {
		first := SanitizeCredentials(in)
		second := SanitizeCredentials(first)
		if first != second {
			t.Fatalf("SanitizeCredentials not idempotent for %q: first=%q second=%q", in, first, second)
		}
	}
}

func TestSanitize_MultipleSecretsInOneString(t *testing.T) {
	in := "key sk-AAAAAAAAAAAAAAAA and Bearer BBBBBBBBBBBBBBBBBBBB and /Users/jane/code"
	got := Sanitize(in)
	if strings.Contains(got, "sk-AAAAAAAAAAAAAAAA") {
		t.Fatalf("sk- not redacted: %q", got)
	}
	if strings.Contains(got, "BBBBBBBBBBBBBBBBBBBB") {
		t.Fatalf("Bearer not redacted: %q", got)
	}
	if strings.Contains(got, "/Users/jane/") {
		t.Fatalf("home path not redacted: %q", got)
	}
}
