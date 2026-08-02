package share

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/i18n"
)

func TestBuildShareHTML_RendersChatLayout(t *testing.T) {
	t.Parallel()
	completed := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	snap := &Snapshot{
		Version:    SnapshotVersion,
		ExportedAt: completed,
		Task:       TaskMeta{Title: "Investigate <flaky> test"},
		Session: SessionMeta{
			AgentType:    "claude-acp",
			Model:        "claude-opus-4-7",
			ExecutorType: "local_docker",
			StartedAt:    completed.Add(-time.Minute),
			CompletedAt:  &completed,
		},
		Messages: []Message{
			{Role: roleUser, Ts: completed, Blocks: []Block{{Kind: blockKindText, Text: "why is X flaky?"}}},
			{Role: roleAssistant, Ts: completed, Blocks: []Block{{Kind: blockKindText, Text: "Looking into it."}}},
			{Role: roleAssistant, Ts: completed, Blocks: []Block{
				{Kind: blockKindToolCall, ToolName: "shell", Text: "ran tests",
					Args: json.RawMessage(`{"cmd":"go test ./..."}`)},
			}},
			{Role: roleAssistant, Ts: completed, Blocks: []Block{
				{Kind: blockKindToolResult, Output: "FAIL pkg/foo TestX"},
				{Kind: blockKindText, Text: "Found it. Use `t.Cleanup`."},
				{Kind: blockKindDiff, Path: "src/x.go",
					UnifiedDiff: "--- a/x.go\n+++ b/x.go\n@@ -1,1 +1,1 @@\n-old\n+new\n context\n"},
			}},
		},
		Redaction: RedactionLog{AppliedRules: []string{RuleAbsPath}},
	}

	doc := BuildShareHTML(snap, "en")

	assertContains(t, doc, "<!doctype html>")
	assertContains(t, doc, "<title>Investigate &lt;flaky&gt; test — kandev share</title>")
	assertContains(t, doc, "<style>")
	if strings.Contains(doc, "<flaky>") {
		t.Fatal("unescaped angle brackets leaked into HTML")
	}

	assertContains(t, doc, ">claude-acp<")
	assertContains(t, doc, ">4 messages<")
	assertContains(t, doc, "abs-path")
}

func TestBuildShareHTML_GroupsConsecutiveAssistantMessages(t *testing.T) {
	t.Parallel()
	snap := &Snapshot{
		Task: TaskMeta{Title: "T"},
		Messages: []Message{
			{Role: roleUser, Blocks: []Block{{Kind: blockKindText, Text: "hi"}}},
			{Role: roleAssistant, Blocks: []Block{{Kind: blockKindText, Text: "a1"}}},
			{Role: roleAssistant, Blocks: []Block{{Kind: blockKindToolCall, ToolName: "shell"}}},
			{Role: roleAssistant, Blocks: []Block{{Kind: blockKindText, Text: "a2"}}},
			{Role: roleUser, Blocks: []Block{{Kind: blockKindText, Text: "ok"}}},
		},
	}
	doc := BuildShareHTML(snap, "en")
	// 3 groups expected: user, assistant (merged 3 messages), user — so 3 sections.
	if got := strings.Count(doc, `<section class="group `); got != 3 {
		t.Fatalf("expected 3 groups, got %d in:\n%s", got, doc)
	}
	// User-right + assistant-left CSS classes both present.
	assertContains(t, doc, "group-user")
	assertContains(t, doc, "group-assistant")
}

func TestBuildShareHTML_ToolCallIsCollapsedByDefault(t *testing.T) {
	t.Parallel()
	snap := &Snapshot{
		Task: TaskMeta{Title: "T"},
		Messages: []Message{
			{Role: roleAssistant, Blocks: []Block{
				{Kind: blockKindToolCall, ToolName: "shell", Text: "go test",
					Args: json.RawMessage(`{"cmd":"go test"}`)},
			}},
		},
	}
	doc := BuildShareHTML(snap, "en")
	// `<details>` without `open` is closed by default.
	if strings.Contains(doc, "<details class=\"tool tool-call\" open") {
		t.Fatal("tool call must not be open by default")
	}
	assertContains(t, doc, "<details class=\"tool tool-call\">")
	assertContains(t, doc, ">shell<")
	assertContains(t, doc, "go test")
}

func TestBuildShareHTML_DiffLineClasses(t *testing.T) {
	t.Parallel()
	snap := &Snapshot{
		Task: TaskMeta{Title: "T"},
		Messages: []Message{
			{Role: roleAssistant, Blocks: []Block{
				{Kind: blockKindDiff, Path: "src/x.go",
					UnifiedDiff: "--- a\n+++ b\n@@ -1,1 +1,1 @@\n-old\n+new\n ctx\n"},
			}},
		},
	}
	doc := BuildShareHTML(snap, "en")
	for _, cls := range []string{"diff-add", "diff-del", "diff-hunk", "diff-file", "diff-ctx"} {
		if !strings.Contains(doc, `class="`+cls+`"`) {
			t.Fatalf("missing class %q in diff: %s", cls, doc)
		}
	}
}

func TestBuildShareHTML_InlineBackticksAndFencedCode(t *testing.T) {
	t.Parallel()
	snap := &Snapshot{
		Task: TaskMeta{Title: "T"},
		Messages: []Message{
			{Role: roleAssistant, Blocks: []Block{
				{Kind: blockKindText, Text: "Use `t.Cleanup` to dispose.\n\n```go\nfunc TestX(t *testing.T) { t.Cleanup(cleanup) }\n```\n\nDone."},
			}},
		},
	}
	doc := BuildShareHTML(snap, "en")
	assertContains(t, doc, `<code>t.Cleanup</code>`)
	assertContains(t, doc, `<pre><code class="language-go">`)
	assertContains(t, doc, "func TestX(t *testing.T)")
	assertContains(t, doc, "<p>Done.</p>")
}

func TestBuildShareHTML_RendersMarkdownWithoutUnsafeHTML(t *testing.T) {
	t.Parallel()
	snap := &Snapshot{
		Task: TaskMeta{Title: "T"},
		Messages: []Message{
			{Role: roleAssistant, Blocks: []Block{{
				Kind: blockKindText,
				Text: "## Summary\n\n**Pushed** successfully.\n\n- tests passed\n- lint passed\n\n[docs](https://example.com)\n\n![tracker](https://attacker.example/pixel)\n\n[![linked tracker](https://attacker.example/pixel)](https://linked.example/target)\n\n<script>alert('xss')</script>\n\n[bad](javascript:alert('xss'))\n\n[data](data:text/html;base64,PHNjcmlwdD4=)",
			}}},
		},
	}

	doc := BuildShareHTML(snap, "en")

	assertContains(t, doc, "<h2>Summary</h2>")
	assertContains(t, doc, "<strong>Pushed</strong>")
	assertContains(t, doc, "<li>tests passed</li>")
	assertContains(t, doc, `<a href="https://example.com">docs</a>`)
	if strings.Contains(doc, "<script>") || strings.Contains(doc, `href="javascript:`) || strings.Contains(doc, `href="data:`) {
		t.Fatalf("unsafe markdown content leaked into rendered HTML:\n%s", doc)
	}
	if strings.Contains(doc, "<img") || strings.Contains(doc, "attacker.example") || strings.Contains(doc, "linked.example") {
		t.Fatalf("external markdown image leaked into rendered HTML:\n%s", doc)
	}
}

func TestBuildShareHTML_EmptyMessagesShowsPlaceholder(t *testing.T) {
	t.Parallel()
	doc := BuildShareHTML(&Snapshot{Task: TaskMeta{Title: "Empty"}}, "en")
	assertContains(t, doc, `class="empty"`)
	assertContains(t, doc, "No messages.")
}

func TestBuildShareHTML_NilSafe(t *testing.T) {
	t.Parallel()
	if got := BuildShareHTML(nil, "en"); got == "" {
		t.Fatal("nil snapshot should produce a placeholder document")
	}
}

// htmlFullKeys lists the catalog keys BuildShareHTML renders for a snapshot
// with content. Interpolated messages are asserted separately because T
// cannot resolve their placeholders.
var htmlFullKeys = []string{
	"share.brandTag",
	"share.roleYou",
	keyRoleAssistant,
	keyRoleSystem,
	keyToolOutput,
	keyToolOutputTruncated,
	"share.redacted",
	"share.ctaTryKandev",
}

// htmlEmptyKeys lists the copy that only appears on the degenerate path.
var htmlEmptyKeys = []string{
	keyUntitledTask,
	keyNoMessages,
	"share.brandTag",
	"share.ctaTryKandev",
}

func TestBuildShareHTML_LocalizesCopy(t *testing.T) {
	t.Parallel()
	for _, tc := range localizationCases(htmlFullKeys, htmlEmptyKeys) {
		tc := tc
		for _, locale := range []string{"en", "pseudo"} {
			locale := locale
			t.Run(tc.name+"/"+locale, func(t *testing.T) {
				t.Parallel()
				doc := BuildShareHTML(tc.snap, locale)
				// <html lang> and the messages must agree on the locale.
				assertContains(t, doc, `<html lang="`+locale+`">`)
				for _, key := range tc.keys {
					assertContains(t, doc, i18n.T(locale, key))
				}
				title := tc.snap.Task.Title
				if title == "" {
					title = i18n.T(locale, keyUntitledTask)
				}
				assertContains(t, doc, i18n.Tf(locale, "share.pageTitle",
					map[string]any{"title": title}))
				assertContains(t, doc, i18n.Tf(locale, keyMessageCount,
					map[string]any{"count": len(tc.snap.Messages)}))
			})
		}
		t.Run(tc.name+"/pseudo_differs", func(t *testing.T) {
			t.Parallel()
			assertPseudoDiffers(t, BuildShareHTML(tc.snap, "pseudo"), tc.keys)
		})
	}
}

// TestBuildShareHTML_UnsupportedLocaleFallsBack pins that a locale we have no
// catalog for degrades to English in both <html lang> and the copy, rather
// than emitting a lang attribute the browser cannot use.
func TestBuildShareHTML_UnsupportedLocaleFallsBack(t *testing.T) {
	t.Parallel()
	doc := BuildShareHTML(localizationSnapshot(), "klingon")
	assertContains(t, doc, `<html lang="en">`)
	assertContains(t, doc, i18n.T("en", "share.ctaTryKandev"))
}

// TestMessageRoleAttrs_TranslatesOnlyTheLabel guards the trap in this
// function: it returns a CSS class and an avatar next to the label, and the
// unknown-role branch echoes the raw wire value from the message store. Only
// the label may ever change with the locale.
func TestMessageRoleAttrs_TranslatesOnlyTheLabel(t *testing.T) {
	t.Parallel()
	for _, role := range []string{roleUser, roleAssistant, roleSystem} {
		enCls, enLabel, enIcon := messageRoleAttrs(role, "en")
		psCls, psLabel, psIcon := messageRoleAttrs(role, "pseudo")
		if enCls != psCls {
			t.Fatalf("role %q: CSS class changed with locale (%q vs %q)", role, enCls, psCls)
		}
		if enIcon != psIcon {
			t.Fatalf("role %q: avatar changed with locale (%q vs %q)", role, enIcon, psIcon)
		}
		if enLabel == psLabel {
			t.Fatalf("role %q: label %q is identical in both locales — it is hardcoded", role, enLabel)
		}
	}
	// An unrecognised role is wire data, not copy: it is echoed verbatim.
	cls, label, _ := messageRoleAttrs("reviewer", "pseudo")
	if cls != "other" || label != "reviewer" {
		t.Fatalf("unknown role should pass through, got cls=%q label=%q", cls, label)
	}
}

func TestGroupMessages_RunsFusedAcrossEmptyMessages(t *testing.T) {
	t.Parallel()
	groups := groupMessages([]Message{
		{Role: roleAssistant, Blocks: []Block{{Kind: blockKindText, Text: "a"}}},
		{Role: roleAssistant, Blocks: []Block{}}, // dropped (empty)
		{Role: roleAssistant, Blocks: []Block{{Kind: blockKindText, Text: "b"}}},
	})
	if len(groups) != 1 || len(groups[0].blocks) != 2 {
		t.Fatalf("expected 1 group with 2 blocks, got %+v", groups)
	}
}

func TestRenderedURLForGist(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"authenticated_gist", "https://gist.github.com/jane/abc123", "https://gist.githack.com/jane/abc123/raw/share.html"},
		// Anonymous gists lack the owner segment githack needs to address the
		// file; we return "" so callers fall back to whatever was stored.
		{"anonymous_gist", "https://gist.github.com/abc123", ""},
		{"wrong_host", "https://example.com/jane/abc123", ""},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := renderedURLForGist(tc.input); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestOwnerAndIDFromGithackURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		input     string
		wantOwner string
		wantID    string
	}{
		{"valid_url", "https://gist.githack.com/jane/abc123/raw/share.html", "jane", "abc123"},
		{"wrong_host", "https://example.com/jane/abc123/raw/share.html", "", ""},
		{"too_short", "https://gist.githack.com/jane", "", ""},
		{"empty", "", "", ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			owner, id := ownerAndIDFromGithackURL(tc.input)
			if owner != tc.wantOwner || id != tc.wantID {
				t.Fatalf("got (%q,%q), want (%q,%q)", owner, id, tc.wantOwner, tc.wantID)
			}
		})
	}
}
