package routingerr

import (
	"net/http"
	"testing"
	"time"
)

func TestClassifyDevinFreeModelRateLimit(t *testing.T) {
	resetInjection()
	observedAt := time.Date(2026, time.November, 18, 11, 53, 0, 0, time.UTC)
	wantReset := time.Date(2026, time.November, 18, 12, 1, 0, 0, time.UTC)
	cases := []struct {
		name   string
		notice string
	}{
		{
			"minutes",
			"Reached free model rate limit. Upgrade to Max for higher limits, or switch to a different model. Your limit will reset in 8 minutes (at 12:01 UTC)",
		},
		{
			"hour and minutes",
			"Reached free model rate limit. Upgrade to Max for higher limits, or switch to a different model. Your limit will reset in 1 hour 12 minutes (at 12:01 UTC)",
		},
		{
			"short minutes",
			"Reached free model rate limit. Upgrade to Max for higher limits, or switch to a different model. Your limit will reset in 5 minutes (at 12:01 UTC)",
		},
		{
			"error label",
			"Error: Reached free model rate limit. Your limit will reset in 8 minutes (at 12:01 UTC)",
		},
	}
	for _, tc := range cases {
		for _, phase := range []Phase{PhasePromptSend, PhaseStreaming} {
			t.Run(tc.name+"/"+string(phase), func(t *testing.T) {
				got := Classify(Input{
					Phase: phase, ProviderID: "devin-acp", Stderr: tc.notice, OccurredAt: observedAt,
				})
				if got.Code != CodeRateLimited || got.Confidence != ConfHigh || got.Class != ClassTransient {
					t.Fatalf("classification = %+v, want high-confidence transient rate limit", got)
				}
				if got.ClassifierRule != ruleDevinFreeModelRateLimit {
					t.Fatalf("classifier rule = %q, want %q", got.ClassifierRule, ruleDevinFreeModelRateLimit)
				}
				if !got.FallbackAllowed || !got.AutoRetryable || got.UserAction {
					t.Fatalf("rate recovery flags violated: %+v", got)
				}
				if got.ResetHint == nil || !got.ResetHint.Equal(wantReset) || !got.ResetHintInferred {
					t.Fatalf("reset hint = %v inferred=%t, want %s inferred", got.ResetHint, got.ResetHintInferred, wantReset)
				}
			})
		}
	}
}

// A clock-only notice names no date. Without a relative duration to anchor it,
// once its clock has elapsed the reset belongs to the next calendar day.
func TestClassifyDevinFreeModelRateLimitRollsClockToNextDay(t *testing.T) {
	resetInjection()
	got := Classify(Input{
		ProviderID: "devin-acp", Phase: PhasePromptSend,
		Stderr: "Reached free model rate limit. Your limit will reset (at 12:01 UTC)",
		// 23:30 on the prior day would also roll; use a time after the clock
		// on the same day to prove the next-day rule.
		OccurredAt: time.Date(2026, time.November, 18, 12, 30, 0, 0, time.UTC),
	})
	want := time.Date(2026, time.November, 19, 12, 1, 0, 0, time.UTC)
	if got.ResetHint == nil || !got.ResetHint.Equal(want) {
		t.Fatalf("reset hint = %v, want %s", got.ResetHint, want)
	}
}

// The relative duration and the clock must agree on the day. A clock that has
// just elapsed by a few seconds (provider truncation, latency, or a slightly
// fast local clock) must not roll the hint a whole day forward.
func TestClassifyDevinFreeModelRateLimitUsesRelativeDuration(t *testing.T) {
	resetInjection()
	cases := []struct {
		name       string
		observedAt time.Time
		notice     string
		want       time.Time
	}{
		{
			name:       "clock elapsed by seconds",
			observedAt: time.Date(2026, time.November, 18, 12, 1, 5, 0, time.UTC),
			notice:     "Reached free model rate limit. Your limit will reset in 1 minute (at 12:01 UTC)",
			want:       time.Date(2026, time.November, 18, 12, 1, 0, 0, time.UTC),
		},
		{
			name:       "zero minutes",
			observedAt: time.Date(2026, time.November, 18, 12, 1, 5, 0, time.UTC),
			notice:     "Reached free model rate limit. Your limit will reset in 0 minutes (at 12:01 UTC)",
			want:       time.Date(2026, time.November, 18, 12, 1, 0, 0, time.UTC),
		},
		{
			name:       "duration crosses midnight",
			observedAt: time.Date(2026, time.November, 18, 23, 0, 0, 0, time.UTC),
			notice:     "Reached free model rate limit. Your limit will reset in 1 hour 30 minutes (at 00:30 UTC)",
			want:       time.Date(2026, time.November, 19, 0, 30, 0, 0, time.UTC),
		},
		{
			name:       "clock elapsed just after midnight",
			observedAt: time.Date(2026, time.November, 19, 0, 0, 5, 0, time.UTC),
			notice:     "Reached free model rate limit. Your limit will reset in 0 minutes (at 23:59 UTC)",
			want:       time.Date(2026, time.November, 18, 23, 59, 0, 0, time.UTC),
		},
		{
			name:       "duration extends beyond next calendar day",
			observedAt: time.Date(2026, time.November, 18, 23, 55, 0, 0, time.UTC),
			notice:     "Reached free model rate limit. Your limit will reset in 24 hours 10 minutes (at 00:05 UTC)",
			want:       time.Date(2026, time.November, 20, 0, 5, 0, 0, time.UTC),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(Input{
				ProviderID: "devin-acp", Phase: PhasePromptSend,
				Stderr: tc.notice, OccurredAt: tc.observedAt,
			})
			if got.ResetHint == nil || !got.ResetHint.Equal(tc.want) {
				t.Fatalf("reset hint = %v, want %s", got.ResetHint, tc.want)
			}
		})
	}
}

func TestClassifyDevinFreeModelRateLimitUsesMatchedLineForStructuredHTTP429(t *testing.T) {
	resetInjection()
	observedAt := time.Date(2026, time.November, 18, 11, 53, 0, 0, time.UTC)
	want := time.Date(2026, time.November, 18, 12, 1, 0, 0, time.UTC)
	notice := "Reached free model rate limit. Your limit will reset in 8 minutes (at 12:01 UTC)"

	t.Run("infers from Devin line and preserves structured classification", func(t *testing.T) {
		got := Classify(Input{
			HTTPStatus: http.StatusTooManyRequests, ProviderID: providerDevinACP,
			Phase: PhasePromptSend, OccurredAt: observedAt,
			Stderr: "Another job: reset in 2 minutes (at 11:55 UTC)\n" + notice,
		})
		if got.Code != CodeRateLimited || got.ClassifierRule != ruleHTTP429 {
			t.Fatalf("classification = %s/%s, want structured HTTP rate limit", got.Code, got.ClassifierRule)
		}
		if got.ResetHint == nil || !got.ResetHint.Equal(want) || !got.ResetHintInferred {
			t.Fatalf("reset hint = %v inferred=%t, want %s inferred from Devin notice", got.ResetHint, got.ResetHintInferred, want)
		}
	})

	t.Run("explicit reset takes precedence", func(t *testing.T) {
		explicit := observedAt.Add(2 * time.Hour)
		got := Classify(Input{
			HTTPStatus: http.StatusTooManyRequests, ProviderID: providerDevinACP,
			Phase: PhasePromptSend, OccurredAt: observedAt, ResetHint: &explicit,
			Stderr: notice,
		})
		if got.Code != CodeRateLimited || got.ClassifierRule != ruleHTTP429 {
			t.Fatalf("classification = %s/%s, want structured HTTP rate limit", got.Code, got.ClassifierRule)
		}
		if got.ResetHint == nil || !got.ResetHint.Equal(explicit) || got.ResetHintInferred {
			t.Fatalf("reset hint = %v inferred=%t, want explicit %s", got.ResetHint, got.ResetHintInferred, explicit)
		}
	})

	t.Run("unmatched text cannot supply a reset", func(t *testing.T) {
		got := Classify(Input{
			HTTPStatus: http.StatusTooManyRequests, ProviderID: providerDevinACP,
			Phase: PhasePromptSend, OccurredAt: observedAt,
			Stderr: "Another job: reset in 2 minutes (at 11:55 UTC)",
		})
		if got.Code != CodeRateLimited || got.ClassifierRule != ruleHTTP429 || got.ResetHint != nil {
			t.Fatalf("classification = %s/%s reset=%v, want structured rate limit without inferred reset", got.Code, got.ClassifierRule, got.ResetHint)
		}
	})
}

func TestClassifyDevinFreeModelRateLimitRejectsUnrelatedText(t *testing.T) {
	resetInjection()
	cases := []struct {
		name, provider, notice string
	}{
		{"assistant prose", "devin-acp", "The child reported: Reached free model rate limit."},
		{"mid-sentence", "devin-acp", "It stopped because it reached free model rate limit."},
		{"fenced example", "devin-acp", "```text\nReached free model rate limit. Your limit will reset in 8 minutes (at 12:01 UTC)\n```"},
		{"incomplete notice", "devin-acp", "Reached free model"},
		{"missing word boundary", "devin-acp", "Reached free model rate limitless"},
		{"other provider", "claude-acp", "Reached free model rate limit. Your limit will reset in 8 minutes (at 12:01 UTC)"},
		{"opencode provider", "opencode-acp", "Reached free model rate limit. Your limit will reset in 8 minutes (at 12:01 UTC)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(Input{Phase: PhasePromptSend, ProviderID: tc.provider, Stderr: tc.notice})
			if got.Code != CodeAgentRuntime || got.ClassifierRule != "phase.poststart.unknown" {
				t.Fatalf("classification = %s/%s, want unmatched runtime error", got.Code, got.ClassifierRule)
			}
			if got.ResetHint != nil {
				t.Fatalf("reset hint = %v, want none", got.ResetHint)
			}
		})
	}
}

// A diagnostic after a closed fence still classifies, and an unrelated clock
// on another line must not become its reset.
func TestClassifyDevinFreeModelRateLimitBindsResetToDiagnosticLine(t *testing.T) {
	resetInjection()
	observedAt := time.Date(2026, time.November, 18, 11, 53, 0, 0, time.UTC)
	want := time.Date(2026, time.November, 18, 12, 1, 0, 0, time.UTC)

	afterFence := Classify(Input{
		ProviderID: "devin-acp", Phase: PhasePromptSend, OccurredAt: observedAt,
		Stderr: "```\nReached free model rate limit\n```\n" +
			"Reached free model rate limit. Your limit will reset in 8 minutes (at 12:01 UTC)",
	})
	if afterFence.Code != CodeRateLimited || afterFence.ClassifierRule != ruleDevinFreeModelRateLimit {
		t.Fatalf("diagnostic after closed fence = %s/%s, want devin rate limit", afterFence.Code, afterFence.ClassifierRule)
	}
	if afterFence.ResetHint == nil || !afterFence.ResetHint.Equal(want) {
		t.Fatalf("reset hint = %v, want %s", afterFence.ResetHint, want)
	}

	unrelatedClock := Classify(Input{
		ProviderID: "devin-acp", Phase: PhasePromptSend, OccurredAt: observedAt,
		Stderr: "Another job: reset in 2 minutes (at 11:55 UTC)\n" +
			"Reached free model rate limit. Your limit will reset in 8 minutes (at 12:01 UTC)",
	})
	if unrelatedClock.ResetHint == nil || !unrelatedClock.ResetHint.Equal(want) {
		t.Fatalf("reset hint = %v, want the matching line's %s", unrelatedClock.ResetHint, want)
	}

	noClock := Classify(Input{
		ProviderID: "devin-acp", Phase: PhasePromptSend, OccurredAt: observedAt,
		Stderr: "Reached free model rate limit. Your limit will reset in 8 minutes",
	})
	if noClock.Code != CodeRateLimited || noClock.ResetHint != nil {
		t.Fatalf("clockless notice = %+v, want rate limit without a reset", noClock)
	}
}
