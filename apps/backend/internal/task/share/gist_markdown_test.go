package share

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBuildGistREADME_FullConversation(t *testing.T) {
	t.Parallel()
	completed := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	snap := &Snapshot{
		Version:    SnapshotVersion,
		ExportedAt: completed,
		Task:       TaskMeta{Title: "Investigate flaky test"},
		Session: SessionMeta{
			AgentType:    "claude-acp",
			Model:        "claude-opus-4-7",
			ExecutorType: "local_docker",
			StartedAt:    completed.Add(-time.Minute),
			CompletedAt:  &completed,
		},
		Messages: []Message{
			{Role: roleUser, Ts: completed, Blocks: []Block{{Kind: blockKindText, Text: "why is X flaky?"}}},
			{
				Role: roleAssistant, Ts: completed,
				Blocks: []Block{
					{Kind: blockKindText, Text: "Looking into it."},
					{
						Kind:     blockKindToolCall,
						ToolName: "shell",
						Text:     "ran tests",
						Args:     json.RawMessage(`{"cmd":"go test ./..."}`),
					},
					{Kind: blockKindToolResult, Output: "FAIL pkg/foo TestX"},
					{
						Kind:        blockKindDiff,
						Path:        "src/x.go",
						UnifiedDiff: "--- a\n+++ b\n@@\n-old\n+new\n",
					},
				},
			},
		},
		Redaction: RedactionLog{AppliedRules: []string{RuleAbsPath}},
	}

	md := BuildGistREADME(snap, "https://gist.githack.com/jane/mock-gist-1/raw/share.html")
	assertContains(t, md, "# Investigate flaky test")
	assertContains(t, md, "<kbd>claude-acp</kbd>")
	assertContains(t, md, "<kbd>claude-opus-4-7</kbd>")
	assertContains(t, md, "<kbd>local_docker</kbd>")
	assertContains(t, md, "📊 Session details")
	assertContains(t, md, "| **Agent** | claude-acp |")
	assertContains(t, md, "Redacted before publish:")
	assertContains(t, md, "🧑 User")
	// User text is wrapped in a blockquote for visual accent.
	assertContains(t, md, "> why is X flaky?")
	assertContains(t, md, "🤖 Assistant")
	assertContains(t, md, "🔧 <strong>shell</strong>")
	// Tool args and output render via HTML <pre><code> rather than a
	// triple-backtick fence so payloads containing ``` can't break out.
	assertContains(t, md, `<pre><code class="language-json">`)
	assertContains(t, md, `"cmd": "go test ./..."`)
	assertContains(t, md, "📤 Tool output")
	assertContains(t, md, "FAIL pkg/foo TestX")
	assertContains(t, md, "**📝 `src/x.go`**")
	assertContains(t, md, "```diff")
	assertContains(t, md, "snapshot.json")
	assertContains(t, md, "github.com/kdlbs/kandev")
}

func TestBuildGistREADME_NilAndEmpty(t *testing.T) {
	t.Parallel()
	if got := BuildGistREADME(nil, ""); got == "" {
		t.Fatal("nil snapshot should still produce a non-empty README")
	}
	empty := &Snapshot{Task: TaskMeta{Title: "Untitled"}}
	got := BuildGistREADME(empty, "https://gist.githack.com/jane/g1/raw/share.html")
	if !strings.Contains(got, "_(No messages.)_") {
		t.Fatalf("expected empty-messages placeholder, got: %s", got)
	}
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("expected output to contain %q\n--- output ---\n%s", needle, haystack)
	}
}
