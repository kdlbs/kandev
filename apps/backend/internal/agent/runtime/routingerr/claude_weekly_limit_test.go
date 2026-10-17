package routingerr

import (
	"net/http"
	"testing"
	"time"
)

func TestClassifyClaudeWeeklyLimit(t *testing.T) {
	resetInjection()
	for _, notice := range []string{
		"Internal error: You've hit your weekly limit · resets 4am (UTC)",
		"You’ve hit your weekly limit",
		"You have hit your weekly limit",
		"YOU HAVE\nHIT YOUR\tWEEKLY LIMIT",
	} {
		for _, phase := range []Phase{PhasePromptSend, PhaseStreaming} {
			t.Run(string(phase)+"/"+notice, func(t *testing.T) {
				e := Classify(Input{ProviderID: "claude-acp", Phase: phase, Stderr: notice})
				if e.Code != CodeQuotaLimited || e.Confidence != ConfHigh || e.Class != ClassHard || !e.FallbackAllowed || !e.AutoRetryable || e.UserAction {
					t.Fatalf("classification = %+v, want high-confidence hard quota_limited with fallback", e)
				}
				if e.ClassifierRule != "claude.stderr.weekly_limit.v1" {
					t.Fatalf("classifier rule = %q, want claude.stderr.weekly_limit.v1", e.ClassifierRule)
				}
			})
		}
	}
}

// A weekly notice names a clock time but no date. Guessing the date from the
// clock would put the reset up to a week early, so only a structured hint counts.
func TestClassifyClaudeWeeklyLimitDoesNotInventResetDate(t *testing.T) {
	resetInjection()
	in := Input{
		ProviderID: "claude-acp", Phase: PhasePromptSend,
		Stderr:     "Internal error: You've hit your weekly limit · resets 4am (UTC)",
		OccurredAt: time.Date(2026, time.October, 8, 21, 42, 19, 0, time.UTC),
	}
	if e := Classify(in); e.Code != CodeQuotaLimited || e.ResetHint != nil {
		t.Fatalf("classification = %+v, want quota without a guessed reset time", e)
	}
	structured := time.Date(2026, time.October, 9, 19, 0, 0, 0, time.UTC)
	in.ResetHint = &structured
	if e := Classify(in); e.ResetHint == nil || !e.ResetHint.Equal(structured) {
		t.Fatalf("structured reset = %v, want %v", e.ResetHint, structured)
	}
}

func TestClassifyClaudeWeeklyLimitRejectsUnrelatedText(t *testing.T) {
	resetInjection()
	for _, notice := range []string{
		"weekly limit",
		"You are approaching your weekly limit",
		"You've hit your weekly limitless plan",
		"The agent reported: You've hit your weekly limit",
	} {
		if e := Classify(Input{ProviderID: "claude-acp", Phase: PhasePromptSend, Stderr: notice}); e.Code == CodeQuotaLimited {
			t.Fatalf("unrelated notice %q classified as quota: %+v", notice, e)
		}
	}
	if e := Classify(Input{ProviderID: "codex-acp", Phase: PhasePromptSend, Stderr: "You've hit your weekly limit"}); e.Code == CodeQuotaLimited {
		t.Fatalf("other provider acquired the Claude weekly-limit rule: %+v", e)
	}
	e := Classify(Input{
		ProviderID: "claude-acp", Phase: PhasePromptSend,
		Stderr: "You've hit your weekly limit", HTTPStatus: http.StatusTooManyRequests,
	})
	if e.Code != CodeRateLimited {
		t.Fatalf("structured HTTP status lost precedence: %+v", e)
	}
}
