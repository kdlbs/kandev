package routingerr

import (
	"strings"
	"testing"
)

// reasoningContentMessage is a sanitized DeepSeek missing-reasoning diagnostic
// surfaced through an ACP adapter.
const reasoningContentMessage = "Internal error: {\"message\":\"The `reasoning_content` in the thinking mode " +
	"must be passed back to the API. (request_id: ***)\",\"type\":\"invalid_request_error\",\"param\":null," +
	"\"code\":\"invalid_request_error\"}"

func TestClassify_ReasoningContentNonReplayable(t *testing.T) {
	resetInjection()
	e := Classify(Input{Phase: PhasePromptSend, ProviderID: "opencode-acp", Stderr: reasoningContentMessage})
	if e == nil {
		t.Fatal("expected non-nil Error")
	}
	if e.Code != CodeResumeCorrupted {
		t.Fatalf("Code = %q, want %q", e.Code, CodeResumeCorrupted)
	}
	if e.Confidence != ConfHigh {
		t.Fatalf("Confidence = %q, want %q", e.Confidence, ConfHigh)
	}
	if e.ClassifierRule != reasoningContentNonReplayableRuleID {
		t.Fatalf("ClassifierRule = %q, want %q", e.ClassifierRule, reasoningContentNonReplayableRuleID)
	}
	if e.Class != ClassUnclassified {
		t.Fatalf("Class = %q, want unclassified", e.Class)
	}
	if e.RemediationPath != RemediationStartFreshSession {
		t.Fatalf("RemediationPath = %q, want %q", e.RemediationPath, RemediationStartFreshSession)
	}
	if !e.UserAction {
		t.Errorf("UserAction = false, want true")
	}
	if e.AutoRetryable {
		t.Errorf("AutoRetryable = true, want false")
	}
	if e.FallbackAllowed {
		t.Errorf("FallbackAllowed = true, want false")
	}
}

func TestIsResumeCorrupted_ReasoningContent(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		want bool
	}{
		{"deepseek reasoning_content 400", reasoningContentMessage, true},
		{"bare phrasing", "The reasoning_content in the thinking mode must be passed back to the API.", true},
		{"gap at the bound", "Internal error: reasoning_content" + strings.Repeat("x", 80) + "must be passed back", true},
		{"gap beyond the bound", "Internal error: reasoning_content" + strings.Repeat("x", 81) + "must be passed back", false},
		{"phrases on separate lines", "Internal error: reasoning_content\nmust be passed back", false},
		{"agent explanation", "DeepSeek requires reasoning_content in thinking mode and it must be passed back to the API", false},
		{"quoted diagnostic", "The child reported: The reasoning_content in the thinking mode must be passed back to the API.", false},
		{"reasoning_content without the phrase", "the model emitted reasoning_content blocks inline", false},
		{"ordinary prose", "please pass the reasoning back to me", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		if got := IsResumeCorrupted(tc.msg); got != tc.want {
			t.Errorf("%s: IsResumeCorrupted = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestClassify_ReasoningContentRawBound(t *testing.T) {
	resetInjection()
	for _, tc := range []struct {
		name string
		gap  string
		want Code
	}{
		{"redactable gap at the bound", " " + strings.Repeat("x", 78) + " ", CodeResumeCorrupted},
		{"redactable gap beyond the bound", " " + strings.Repeat("x", 79) + " ", CodeAgentRuntime},
		{"phrases on separate lines", "\n", CodeAgentRuntime},
	} {
		t.Run(tc.name, func(t *testing.T) {
			msg := "Internal error: reasoning_content" + tc.gap + "must be passed back"
			got := Classify(Input{Phase: PhasePromptSend, ProviderID: "opencode-acp", Stderr: msg})
			if got.Code != tc.want {
				t.Fatalf("Code = %q, want %q (excerpt %q)", got.Code, tc.want, got.RawExcerpt)
			}
			if IsResumeCorrupted(msg) != (got.Code == CodeResumeCorrupted) {
				t.Fatal("raw recovery predicate and classifier disagree")
			}
		})
	}
}

func TestClassify_ReasoningContentAgentProse(t *testing.T) {
	resetInjection()
	for _, msg := range []string{
		"DeepSeek requires reasoning_content in thinking mode and it must be passed back to the API",
		"The child reported: The reasoning_content in the thinking mode must be passed back to the API.",
	} {
		got := Classify(Input{Phase: PhasePromptSend, ProviderID: "opencode-acp", Stderr: msg})
		if got.Code != CodeAgentRuntime || IsResumeCorrupted(msg) {
			t.Errorf("agent prose classified as resume corruption: %q (code %q)", msg, got.Code)
		}
	}
}
