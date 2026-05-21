package share

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestRedactor_StringSecretRules(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		input      string
		wantRules  []string
		mustNotHas string // substring that must not appear in output
	}{
		{
			name:       "sk_token",
			input:      "key=sk-abcdefghijklmnopqrstuv0123456789 done",
			wantRules:  []string{RuleSecretSK},
			mustNotHas: "sk-abcdefghijklmnopqrstuv0123456789",
		},
		{
			name:       "ghp_token",
			input:      "GH_TOKEN=ghp_1234567890abcdefghijklmnopqrstuvwxyz",
			wantRules:  []string{RuleSecretGHP},
			mustNotHas: "ghp_1234567890",
		},
		{
			name:       "gho_token",
			input:      "Authorization: bearer gho_1234567890abcdefghijklmnopqrstuvwxyz",
			wantRules:  []string{RuleSecretGHO},
			mustNotHas: "gho_1234567890",
		},
		{
			name:       "github_pat_token",
			input:      "pat=github_pat_11ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789xyz",
			wantRules:  []string{RuleSecretGitHubPAT},
			mustNotHas: "github_pat_11ABCDEFG",
		},
		{
			name:       "aws_key",
			input:      "aws AKIAIOSFODNN7EXAMPLE rest",
			wantRules:  []string{RuleSecretAWS},
			mustNotHas: "AKIAIOSFODNN7EXAMPLE",
		},
		{
			name:      "no_secret",
			input:     "just a normal string with no juicy bits",
			wantRules: nil,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := NewRedactor("")
			got := r.String(tc.input)
			if tc.mustNotHas != "" && strings.Contains(got, tc.mustNotHas) {
				t.Fatalf("expected %q to be removed, got output: %q", tc.mustNotHas, got)
			}
			assertRulesEqual(t, r.Applied(), tc.wantRules)
		})
	}
}

func TestRedactor_AbsPath_RewritesToRelative(t *testing.T) {
	t.Parallel()
	r := NewRedactor("/Users/foo/proj")
	got := r.String("opened /Users/foo/proj/src/x.ts for editing")
	want := "opened src/x.ts for editing"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	assertRulesEqual(t, r.Applied(), []string{RuleAbsPath})
}

func TestRedactor_AbsPath_NoMatchLeavesUnchanged(t *testing.T) {
	t.Parallel()
	r := NewRedactor("/Users/foo/proj")
	got := r.String("nothing to redact here")
	if got != "nothing to redact here" {
		t.Fatalf("unexpected mutation: %q", got)
	}
	assertRulesEqual(t, r.Applied(), nil)
}

func TestRedactor_AbsPath_EmptyRootSkipsRule(t *testing.T) {
	t.Parallel()
	r := NewRedactor("")
	in := "see /Users/foo/proj/src/x.ts"
	got := r.String(in)
	if got != in {
		t.Fatalf("expected pass-through with empty root, got %q", got)
	}
	if len(r.Applied()) != 0 {
		t.Fatalf("expected no rules applied, got %v", r.Applied())
	}
}

func TestRedactor_JSON_DropsTopLevelEnvField(t *testing.T) {
	t.Parallel()
	r := NewRedactor("")
	in := json.RawMessage(`{"cmd":"ls","env":{"SECRET":"abc"}}`)
	out := r.JSON(in)

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("output not valid json: %v (%s)", err, string(out))
	}
	if _, present := got["env"]; present {
		t.Fatalf("env field should have been dropped, got %v", got)
	}
	if got["cmd"] != "ls" {
		t.Fatalf("cmd should be preserved, got %v", got)
	}
	assertRulesEqual(t, r.Applied(), []string{RuleEnvVars})
}

func TestRedactor_JSON_RedactsNestedStringSecrets(t *testing.T) {
	t.Parallel()
	r := NewRedactor("")
	in := json.RawMessage(`{"cmd":"curl","headers":["Authorization: token ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"]}`)
	out := r.JSON(in)
	if strings.Contains(string(out), "ghp_AAAA") {
		t.Fatalf("expected ghp_ token to be redacted in nested string, got %s", string(out))
	}
	assertRulesEqual(t, r.Applied(), []string{RuleSecretGHP})
}

func TestRedactor_JSON_InvalidJSONFallsBackToStringRedaction(t *testing.T) {
	t.Parallel()
	r := NewRedactor("")
	in := json.RawMessage(`not-json sk-abcdefghijklmnopqrstuv0123456789`)
	out := r.JSON(in)
	if strings.Contains(string(out), "sk-abcdefghij") {
		t.Fatalf("expected fallback to redact secret, got %s", string(out))
	}
	assertRulesEqual(t, r.Applied(), []string{RuleSecretSK})
}

func TestRedactor_RedactToolResult_EnvFileScrubbed(t *testing.T) {
	t.Parallel()
	r := NewRedactor("")
	output := "DATABASE_URL=postgres://user:pass@host/db\nAPI_KEY=12345\n"
	got := r.RedactToolResult(output, ".env.production")
	if got != envFilePlaceholder {
		t.Fatalf("expected env-file placeholder, got %q", got)
	}
	assertRulesEqual(t, r.Applied(), []string{RuleEnvFile})
}

func TestRedactor_RedactToolResult_NonEnvFileStillRedactsSecrets(t *testing.T) {
	t.Parallel()
	r := NewRedactor("")
	output := "token=ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	got := r.RedactToolResult(output, "src/main.go")
	if strings.Contains(got, "ghp_AAAA") {
		t.Fatalf("expected token to be redacted in non-env-file output, got %q", got)
	}
	assertRulesEqual(t, r.Applied(), []string{RuleSecretGHP})
}

func TestRedactor_NilSafe(t *testing.T) {
	t.Parallel()
	var r *Redactor // nil
	if got := r.String("hello"); got != "hello" {
		t.Fatalf("nil receiver should pass through, got %q", got)
	}
	if got := r.RedactToolResult("hello", ".env"); got != "hello" {
		t.Fatalf("nil receiver should pass through, got %q", got)
	}
	if got := r.Applied(); len(got) != 0 {
		t.Fatalf("nil receiver should report no rules, got %v", got)
	}
}

func TestRedactor_Applied_ReturnsSortedDistinct(t *testing.T) {
	t.Parallel()
	r := NewRedactor("/Users/foo/proj")
	r.String("token ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA in /Users/foo/proj/x.ts")
	r.String("again ghp_BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB and /Users/foo/proj/y.ts")
	got := r.Applied()
	want := []string{RuleAbsPath, RuleSecretGHP}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func assertRulesEqual(t *testing.T, got, want []string) {
	t.Helper()
	gotSet := make(map[string]struct{}, len(got))
	for _, g := range got {
		gotSet[g] = struct{}{}
	}
	wantSet := make(map[string]struct{}, len(want))
	for _, w := range want {
		wantSet[w] = struct{}{}
	}
	for w := range wantSet {
		if _, ok := gotSet[w]; !ok {
			t.Fatalf("expected rule %q to be applied; got %v", w, got)
		}
	}
	for g := range gotSet {
		if _, ok := wantSet[g]; !ok {
			t.Fatalf("unexpected extra rule %q applied; got %v want %v", g, got, want)
		}
	}
}
