package share

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/i18n"
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

	md := BuildGistREADME(snap, "https://gist.githack.com/jane/mock-gist-1/raw/share.html", "en")
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
	if got := BuildGistREADME(nil, "", "en"); got == "" {
		t.Fatal("nil snapshot should still produce a non-empty README")
	}
	empty := &Snapshot{Task: TaskMeta{Title: "Untitled"}}
	got := BuildGistREADME(empty, "https://gist.githack.com/jane/g1/raw/share.html", "en")
	if !strings.Contains(got, "_(No messages.)_") {
		t.Fatalf("expected empty-messages placeholder, got: %s", got)
	}
}

// markdownFullKeys lists the catalog keys BuildGistREADME renders for a
// snapshot with content. Interpolated messages are asserted separately
// because T cannot resolve their placeholders.
var markdownFullKeys = []string{
	"share.roleUser",
	keyRoleAssistant,
	keyRoleSystem,
	keyToolOutput,
	keyToolOutputTruncated,
	"share.emptyMessage",
	"share.redactedBeforePublish",
	"share.sessionDetails",
	"share.metaAgent",
	"share.metaModel",
	"share.metaExecutor",
	"share.metaStarted",
	"share.metaCompleted",
	"share.metaWorkflowStep",
	"share.rawExport",
	"share.builtWith",
}

// markdownEmptyKeys lists the copy that only appears on the degenerate path.
var markdownEmptyKeys = []string{
	keyUntitledTask,
	keyNoMessages,
	"share.noMetadata",
	"share.sessionDetails",
	"share.rawExport",
	"share.builtWith",
}

func TestBuildGistREADME_LocalizesCopy(t *testing.T) {
	t.Parallel()
	for _, tc := range localizationCases(markdownFullKeys, markdownEmptyKeys) {
		tc := tc
		for _, locale := range []string{"en", "pseudo"} {
			locale := locale
			t.Run(tc.name+"/"+locale, func(t *testing.T) {
				t.Parallel()
				md := BuildGistREADME(tc.snap, "", locale)
				for _, key := range tc.keys {
					assertContains(t, md, i18n.T(locale, key))
				}
				assertContains(t, md, i18n.Tf(locale, keyMessageCount,
					map[string]any{"count": len(tc.snap.Messages)}))
				assertContains(t, md, i18n.Tf(locale, "share.pitch",
					map[string]any{"url": kandevRepoURL}))
			})
		}
		t.Run(tc.name+"/pseudo_differs", func(t *testing.T) {
			t.Parallel()
			assertPseudoDiffers(t, BuildGistREADME(tc.snap, "", "pseudo"), tc.keys)
		})
	}
}

// TestBuildGistREADME_LocalizesBothCTAs covers the two mutually exclusive
// call-to-action branches, and pins that the interpolated filename and URL
// survive the pseudo locale untransliterated — a transliterated `share.html`
// or gist URL would be a dead pointer.
func TestBuildGistREADME_LocalizesBothCTAs(t *testing.T) {
	t.Parallel()
	const rendered = "https://gist.githack.com/jane/g1/raw/share.html"
	for _, locale := range []string{"en", "pseudo"} {
		locale := locale
		t.Run(locale, func(t *testing.T) {
			t.Parallel()
			withoutURL := BuildGistREADME(localizationSnapshot(), "", locale)
			assertContains(t, withoutURL, i18n.Tf(locale, "share.openShareHTML",
				map[string]any{"file": "`share.html`"}))
			assertContains(t, withoutURL, "`share.html`")

			withURL := BuildGistREADME(localizationSnapshot(), rendered, locale)
			assertContains(t, withURL, i18n.Tf(locale, "share.openRenderedView",
				map[string]any{"url": rendered}))
			assertContains(t, withURL, rendered)
		})
	}
}

// TestMessageHeading_TranslatesOnlyTheLabel mirrors the HTML builder's
// TestMessageRoleAttrs_TranslatesOnlyTheLabel: the avatar is decoration, the
// label is copy, and an unrecognised role is wire data echoed through.
func TestMessageHeading_TranslatesOnlyTheLabel(t *testing.T) {
	t.Parallel()
	for _, role := range []string{roleUser, roleAssistant, roleSystem} {
		en, pseudo := messageHeading(role, "en"), messageHeading(role, "pseudo")
		if en == pseudo {
			t.Fatalf("role %q: heading %q is identical in both locales — it is hardcoded", role, en)
		}
		// The avatar is the first rune and must not change with the locale.
		if []rune(en)[0] != []rune(pseudo)[0] {
			t.Fatalf("role %q: avatar changed with locale (%q vs %q)", role, en, pseudo)
		}
	}
	// An unrecognised role is wire data: passed through, and escaped because the
	// heading is rendered as HTML by GitHub.
	if got := messageHeading("reviewer", "pseudo"); got != "reviewer" {
		t.Fatalf("unknown role should pass through verbatim, got %q", got)
	}
	if got := messageHeading("<img src=x>", "en"); got != "&lt;img src=x&gt;" {
		t.Fatalf("unknown role must be HTML-escaped, got %q", got)
	}
}

// localizationCase pairs a fixture with the catalog keys its render must
// contain. Two are needed per builder because some copy only appears on the
// degenerate path — an untitled task with no messages and no metadata.
type localizationCase struct {
	name string
	snap *Snapshot
	keys []string
}

func localizationCases(fullKeys, emptyKeys []string) []localizationCase {
	return []localizationCase{
		{name: "full", snap: localizationSnapshot(), keys: fullKeys},
		{name: "degenerate", snap: &Snapshot{}, keys: emptyKeys},
	}
}

// localizationSnapshot exercises every branch of both builders that renders
// copy: all three roles, a plain and a truncated tool result, a message whose
// blocks are all blank, a redaction note, and complete session metadata.
// Every value is deliberately free of English words so the pseudo-locale
// assertions cannot pass by coincidence.
func localizationSnapshot() *Snapshot {
	completed := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	return &Snapshot{
		Task: TaskMeta{Title: "Fixture", WorkflowStep: "step-1"},
		Session: SessionMeta{
			AgentType:    "claude-acp",
			Model:        "claude-opus-4-7",
			ExecutorType: "local_docker",
			StartedAt:    completed.Add(-time.Minute),
			CompletedAt:  &completed,
		},
		Messages: []Message{
			{Role: roleUser, Blocks: []Block{{Kind: blockKindText, Text: "q"}}},
			{Role: roleAssistant, Blocks: []Block{{Kind: blockKindToolResult, Output: "out"}}},
			{Role: roleAssistant, Blocks: []Block{
				{Kind: blockKindToolResult, Output: "cut", Truncated: true},
			}},
			{Role: roleSystem, Blocks: []Block{{Kind: blockKindText, Text: "s"}}},
			// Blocks present but all blank — drives the "(empty)" placeholder.
			{Role: roleUser, Blocks: []Block{{Kind: blockKindText, Text: "   "}}},
		},
		Redaction: RedactionLog{AppliedRules: []string{RuleAbsPath}},
	}
}

// assertPseudoDiffers is the real proof that a string is externalized: a
// hardcoded literal renders byte-identically in every locale, so finding the
// English message inside a pseudo-locale render means the key is decorative
// and the call site never went through the catalog.
func assertPseudoDiffers(t *testing.T, pseudoDoc string, keys []string) {
	t.Helper()
	for _, key := range keys {
		english := i18n.T("en", key)
		if strings.Contains(pseudoDoc, english) {
			t.Fatalf("key %q still renders its English message %q under the pseudo locale — the string is hardcoded",
				key, english)
		}
	}
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("expected output to contain %q\n--- output ---\n%s", needle, haystack)
	}
}
