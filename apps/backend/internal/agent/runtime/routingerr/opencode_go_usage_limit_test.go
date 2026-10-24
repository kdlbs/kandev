package routingerr

import (
	"testing"
	"time"
)

func TestClassify_OpenCodeGoUsageLimitExceededIsHighConfidenceQuota(t *testing.T) {
	resetInjection()
	for _, notice := range []string{
		"AI_APICallError: Go usage limit exceeded",
		"Go usage limit exceeded",
		"Error: GO USAGE LIMIT EXCEEDED",
	} {
		t.Run(notice, func(t *testing.T) {
			e := Classify(Input{Phase: PhaseStreaming, ProviderID: "opencode-acp", Stderr: notice})
			if e.Code != CodeQuotaLimited || e.Confidence != ConfHigh || e.Class != ClassHard {
				t.Fatalf("classification = %+v, want high-confidence hard quota_limited", e)
			}
			if e.ClassifierRule != "opencode.acp.go_usage_limit.v1" {
				t.Fatalf("classifier rule = %s, want opencode.acp.go_usage_limit.v1", e.ClassifierRule)
			}
			if !e.FallbackAllowed || !e.AutoRetryable || e.UserAction {
				t.Fatalf("Go usage limit recovery flags violated: %+v", e)
			}
			if e.ResetHint != nil {
				t.Fatalf("ResetHint = %v, want none for a notice without a reset time", e.ResetHint)
			}
		})
	}
}

// A correlated period notice carries the reset time, so its own rule wins and
// keeps the reset hint.
func TestClassify_OpenCodeGoUsageLimitKeepsPeriodNoticeResetHint(t *testing.T) {
	resetInjection()
	resetAt := time.Date(2026, 10, 3, 15, 19, 0, 0, time.UTC)
	e := Classify(Input{
		Phase:      PhaseStreaming,
		ProviderID: "opencode-acp",
		ResetHint:  &resetAt,
		Stderr:     "AI_APICallError: Go usage limit exceeded\nAI_APICallError: 5-hour usage limit reached. Resets in 4hr 19min.",
	})
	if e.Code != CodeQuotaLimited || e.ClassifierRule != "opencode.stderr.usage_limit.v1" {
		t.Fatalf("classification = %+v, want the period usage-limit rule", e)
	}
	if e.ResetHint == nil || !e.ResetHint.Equal(resetAt) {
		t.Fatalf("ResetHint = %v, want %v", e.ResetHint, resetAt)
	}
}

func TestClassify_OpenCodeGoUsageLimitRejectsUnrelatedText(t *testing.T) {
	resetInjection()
	for _, stderr := range []string{
		"The agent explained: Go usage limit exceeded on the free plan",
		"Go usage limit",
	} {
		if e := Classify(Input{Phase: PhaseStreaming, ProviderID: "opencode-acp", Stderr: stderr}); e.ClassifierRule == "opencode.acp.go_usage_limit.v1" {
			t.Fatalf("%q acquired the Go usage-limit rule: %+v", stderr, e)
		}
	}
	e := Classify(Input{Phase: PhaseStreaming, ProviderID: "codex-acp", Stderr: "AI_APICallError: Go usage limit exceeded"})
	if e.ClassifierRule == "opencode.acp.go_usage_limit.v1" {
		t.Fatalf("other providers must not use the OpenCode Go rule: %+v", e)
	}
}
