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
	if got := Decide(ContextKanban, &Error{Code: CodeRateLimited, Confidence: ConfHigh}, now); got != DecisionShortRetry {
		t.Fatalf("high-confidence rate limit without a deadline decision = %q, want short_retry", got)
	}
}

// @covers AC-PLATFORM-RATE-THROTTLE-RETRY-001.1 AC-PLATFORM-RATE-THROTTLE-RETRY-001.2 AC-PLATFORM-RATE-THROTTLE-RETRY-001.3 AC-PLATFORM-RATE-THROTTLE-RETRY-001.5
func TestInteractiveRateLimitWithoutReset(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	zero := time.Time{}
	future := now.Add(30 * time.Second)
	expired := now.Add(-time.Second)
	long := now.Add(61 * time.Second)
	cases := []struct {
		name       string
		failure    *Error
		kanbanWant RecoveryDecision
		officeWant RecoveryDecision
	}{
		{name: "high confidence without hint", failure: &Error{Code: CodeRateLimited, Confidence: ConfHigh}, kanbanWant: DecisionShortRetry, officeWant: DecisionLongRetry},
		{name: "medium confidence without hint", failure: &Error{Code: CodeRateLimited, Confidence: ConfMedium}, kanbanWant: DecisionManual, officeWant: DecisionLongRetry},
		{name: "low confidence without hint", failure: &Error{Code: CodeRateLimited, Confidence: ConfLow}, kanbanWant: DecisionManual, officeWant: DecisionLongRetry},
		{name: "zero hint", failure: &Error{Code: CodeRateLimited, Confidence: ConfHigh, ResetHint: &zero}, kanbanWant: DecisionManual, officeWant: DecisionLongRetry},
		{name: "future hint", failure: &Error{Code: CodeRateLimited, Confidence: ConfHigh, ResetHint: &future}, kanbanWant: DecisionShortRetry, officeWant: DecisionShortRetry},
		{name: "expired hint", failure: &Error{Code: CodeRateLimited, Confidence: ConfHigh, ResetHint: &expired}, kanbanWant: DecisionManual, officeWant: DecisionLongRetry},
		{name: "long hint", failure: &Error{Code: CodeRateLimited, Confidence: ConfHigh, ResetHint: &long}, kanbanWant: DecisionManual, officeWant: DecisionLongRetry},
		{name: "quota without hint", failure: &Error{Code: CodeQuotaLimited, Confidence: ConfHigh}, kanbanWant: DecisionManual, officeWant: DecisionLongRetry},
		{name: "nil failure", failure: nil, kanbanWant: DecisionManual, officeWant: DecisionManual},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Decide(ContextKanban, tc.failure, now); got != tc.kanbanWant {
				t.Errorf("Kanban decision = %q, want %q", got, tc.kanbanWant)
			}
			if got := Decide(ContextOffice, tc.failure, now); got != tc.officeWant {
				t.Errorf("Office decision = %q, want %q", got, tc.officeWant)
			}
		})
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
