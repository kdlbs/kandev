package handlers

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestBuildSnippet_UTF8Safe(t *testing.T) {
	tests := []struct {
		name    string
		content string
		query   string
	}{
		{
			name:    "rocket emoji near start",
			content: "Deploying 🚀 to production and waiting for the new release to roll out to every host in the fleet",
			query:   "production",
		},
		{
			name:    "CJK content",
			content: "部署中 プロダクション production 新しいリリースを待機中 — デプロイが完了するまでしばらくお待ちください",
			query:   "production",
		},
		{
			name:    "query lands inside multi-byte run",
			content: strings.Repeat("日本語テスト ", 20) + "match here " + strings.Repeat("日本語テスト ", 20),
			query:   "match",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildSnippet(tc.content, tc.query)
			if !utf8.ValidString(got) {
				t.Fatalf("snippet is not valid UTF-8: %q", got)
			}
			if !strings.Contains(strings.ToLower(got), strings.ToLower(tc.query)) {
				t.Errorf("snippet does not contain query %q: got %q", tc.query, got)
			}
		})
	}
}

func TestBuildSnippet_CaseInsensitiveWithUnicode(t *testing.T) {
	// Mixed-case query should match regardless of case.
	content := "Some long prefix before the match CaMeL inside the middle and a long tail after it"
	snippet := buildSnippet(content, "camel")
	if !strings.Contains(snippet, "CaMeL") {
		t.Errorf("expected snippet to include mixed-case match, got %q", snippet)
	}
}

func TestBuildSnippet_NoMatch_ReturnsLeadingSlice(t *testing.T) {
	content := strings.Repeat("x", 400)
	got := buildSnippet(content, "nope")
	if !utf8.ValidString(got) {
		t.Fatalf("snippet is not valid UTF-8")
	}
	// Should truncate with ellipsis when content exceeds max length.
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected leading-slice snippet to end with …, got %q", got)
	}
}
