package routingerr

import (
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// fuzzIdempotenceIterations bounds the seeded idempotence fuzz tests (see
// TestSanitize_IdempotentFuzz / TestSanitizeCredentials_IdempotentFuzz).
const fuzzIdempotenceIterations = 20000

// credentialFuzzAlphabet is a small vocabulary of tokens combined randomly
// by randCredentialString. It is dense in the shapes redactAssignments and
// scanValue branch on — quotes, separators, structural delimiters, and
// keyword fragments — so a seeded run reliably explores the boundary cases a
// fixed golden list would only cover by accident.
var credentialFuzzAlphabet = []string{
	`"`, `'`, ":", "=", " ", "\t", "\n", ",", "}", ")", "]", ">", ";", `\`,
	"password", "secret", "token", "api_key", "apikey", "api-key",
	"max_tokens", "AWS_SECRET_ACCESS_KEY",
	"a", "1", "2", "_", "-", ".", "@", "hunter2", "x", "kid-1234",
}

func randCredentialString(rng *rand.Rand) string {
	n := rng.Intn(12) + 1
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(credentialFuzzAlphabet[rng.Intn(len(credentialFuzzAlphabet))])
	}
	return b.String()
}

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
			name:        "password value contains a quote",
			in:          "password=p@ss'word-tail",
			mustNotHave: []string{"word-tail", "p@ss"},
			mustHave:    []string{"password: ***"},
		},
		{
			name:        "secret value contains multiple quotes",
			in:          "secret=abc'def'ghi",
			mustNotHave: []string{"abc", "def", "ghi"},
			mustHave:    []string{"secret: ***"},
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
		{
			name:        "quoted value with unescaped trailing quote leaks tail",
			in:          `token: "a"LEAKED"`,
			mustNotHave: []string{"LEAKED"},
			mustHave:    []string{"token: ***"},
		},
		{
			name:        "quoted value with embedded at-sign then trailing quote",
			in:          `password="p@ss"LEAKED"`,
			mustNotHave: []string{"LEAKED", "p@ss"},
			mustHave:    []string{"password: ***"},
		},
		{
			name:        "single-quoted value with trailing quote",
			in:          `secret='x'LEAKED'`,
			mustNotHave: []string{"LEAKED"},
			mustHave:    []string{"secret: ***"},
		},
		{
			name:        "JSON-escaped quote inside quoted value",
			in:          `{"password":"abc\"LEAKED_SUFFIX"}`,
			mustNotHave: []string{"LEAKED_SUFFIX"},
			mustHave:    []string{"{password: ***}"},
		},
		{
			name:        "bare value starting with comma delimiter",
			in:          "password=,LEAKED",
			mustNotHave: []string{"LEAKED"},
			mustHave:    []string{"password: ***"},
		},
		{
			name:        "bare value starting with close brace",
			in:          "secret=}LEAKED",
			mustNotHave: []string{"LEAKED"},
			mustHave:    []string{"secret: ***"},
		},
		{
			name:        "bare value starting with semicolon",
			in:          "token=;LEAKED",
			mustNotHave: []string{"LEAKED"},
			mustHave:    []string{"token: ***"},
		},
		{
			name:        "bare value starting with close paren",
			in:          "password=)LEAKED",
			mustNotHave: []string{"LEAKED"},
			mustHave:    []string{"password: ***"},
		},
		{
			name:        "nested JSON quoted token value stops at closing brace",
			in:          `{"a": {"token": "hunter2"}, "b": 1}`,
			mustNotHave: []string{"hunter2"},
			mustHave:    []string{`{"a": {token: ***}, "b": 1}`},
		},
		{
			name:        "quoted value containing spaces",
			in:          `secret: "multi word secret"`,
			mustNotHave: []string{"multi word secret"},
			mustHave:    []string{"secret: ***"},
		},
		{
			name:        "quoted JSON password value stops at closing brace",
			in:          `{"password": "hunter2"}`,
			mustNotHave: []string{"hunter2"},
			mustHave:    []string{"{password: ***}"},
		},
		{
			name:        "bare token value stops at trailing brace",
			in:          "token=hunter2}",
			mustNotHave: []string{"hunter2"},
			mustHave:    []string{"token: ***}"},
		},
		{
			name:        "bare value containing a comma is fully consumed",
			in:          "password=abc,def",
			mustNotHave: []string{"abc", "def"},
			mustHave:    []string{"password: ***"},
		},
		{
			name:        "bare value containing a semicolon is fully consumed",
			in:          "token=abc;def",
			mustNotHave: []string{"abc", "def"},
			mustHave:    []string{"token: ***"},
		},
		{
			name:        "max_tokens numeric value is not redacted",
			in:          "max_tokens: 8192 > 4096, which is the maximum allowed for this model",
			mustNotHave: []string{"***"},
			mustHave:    []string{"max_tokens: 8192 > 4096"},
		},
		{
			name:        "input_tokens and output_tokens numeric values are not redacted",
			in:          "input_tokens=12000 output_tokens=900 total cost 0.12",
			mustNotHave: []string{"***"},
			mustHave:    []string{"input_tokens=12000 output_tokens=900"},
		},
		{
			name:        "max_tokens numeric value in parens is not redacted",
			in:          "context window exceeded (max_tokens=4096)",
			mustNotHave: []string{"***"},
			mustHave:    []string{"(max_tokens=4096)"},
		},
		{
			name:        "tokens numeric value is not redacted",
			in:          "tokens: 500 remaining",
			mustNotHave: []string{"***"},
			mustHave:    []string{"tokens: 500 remaining"},
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

// TestSanitize_IdempotentFuzz replaces a fixed golden list with a seeded
// fuzz over a credential-flavoured alphabet: a golden list only proves
// idempotence for the handful of inputs it enumerates, and a prior regression
// (an unbalanced optional quote consumed by the key match but never
// re-emitted) shrank output by one byte on the second pass for inputs the
// golden list did not happen to cover.
func TestSanitize_IdempotentFuzz(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < fuzzIdempotenceIterations; i++ {
		s := randCredentialString(rng)
		first := Sanitize(s)
		second := Sanitize(first)
		if first != second {
			t.Fatalf("Sanitize not idempotent for %q: first=%q second=%q", s, first, second)
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
		{
			name:        "url userinfo credential",
			in:          "clone from https://alice:s3cr3tpassw0rd@example.test/api",
			mustNotHave: []string{"s3cr3tpassw0rd", "alice:s3cr3tpassw0rd@"},
			mustHave:    []string{"https://***@example.test/api"},
		},
		{
			name:        "api_key query param",
			in:          "callback https://example.test/callback?api_key=abc123def456",
			mustNotHave: []string{"abc123def456"},
			mustHave:    []string{"api_key: ***"},
		},
		{
			name:        "non-http scheme userinfo credential",
			in:          "repro via postgres://admin:hunter2@db.internal:5432/app",
			mustNotHave: []string{"hunter2", "admin:hunter2@"},
			mustHave:    []string{"postgres://***@db.internal:5432/app"},
		},
		{
			name:        "quoted JSON password field",
			in:          `{"password": "hunter2-rocks"}`,
			mustNotHave: []string{"hunter2-rocks"},
			mustHave:    []string{"password: ***"},
		},
		{
			name:        "secret=value",
			in:          "secret=hunter2-rocks",
			mustNotHave: []string{"hunter2-rocks"},
			mustHave:    []string{"secret: ***"},
		},
		{
			name:        "token=value",
			in:          "token=hunter2-rocks",
			mustNotHave: []string{"hunter2-rocks"},
			mustHave:    []string{"token: ***"},
		},
		{
			name:        "password value contains a quote",
			in:          "password=p@ss'word-tail",
			mustNotHave: []string{"word-tail", "p@ss"},
			mustHave:    []string{"password: ***"},
		},
		{
			name:        "secret value contains multiple quotes",
			in:          "secret=abc'def'ghi",
			mustNotHave: []string{"abc", "def", "ghi"},
			mustHave:    []string{"secret: ***"},
		},
		{
			name:        "qualified env var key AWS_SECRET_ACCESS_KEY",
			in:          "AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMIK7MDENGbPxRfiCYEXAMPLEKEY",
			mustNotHave: []string{"wJalrXUtnFEMIK7MDENGbPxRfiCYEXAMPLEKEY"},
			mustHave:    []string{"***"},
		},
		{
			name:        "qualified env var key SECRET_KEY",
			in:          "SECRET_KEY=django-insecure-abc123def456ghi789",
			mustNotHave: []string{"django-insecure-abc123def456ghi789"},
			mustHave:    []string{"***"},
		},
		{
			name:        "quoted value with unescaped trailing quote leaks tail",
			in:          `token: "a"LEAKED"`,
			mustNotHave: []string{"LEAKED"},
			mustHave:    []string{"token: ***"},
		},
		{
			name:        "quoted value with embedded at-sign then trailing quote",
			in:          `password="p@ss"LEAKED"`,
			mustNotHave: []string{"LEAKED", "p@ss"},
			mustHave:    []string{"password: ***"},
		},
		{
			name:        "single-quoted value with trailing quote",
			in:          `secret='x'LEAKED'`,
			mustNotHave: []string{"LEAKED"},
			mustHave:    []string{"secret: ***"},
		},
		{
			name:        "JSON-escaped quote inside quoted value",
			in:          `{"password":"abc\"LEAKED_SUFFIX"}`,
			mustNotHave: []string{"LEAKED_SUFFIX"},
			mustHave:    []string{"{password: ***}"},
		},
		{
			name:        "bare value starting with comma delimiter",
			in:          "password=,LEAKED",
			mustNotHave: []string{"LEAKED"},
			mustHave:    []string{"password: ***"},
		},
		{
			name:        "bare value starting with close brace",
			in:          "secret=}LEAKED",
			mustNotHave: []string{"LEAKED"},
			mustHave:    []string{"secret: ***"},
		},
		{
			name:        "bare value starting with semicolon",
			in:          "token=;LEAKED",
			mustNotHave: []string{"LEAKED"},
			mustHave:    []string{"token: ***"},
		},
		{
			name:        "bare value starting with close paren",
			in:          "password=)LEAKED",
			mustNotHave: []string{"LEAKED"},
			mustHave:    []string{"password: ***"},
		},
		{
			name:        "url userinfo with multiple at-signs leaks tail past first",
			in:          "postgres://alice:prefix@LEAKED_SUFFIX@db.internal/app",
			mustNotHave: []string{"LEAKED_SUFFIX", "alice:prefix"},
			mustHave:    []string{"postgres://***@db.internal/app"},
		},
		{
			name:        "nested JSON quoted token value stops at closing brace",
			in:          `{"a": {"token": "hunter2"}, "b": 1}`,
			mustNotHave: []string{"hunter2"},
			mustHave:    []string{`{"a": {token: ***}, "b": 1}`},
		},
		{
			name:        "quoted value containing spaces",
			in:          `secret: "multi word secret"`,
			mustNotHave: []string{"multi word secret"},
			mustHave:    []string{"secret: ***"},
		},
		{
			name:        "quoted JSON password value stops at closing brace",
			in:          `{"password": "hunter2"}`,
			mustNotHave: []string{"hunter2"},
			mustHave:    []string{"{password: ***}"},
		},
		{
			name:        "bare token value stops at trailing brace",
			in:          "token=hunter2}",
			mustNotHave: []string{"hunter2"},
			mustHave:    []string{"token: ***}"},
		},
		{
			name:        "bare value containing a comma is fully consumed",
			in:          "password=abc,def",
			mustNotHave: []string{"abc", "def"},
			mustHave:    []string{"password: ***"},
		},
		{
			name:        "bare value containing a semicolon is fully consumed",
			in:          "token=abc;def",
			mustNotHave: []string{"abc", "def"},
			mustHave:    []string{"token: ***"},
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
		{name: "max_tokens numeric value is not redacted", in: "max_tokens: 8192 > 4096, which is the maximum allowed for this model"},
		{name: "input_tokens and output_tokens numeric values are not redacted", in: "input_tokens=12000 output_tokens=900 total cost 0.12"},
		{name: "max_tokens numeric value in parens is not redacted", in: "context window exceeded (max_tokens=4096)"},
		{name: "tokens numeric value is not redacted", in: "tokens: 500 remaining"},
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

// TestSanitizeCredentials_IdempotentFuzz is SanitizeCredentials' counterpart
// to TestSanitize_IdempotentFuzz; see its comment for why this replaced a
// golden list.
func TestSanitizeCredentials_IdempotentFuzz(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	for i := 0; i < fuzzIdempotenceIterations; i++ {
		s := randCredentialString(rng)
		first := SanitizeCredentials(s)
		second := SanitizeCredentials(first)
		if first != second {
			t.Fatalf("SanitizeCredentials not idempotent for %q: first=%q second=%q", s, first, second)
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
