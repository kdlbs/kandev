package routingerr

import "testing"

// realThinkingBlocks400 is the exact Anthropic 400 surfaced by the
// claude-agent-acp adapter after a session/load resume, when the
// reconstructed conversation history loses the extended-thinking block
// signatures. The persisted thinking state is poisoned, so resuming is
// futile — only a fresh session recovers.
const realThinkingBlocks400 = `{"code":-32603,"message":"Internal error: API Error: 400 messages.0.content.1: ` +
	"`thinking`" + ` or ` + "`redacted_thinking`" + ` blocks in the latest assistant message cannot be modified. ` +
	`These blocks must remain as they were in the original response.","data":{"errorKind":"unknown"}}`

func TestMatchRuntimeEnvironmentRules_ResumeCorrupted(t *testing.T) {
	got, ok := matchRuntimeEnvironmentRules(realThinkingBlocks400)
	if !ok {
		t.Fatalf("expected match, got none")
	}
	if got.Code != CodeResumeCorrupted {
		t.Fatalf("Code = %q, want %q", got.Code, CodeResumeCorrupted)
	}
	if got.ClassifierRule != "anthropic.thinking_blocks.immutable.v1" {
		t.Fatalf("ClassifierRule = %q, want anthropic.thinking_blocks.immutable.v1", got.ClassifierRule)
	}
	if got.Confidence != ConfHigh {
		t.Fatalf("Confidence = %q, want %q", got.Confidence, ConfHigh)
	}
	if got.RemediationPath != "start_fresh_session" {
		t.Fatalf("RemediationPath = %q, want start_fresh_session", got.RemediationPath)
	}
}

func TestMatchRuntimeEnvironmentRules_RedactedThinkingVariant(t *testing.T) {
	// The signature must also fire when only redacted_thinking is named.
	msg := "API Error: 400 redacted_thinking blocks in the latest assistant message cannot be modified."
	got, ok := matchRuntimeEnvironmentRules(msg)
	if !ok || got.Code != CodeResumeCorrupted {
		t.Fatalf("expected resume_corrupted match, got ok=%v err=%v", ok, got)
	}
}

func TestClassify_ResumeCorrupted_Invariants(t *testing.T) {
	resetInjection()
	e := Classify(Input{
		Phase:      PhasePromptSend,
		ProviderID: "claude-acp",
		Stderr:     realThinkingBlocks400,
	})
	if e == nil {
		t.Fatal("expected non-nil Error")
	}
	if e.Code != CodeResumeCorrupted {
		t.Fatalf("Code = %q, want %q", e.Code, CodeResumeCorrupted)
	}
	if !e.UserAction {
		t.Errorf("UserAction = false, want true")
	}
	if e.AutoRetryable {
		t.Errorf("AutoRetryable = true, want false (resuming is futile)")
	}
	if e.FallbackAllowed {
		t.Errorf("FallbackAllowed = true, want false (session-state corruption, not a provider problem)")
	}
	if e.RemediationPath != "start_fresh_session" {
		t.Errorf("RemediationPath = %q, want start_fresh_session", e.RemediationPath)
	}
	if e.Phase != PhasePromptSend {
		t.Errorf("Phase = %q, want %q", e.Phase, PhasePromptSend)
	}
}

func TestIsResumeCorrupted(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		want bool
	}{
		{"real 400 envelope", realThinkingBlocks400, true},
		{"redacted variant", "redacted_thinking blocks ... cannot be modified", true},
		{"unrelated error", "API Error: 401 authentication_error", false},
		{"empty", "", false},
		{"thinking without immutability phrase", "the agent is thinking about blocks", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsResumeCorrupted(tc.msg); got != tc.want {
				t.Errorf("IsResumeCorrupted(%q) = %v, want %v", tc.msg, got, tc.want)
			}
		})
	}
}
