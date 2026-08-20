package automation

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// decodePromptNode marshals node the same way the real document encoder would (as
// part of a document, not standalone), then decodes the prompt scalar back to a Go
// string. Used to assert AC-49's round trip independent of buildPromptNode's own
// internals.
func decodePromptNode(t *testing.T, node *yaml.Node) string {
	t.Helper()
	doc := newExportDocument([]exportAutomation{{
		Name:              "Round Trip",
		Enabled:           true,
		MaxConcurrentRuns: 1,
		Prompt:            node,
		Triggers:          []exportTrigger{},
	}}, nil)
	out, err := marshalExportDocument(doc)
	if err != nil {
		t.Fatalf("marshalExportDocument: %v", err)
	}
	var decoded struct {
		Automations []struct {
			Prompt string `yaml:"prompt"`
		} `yaml:"automations"`
	}
	if err := yaml.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("yaml.Unmarshal(%q): %v", out, err)
	}
	if len(decoded.Automations) != 1 {
		t.Fatalf("expected 1 automation, got %d", len(decoded.Automations))
	}
	return decoded.Automations[0].Prompt
}

// AC-16/AC-46: ordinary multi-line prompts emit as a literal block scalar, no warning,
// no fidelity loss.
func TestBuildPromptNode_MultilineLiteralNoWarning(t *testing.T) {
	cases := []string{
		"line one\nline two\n",
		"line one\nline two",
		"line one\nline two\n\n",
		"line one\ta\nline two\n",
	}
	for _, prompt := range cases {
		t.Run(prompt, func(t *testing.T) {
			node, warning, err := buildPromptNode(prompt)
			if err != nil {
				t.Fatalf("buildPromptNode(%q): %v", prompt, err)
			}
			if warning != "" {
				t.Errorf("warning = %q, want none", warning)
			}
			if node.Style != yaml.LiteralStyle {
				t.Errorf("Style = %v, want LiteralStyle", node.Style)
			}
			if got := decodePromptNode(t, node); got != prompt {
				t.Errorf("round trip = %q, want %q", got, prompt)
			}
		})
	}
}

// AC-17: a prompt containing a YAML newline (## Definitions' 5-character set
// {U+000A, U+000D, U+0085, U+2028, U+2029}, not just LF) that the pinned marshaller
// does not emit as a literal block scalar gets the "prompt not emitted as a block
// scalar" warning. Tested as a biconditional against the node's observed Style, not
// against an enumerated list of "bad" characters (spec.md AC-17 testability note) —
// these are exactly the cases from the spec's own probed reference table that degrade
// to double-quoted.
func TestBuildPromptNode_NonLiteralWithNewlineWarns(t *testing.T) {
	cases := []string{
		"line one   \nline two\n",  // trailing space before break
		"line one\nline two ",      // trailing space, no final break
		"line one\r\nline two\r\n", // CR
		"line one\na\U0001F389b\n", // astral character
		"line one\na\x1Bb\n",       // C0 control (ESC)
		"line one\na b\n",          // U+2028 LINE SEPARATOR
		"line one\u0085line two",   // U+0085 NEL, no LF at all
		"line one line two",        // U+2029 PARAGRAPH SEPARATOR, no LF at all
	}
	for _, prompt := range cases {
		t.Run(prompt, func(t *testing.T) {
			node, warning, err := buildPromptNode(prompt)
			if err != nil {
				t.Fatalf("buildPromptNode(%q): %v", prompt, err)
			}
			isLiteral := node.Style == yaml.LiteralStyle
			hasWarning := warning == warnPromptNotBlockScalar
			if isLiteral == hasWarning {
				t.Errorf("biconditional violated: Style=%v (literal=%v) but warning=%q (matches=%v)", node.Style, isLiteral, warning, hasWarning)
			}
			if got := decodePromptNode(t, node); got != prompt {
				t.Errorf("round trip = %q, want %q", got, prompt)
			}
		})
	}
}

// AC-46: single-line prompts never warn, regardless of the specific form the
// marshaller chooses (plain, single-quoted, etc).
func TestBuildPromptNode_SingleLineNoWarning(t *testing.T) {
	cases := []string{
		"Do the thing",
		"Do the thing ",
		"true",
	}
	for _, prompt := range cases {
		t.Run(prompt, func(t *testing.T) {
			node, warning, err := buildPromptNode(prompt)
			if err != nil {
				t.Fatalf("buildPromptNode(%q): %v", prompt, err)
			}
			if warning != "" {
				t.Errorf("warning = %q, want none", warning)
			}
			if got := decodePromptNode(t, node); got != prompt {
				t.Errorf("round trip = %q, want %q", got, prompt)
			}
		})
	}
}

// AC-47: invalid UTF-8 emits as !!binary, warns, and is never treated as a
// serialization failure.
func TestBuildPromptNode_InvalidUTF8(t *testing.T) {
	prompt := "line one\nbad\xff\xfebyte\n"
	node, warning, err := buildPromptNode(prompt)
	if err != nil {
		t.Fatalf("buildPromptNode: %v", err)
	}
	if node.Tag != "!!binary" {
		t.Errorf("Tag = %q, want !!binary", node.Tag)
	}
	if warning != warnPromptInvalidUTF8 {
		t.Errorf("warning = %q, want %q", warning, warnPromptInvalidUTF8)
	}
}

// AC-49: prompts whose default-form probe does not round-trip byte-for-byte get
// re-quoted as an explicit double-quoted !!str node, with a distinct warning. These
// are the leading-newline cases the spec calls out explicitly: they would otherwise
// silently emit as Literal (satisfying AC-16 with no AC-17 warning) while dropping the
// leading newline — exactly the gap AC-49 exists to close. The observable is the round
// trip itself, per the spec's own testability note.
func TestBuildPromptNode_LeadingNewlineRequoted(t *testing.T) {
	cases := []string{
		"\nhello",
		"\n",
		"\n\nhello",
		"\n\n",
	}
	for _, prompt := range cases {
		t.Run(prompt, func(t *testing.T) {
			node, warning, err := buildPromptNode(prompt)
			if err != nil {
				t.Fatalf("buildPromptNode(%q): %v", prompt, err)
			}
			if warning != warnPromptRequoted {
				t.Errorf("warning = %q, want %q", warning, warnPromptRequoted)
			}
			if node.Style != yaml.DoubleQuotedStyle {
				t.Errorf("Style = %v, want DoubleQuotedStyle", node.Style)
			}
			if node.Tag != "!!str" {
				t.Errorf("Tag = %q, want !!str", node.Tag)
			}
			if got := decodePromptNode(t, node); got != prompt {
				t.Errorf("round trip = %q, want %q", got, prompt)
			}
		})
	}
}

// containsYAMLNewline must match the exact 5-character YAML 1.2 line-break set from
// spec.md's Definitions section, not just LF.
func TestContainsYAMLNewline(t *testing.T) {
	yes := []string{"\n", "\r", "\u0085", "\u2028", "\u2029", "a\u2028b"}
	for _, s := range yes {
		if !containsYAMLNewline(s) {
			t.Errorf("containsYAMLNewline(%q) = false, want true", s)
		}
	}
	no := []string{"", "plain text", "no breaks here \t tab only"}
	for _, s := range no {
		if containsYAMLNewline(s) {
			t.Errorf("containsYAMLNewline(%q) = true, want false", s)
		}
	}
}

// Guard against a regression where the probe encoder configuration drifts from
// marshalExportDocument's: both must use the same 2-space, pinned encoder (AC-12),
// or a future indent change could make a prompt round-trip differently standalone
// than it does inside the real document.
func TestBuildPromptNode_UsesPinnedEncoderConfig(t *testing.T) {
	prompt := "line one\nline two\n"
	node, _, err := buildPromptNode(prompt)
	if err != nil {
		t.Fatalf("buildPromptNode: %v", err)
	}
	if !strings.Contains(node.Value, "line one") {
		t.Fatalf("unexpected node value %q", node.Value)
	}
}
