package routingerr

import (
	"net/http"
	"testing"
	"time"
)

const (
	claudeFiveHourRule = "claude.stderr.five_hour_limit.v1"
	claudeUsageRule    = "claude.stderr.usage_limit.v1"
	codexCreditsRule   = "codex.stderr.out_of_credits.v1"
)

func assertProviderQuotaNotice(t *testing.T, provider string, phase Phase, notice, rule string) Error {
	t.Helper()
	e := Classify(Input{ProviderID: provider, Phase: phase, Stderr: notice})
	if e.Code != CodeQuotaLimited || e.Confidence != ConfHigh || e.Class != ClassHard {
		t.Fatalf("classification = %+v, want high-confidence hard quota_limited", e)
	}
	if !e.FallbackAllowed || !e.AutoRetryable || e.UserAction {
		t.Fatalf("flags = fallback:%t retry:%t user:%t, want fallback and retry without user action",
			e.FallbackAllowed, e.AutoRetryable, e.UserAction)
	}
	if e.ClassifierRule != rule {
		t.Fatalf("classifier rule = %q, want %q", e.ClassifierRule, rule)
	}
	return *e
}

func TestClassifyClaudeFiveHourLimit(t *testing.T) {
	resetInjection()
	notices := []string{
		"Internal error: You've hit your 5-hour limit · resets 12:50pm (Europe/Berlin)",
		"Error: You’ve hit your five-hour limit",
		"API Error: YOU HAVE HIT YOUR 5 HOUR LIMIT",
		"  Internal\t error:   you've   hit\tyour 5-hour limit",
	}
	for _, phase := range []Phase{PhasePromptSend, PhaseStreaming} {
		for _, notice := range notices {
			t.Run(string(phase)+"/"+notice, func(t *testing.T) {
				assertProviderQuotaNotice(t, "claude-acp", phase, notice, claudeFiveHourRule)
			})
		}
	}
}

func TestClassifyClaudeFiveHourLimitInfersZonedReset(t *testing.T) {
	resetInjection()
	observedAt := time.Date(2026, time.November, 9, 9, 10, 0, 0, time.UTC)
	e := Classify(Input{
		ProviderID: "claude-acp", Phase: PhasePromptSend,
		Stderr:     "Internal error: You've hit your 5-hour limit · resets 12:50pm (Europe/Berlin)",
		OccurredAt: observedAt,
	})
	want := time.Date(2026, time.November, 9, 11, 50, 0, 0, time.UTC)
	if e.Code != CodeQuotaLimited || e.ResetHint == nil || !e.ResetHint.Equal(want) || !e.ResetHintInferred {
		t.Fatalf("classification = %+v, want inferred reset %s", e, want)
	}

	invalidZone := Classify(Input{
		ProviderID: "claude-acp", Phase: PhasePromptSend,
		Stderr:     "You've hit your 5-hour limit · resets 12:50pm (Not/AZone)",
		OccurredAt: observedAt,
	})
	if invalidZone.Code != CodeQuotaLimited || invalidZone.ResetHint != nil || invalidZone.ResetHintInferred {
		t.Fatalf("invalid zone classification = %+v, want quota without a hint", invalidZone)
	}

	structured := observedAt.Add(6 * time.Hour)
	precedence := Classify(Input{
		ProviderID: "claude-acp", Phase: PhasePromptSend,
		Stderr:     "You've hit your 5-hour limit · resets 12:50pm (Europe/Berlin)",
		OccurredAt: observedAt, ResetHint: &structured,
	})
	if precedence.ResetHint == nil || !precedence.ResetHint.Equal(structured) || precedence.ResetHintInferred {
		t.Fatalf("structured hint classification = %+v, want supplied hint %s", precedence, structured)
	}

	hourOnly := Classify(Input{
		ProviderID: "claude-acp", Phase: PhasePromptSend,
		Stderr:     "You've hit your 5-hour limit · resets 1pm (UTC)",
		OccurredAt: observedAt,
	})
	if hourOnly.ResetHint == nil || !hourOnly.ResetHint.Equal(time.Date(2026, time.November, 9, 13, 0, 0, 0, time.UTC)) {
		t.Fatalf("hour-only classification = %+v, want 13:00Z reset", hourOnly)
	}
}

func TestClassifyClaudeFiveHourLimitBindsResetToDiagnosticLine(t *testing.T) {
	resetInjection()
	observedAt := time.Date(2026, time.November, 9, 9, 10, 0, 0, time.UTC)
	wantClock := time.Date(2026, time.November, 9, 11, 50, 0, 0, time.UTC)
	notice := "Internal error: You've hit your 5-hour limit · resets 12:50pm (Europe/Berlin)"
	for _, tc := range []struct {
		name     string
		stderr   string
		stdout   string
		want     *time.Time
		inferred bool
	}{
		{
			name:   "fenced clock before notice",
			stderr: "```text\nExample: resets 11pm (UTC)\n```\n" + notice,
			want:   &wantClock, inferred: true,
		},
		{
			name:   "fenced dated timestamp before notice",
			stderr: "```\ntry again at Nov 9th, 2026 11:00 PM UTC\n```\n" + notice,
			want:   &wantClock, inferred: true,
		},
		{
			name:   "prose clock before notice",
			stderr: "The child reported resets 11pm (UTC)\n" + notice,
			want:   &wantClock, inferred: true,
		},
		{
			name:   "other channel clock before notice",
			stderr: "Another job: resets 11pm (UTC)",
			stdout: notice,
			want:   &wantClock, inferred: true,
		},
		{
			name:   "notice without reset beside fenced clock",
			stderr: "```\nresets 11pm (UTC)\n```\nInternal error: You've hit your 5-hour limit",
		},
		{
			name: "notice without reset beside quoted dated timestamp",
			stderr: "The child said \"try again at Nov 9th, 2026 11:00 PM UTC\"\n" +
				"Internal error: You've hit your 5-hour limit",
		},
		{
			name: "dated timestamp on the matching line",
			stderr: "Internal error: You've hit your 5-hour limit. " +
				"try again at Nov 9th, 2026 11:50 AM UTC",
			want: &wantClock,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := Classify(Input{
				ProviderID: "claude-acp", Phase: PhasePromptSend,
				Stderr: tc.stderr, Stdout: tc.stdout, OccurredAt: observedAt,
			})
			if e.Code != CodeQuotaLimited || e.ClassifierRule != claudeFiveHourRule {
				t.Fatalf("classification = %s/%s, want %s", e.Code, e.ClassifierRule, claudeFiveHourRule)
			}
			if tc.want == nil {
				if e.ResetHint != nil || e.ResetHintInferred {
					t.Fatalf("reset hint = %v inferred=%t, want none", e.ResetHint, e.ResetHintInferred)
				}
				return
			}
			if e.ResetHint == nil || !e.ResetHint.Equal(*tc.want) || e.ResetHintInferred != tc.inferred {
				t.Fatalf("reset hint = %v inferred=%t, want %s inferred=%t",
					e.ResetHint, e.ResetHintInferred, *tc.want, tc.inferred)
			}
		})
	}
}

func TestClassifyClaudeUsageLimit(t *testing.T) {
	resetInjection()
	notices := []string{
		"Internal error: You've hit your usage limit",
		"You’ve hit your usage limit",
		"API Error: YOU HAVE HIT YOUR usage limit",
		"Error:\tYou have\thit your\tusage   limit",
	}
	for _, phase := range []Phase{PhasePromptSend, PhaseStreaming} {
		for _, notice := range notices {
			t.Run(string(phase)+"/"+notice, func(t *testing.T) {
				e := assertProviderQuotaNotice(t, "claude-acp", phase, notice, claudeUsageRule)
				if e.ResetHint != nil || e.ResetHintInferred {
					t.Fatalf("usage notice invented a reset: %+v", e)
				}
			})
		}
	}

	observedAt := time.Date(2026, time.November, 9, 9, 10, 0, 0, time.UTC)
	withClock := Classify(Input{
		ProviderID: "claude-acp", Phase: PhasePromptSend,
		Stderr:     "You've hit your usage limit · resets 12:50pm (Europe/Berlin)",
		OccurredAt: observedAt,
	})
	if withClock.Code != CodeQuotaLimited || withClock.ResetHint != nil || withClock.ResetHintInferred {
		t.Fatalf("usage notice clock classification = %+v, want quota without a guessed deadline", withClock)
	}
	structured := observedAt.Add(30 * time.Minute)
	withHint := Classify(Input{
		ProviderID: "claude-acp", Phase: PhasePromptSend,
		Stderr: "You've hit your usage limit", OccurredAt: observedAt, ResetHint: &structured,
	})
	if withHint.ResetHint == nil || !withHint.ResetHint.Equal(structured) {
		t.Fatalf("structured usage hint = %+v, want %s", withHint, structured)
	}
}

func TestClassifyCodexWorkspaceCredits(t *testing.T) {
	resetInjection()
	notices := []string{
		"Your workspace is out of credits. Add credits to continue.",
		"workspace is out of credits",
		"Internal error: YOUR WORKSPACE IS OUT OF CREDITS",
		"API Error:\tyour\tworkspace   is out of\tcredits",
	}
	for _, phase := range []Phase{PhasePromptSend, PhaseStreaming} {
		for _, notice := range notices {
			t.Run(string(phase)+"/"+notice, func(t *testing.T) {
				e := assertProviderQuotaNotice(t, "codex-acp", phase, notice, codexCreditsRule)
				if e.ResetHint != nil || e.ResetHintInferred {
					t.Fatalf("credit notice invented a reset: %+v", e)
				}
			})
		}
	}

	withClock := Classify(Input{
		ProviderID: "codex-acp", Phase: PhasePromptSend,
		Stderr: "Your workspace is out of credits. Add credits to continue.\n" +
			"resets 12:50pm (Europe/Berlin)",
		OccurredAt: time.Date(2026, time.November, 9, 9, 10, 0, 0, time.UTC),
	})
	if withClock.Code != CodeQuotaLimited || withClock.ResetHint != nil {
		t.Fatalf("credit clock classification = %+v, want quota without a deadline", withClock)
	}
}

func TestProviderQuotaNoticeCollisionsAndPrecedence(t *testing.T) {
	resetInjection()
	positives := []struct {
		name, provider, notice, rule string
	}{
		{"claude five-hour", "claude-acp", "Internal error: You've hit your 5-hour limit", claudeFiveHourRule},
		{"claude usage", "claude-acp", "You've hit your usage limit", claudeUsageRule},
		{"codex credits", "codex-acp", "Your workspace is out of credits. Add credits to continue.", codexCreditsRule},
	}
	for _, tc := range positives {
		t.Run(tc.name+" outranks HTTP 429", func(t *testing.T) {
			e := Classify(Input{
				ProviderID: tc.provider, Phase: PhaseStreaming,
				Stderr: tc.notice, HTTPStatus: http.StatusTooManyRequests,
			})
			if e.Code != CodeQuotaLimited || e.Class != ClassHard || e.ClassifierRule != tc.rule {
				t.Fatalf("classification = %+v, want hard quota via %s", e, tc.rule)
			}
		})
		t.Run(tc.name+" preserves other structured statuses", func(t *testing.T) {
			for _, tcStatus := range []struct {
				status int
				code   Code
			}{
				{http.StatusUnauthorized, CodeAuthRequired},
				{http.StatusPaymentRequired, CodeSubscriptionRequired},
				{http.StatusServiceUnavailable, CodeProviderUnavailable},
			} {
				e := Classify(Input{
					ProviderID: tc.provider, Phase: PhaseStreaming,
					Stderr: tc.notice, HTTPStatus: tcStatus.status,
				})
				if e.Code != tcStatus.code {
					t.Fatalf("status %d classification = %s/%s, want %s",
						tcStatus.status, e.Code, e.ClassifierRule, tcStatus.code)
				}
			}
		})
	}

	negatives := []struct {
		name, provider, text string
	}{
		{"claude five-hour mid-sentence", "claude-acp", "The child said: You've hit your 5-hour limit"},
		{"claude five-hour fenced", "claude-acp", "```text\nError: You've hit your 5-hour limit\n```"},
		{"claude five-hour split line", "claude-acp", "You've hit your\n5-hour limit"},
		{"claude five-hour incomplete", "claude-acp", "You've hit your 5-hour"},
		{"claude five-hour other provider", "codex-acp", "You've hit your 5-hour limit"},
		{"claude usage mid-sentence", "claude-acp", "The child reported You've hit your usage limit"},
		{"claude usage fenced", "claude-acp", "```\nYou've hit your usage limit\n```"},
		{"claude usage split line", "claude-acp", "You've hit your\nusage limit"},
		{"claude usage incomplete", "claude-acp", "You've hit your usage"},
		{"claude usage other provider", "codex-acp", "You've hit your usage limit with Claude"},
		{"codex credits mid-sentence", "codex-acp", "The child said Your workspace is out of credits"},
		{"codex credits fenced", "codex-acp", "```\nYour workspace is out of credits\n```"},
		{"codex credits split line", "codex-acp", "Your workspace is\nout of credits"},
		{"codex credits incomplete", "codex-acp", "Your workspace is out of"},
		{"codex credits other provider", "claude-acp", "Your workspace is out of credits"},
	}
	for _, tc := range negatives {
		t.Run(tc.name, func(t *testing.T) {
			e := Classify(Input{ProviderID: tc.provider, Phase: PhasePromptSend, Stderr: tc.text})
			if e.Code == CodeQuotaLimited && e.ClassifierRule != "claude.stderr.quota.v1" &&
				e.ClassifierRule != "codex.stderr.quota.v1" {
				t.Fatalf("%q classified as quota via %s", tc.text, e.ClassifierRule)
			}
		})
	}

	fencedThenReal := Classify(Input{
		ProviderID: "claude-acp", Phase: PhasePromptSend,
		Stderr: "```\nError: You've hit your 5-hour limit\n```\n" +
			"Internal error: You've hit your usage limit",
	})
	if fencedThenReal.Code != CodeQuotaLimited || fencedThenReal.ClassifierRule != claudeUsageRule {
		t.Fatalf("diagnostic after closed fence = %s/%s, want %s",
			fencedThenReal.Code, fencedThenReal.ClassifierRule, claudeUsageRule)
	}

	rate := Classify(Input{ProviderID: "claude-acp", Phase: PhasePromptSend, Stderr: "You've hit your rate limit"})
	if rate.Code != CodeRateLimited || rate.ClassifierRule != "claude.stderr.rate.v1" {
		t.Fatalf("rate notice = %s/%s, want the existing rate rule", rate.Code, rate.ClassifierRule)
	}

	dated := time.Date(2026, time.November, 9, 11, 50, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, provider, text, rule string
		want                       *time.Time
	}{
		{
			name: "usage ignores unrelated fenced reset", provider: "claude-acp",
			text: "```\nresets 11pm (UTC)\n```\nYou've hit your usage limit",
			rule: claudeUsageRule,
		},
		{
			name: "usage keeps same-line dated reset", provider: "claude-acp",
			text: "You've hit your usage limit. try again at Nov 9th, 2026 11:50 AM UTC",
			rule: claudeUsageRule, want: &dated,
		},
		{
			name: "credits ignore unrelated prose clock", provider: "codex-acp",
			text: "The child said resets 11pm (UTC)\nYour workspace is out of credits. Add credits to continue.",
			rule: codexCreditsRule,
		},
		{
			name: "credits keep same-line dated reset", provider: "codex-acp",
			text: "Your workspace is out of credits. try again at Nov 9th, 2026 11:50 AM UTC",
			rule: codexCreditsRule, want: &dated,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := Classify(Input{
				ProviderID: tc.provider, Phase: PhasePromptSend, Stderr: tc.text,
				OccurredAt: time.Date(2026, time.November, 9, 9, 10, 0, 0, time.UTC),
			})
			if e.Code != CodeQuotaLimited || e.ClassifierRule != tc.rule {
				t.Fatalf("classification = %s/%s, want %s", e.Code, e.ClassifierRule, tc.rule)
			}
			if tc.want == nil {
				if e.ResetHint != nil {
					t.Fatalf("reset hint = %v, want none", e.ResetHint)
				}
				return
			}
			if e.ResetHint == nil || !e.ResetHint.Equal(*tc.want) {
				t.Fatalf("reset hint = %v, want %s", e.ResetHint, *tc.want)
			}
		})
	}

	for _, rule := range []string{claudeFiveHourRule, claudeUsageRule, codexCreditsRule} {
		if !hardQuotaRuleOutranksRateStatus(rule) {
			t.Errorf("hardQuotaRuleOutranksRateStatus(%q) = false, want true", rule)
		}
	}
}
