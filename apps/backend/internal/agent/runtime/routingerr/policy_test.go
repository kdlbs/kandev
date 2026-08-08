package routingerr

import (
	"testing"
	"time"
)

func TestDecideSeparatesKanbanHardLimitsFromShortTransients(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		err  *Error
		want RecoveryDecision
	}{
		{name: "capacity", err: &Error{Code: CodeModelCapacity, Confidence: ConfHigh}, want: DecisionShortRetry},
		{name: "network", err: &Error{Code: CodeNetworkUnavailable, Confidence: ConfHigh}, want: DecisionShortRetry},
		{name: "quota", err: &Error{Code: CodeQuotaLimited, Confidence: ConfHigh}, want: DecisionManual},
		{name: "subscription", err: &Error{Code: CodeSubscriptionRequired, Confidence: ConfHigh}, want: DecisionManual},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Decide(ContextKanban, tc.err, now); got != tc.want {
				t.Fatalf("decision = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDecideOnlyShortRetriesValidatedRateLimits(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	short := now.Add(30 * time.Second)
	long := now.Add(61 * time.Second)
	if got := Decide(ContextKanban, &Error{Code: CodeRateLimited, Confidence: ConfHigh, ResetHint: &short}, now); got != DecisionShortRetry {
		t.Fatalf("30-second rate limit decision = %q, want short_retry", got)
	}
	if got := Decide(ContextKanban, &Error{Code: CodeRateLimited, Confidence: ConfHigh, ResetHint: &long}, now); got != DecisionManual {
		t.Fatalf("long rate limit decision = %q, want manual", got)
	}
	if got := Decide(ContextKanban, &Error{Code: CodeRateLimited, Confidence: ConfHigh}, now); got != DecisionManual {
		t.Fatalf("unknown rate limit decision = %q, want manual", got)
	}
}

func TestDecideOfficeMovesLongProviderFailuresToHealthScheduler(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	if got := Decide(ContextOffice, &Error{Code: CodeQuotaLimited, Confidence: ConfHigh}, now); got != DecisionLongRetry {
		t.Fatalf("quota decision = %q, want long_retry", got)
	}
	for _, code := range []Code{CodeProviderUnavailable, CodeProviderOverloaded} {
		if got := Decide(ContextOffice, &Error{Code: code, Confidence: ConfHigh}, now); got != DecisionShortRetry {
			t.Errorf("%s decision = %q, want short_retry", code, got)
		}
	}
	if got := Decide(ContextOffice, &Error{Code: CodeAuthRequired, Confidence: ConfHigh}, now); got != DecisionFallback {
		t.Fatalf("auth decision = %q, want fallback", got)
	}
}
