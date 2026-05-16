package routingerr

import (
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

func TestSanitize_TruncationBoundary(t *testing.T) {
	long := strings.Repeat("a", MaxRawExcerptBytes+500)
	got := Sanitize(long)
	if len(got) > MaxRawExcerptBytes {
		t.Fatalf("expected ≤%d bytes, got %d", MaxRawExcerptBytes, len(got))
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
