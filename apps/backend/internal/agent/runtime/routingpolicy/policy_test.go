package routingpolicy

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

func policyWithRetry(maxRetries int64) Policy {
	policy := DefaultPolicy()
	policy.Retry = RetryPolicy{Enabled: true, MaxRetries: maxRetries, InitialIntervalSeconds: 5}
	return policy
}

func safeFailure(class routingerr.Class) *routingerr.Error {
	return &routingerr.Error{
		Code:             routingerr.CodeRateLimited,
		Class:            class,
		CatalogueVersion: routingerr.CatalogueVersion,
		FallbackAllowed:  true,
	}
}

func TestNextRetryDelayDoublesAndCaps(t *testing.T) {
	tests := []struct {
		ordinal int64
		want    time.Duration
	}{
		{ordinal: 0, want: 5 * time.Second},
		{ordinal: 1, want: 10 * time.Second},
		{ordinal: 2, want: 20 * time.Second},
		{ordinal: 30, want: MaxRetryDelay},
	}
	for _, tt := range tests {
		t.Run("ordinal-"+itoa(tt.ordinal), func(t *testing.T) {
			got, err := NextRetryDelay(5, tt.ordinal)
			if err != nil {
				t.Fatalf("NextRetryDelay: %v", err)
			}
			if got != tt.want {
				t.Fatalf("delay = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestEvaluateOrdersSafetyResetRetryAndExhaustion(t *testing.T) {
	now := time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC)
	document := DefaultDocument()
	document.Transient = policyWithRetry(2)
	document.Transient.WaitForReset = ResetWaitPolicy{Enabled: true, MaxWaitSeconds: 300}

	tests := []struct {
		name          string
		failure       *routingerr.Error
		effectSafe    bool
		retryOrdinal  int64
		resetWaitUsed bool
		wantKind      DecisionKind
		wantOrdinal   int64
		wantDeadline  time.Time
	}{
		{name: "unsafe effect stops", failure: safeFailure(routingerr.ClassTransient), wantKind: DecisionStop},
		{name: "unclassified stops", failure: safeFailure(routingerr.ClassUnclassified), effectSafe: true, wantKind: DecisionStop},
		{
			name:         "trusted near reset waits once",
			failure:      withReset(safeFailure(routingerr.ClassTransient), now.Add(2*time.Minute)),
			effectSafe:   true,
			wantKind:     DecisionWaitForReset,
			wantDeadline: now.Add(2 * time.Minute),
		},
		{
			name:          "reset wait does not loop",
			failure:       withReset(safeFailure(routingerr.ClassTransient), now.Add(2*time.Minute)),
			effectSafe:    true,
			resetWaitUsed: true,
			wantKind:      DecisionRetry,
			wantOrdinal:   1,
			wantDeadline:  now.Add(5 * time.Second),
		},
		{
			name:         "retry doubles",
			failure:      safeFailure(routingerr.ClassTransient),
			effectSafe:   true,
			retryOrdinal: 1,
			wantKind:     DecisionRetry,
			wantOrdinal:  2,
			wantDeadline: now.Add(10 * time.Second),
		},
		{
			name:         "retry exhaustion skips",
			failure:      safeFailure(routingerr.ClassTransient),
			effectSafe:   true,
			retryOrdinal: 2,
			wantKind:     DecisionSkip,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Evaluate(document, EvaluationInput{
				Failure: tt.failure, Now: now, RetryOrdinal: tt.retryOrdinal,
				ResetWaitUsed: tt.resetWaitUsed, EffectSafe: tt.effectSafe,
			})
			if got.Kind != tt.wantKind {
				t.Fatalf("kind = %s, want %s (%#v)", got.Kind, tt.wantKind, got)
			}
			if got.RetryOrdinal != tt.wantOrdinal {
				t.Fatalf("retry ordinal = %d, want %d", got.RetryOrdinal, tt.wantOrdinal)
			}
			if !tt.wantDeadline.IsZero() && !got.Deadline.Equal(tt.wantDeadline) {
				t.Fatalf("deadline = %s, want %s", got.Deadline, tt.wantDeadline)
			}
		})
	}
}

func TestEvaluateHonorsStopAfterExhaustion(t *testing.T) {
	document := DefaultDocument()
	document.Hard = policyWithRetry(1)
	document.Hard.OnExhausted = OutcomeStop
	got := Evaluate(document, EvaluationInput{
		Failure:      safeFailure(routingerr.ClassHard),
		EffectSafe:   true,
		RetryOrdinal: 1,
		Now:          time.Unix(100, 0).UTC(),
	})
	if got.Kind != DecisionStop || got.PendingOutcome != OutcomeStop {
		t.Fatalf("evaluation = %#v, want stop", got)
	}
}

func withReset(failure *routingerr.Error, resetAt time.Time) *routingerr.Error {
	failure.ResetHint = &resetAt
	return failure
}

func itoa(value int64) string {
	if value == 0 {
		return "0"
	}
	return "large"
}
