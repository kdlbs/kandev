package routingerr

import (
	"testing"
	"time"
)

func TestClassify_OpenCodeFreeUsageLimitDerivesRelativeReset(t *testing.T) {
	resetInjection()
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	for _, stderr := range []string{
		"Free usage limit reached. Resets in 4 hours.",
		"free usage limit reached. Resets in 4 hours. Upgrade to continue.",
	} {
		e := Classify(Input{Phase: PhaseStreaming, ProviderID: "opencode-acp", Stderr: stderr, OccurredAt: now})
		if e.Code != CodeQuotaLimited {
			t.Fatalf("classification of %q = %+v, want quota_limited", stderr, e)
		}
		if e.ResetHint == nil || !e.ResetHint.Equal(now.Add(4*time.Hour)) {
			t.Fatalf("reset of %q = %v, want %v", stderr, e.ResetHint, now.Add(4*time.Hour))
		}
	}
}

func TestClassify_OpenCodeFreeUsageLimitWithoutRelativeResetHasNoHint(t *testing.T) {
	resetInjection()
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	e := Classify(Input{Phase: PhaseStreaming, ProviderID: "opencode-acp", Stderr: "Free usage limit reached.", OccurredAt: now})
	if e.Code != CodeQuotaLimited || e.ResetHint != nil {
		t.Fatalf("classification = %+v, want quota_limited without a reset hint", e)
	}
}
