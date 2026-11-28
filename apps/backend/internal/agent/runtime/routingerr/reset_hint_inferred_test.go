package routingerr

import (
	"testing"
	"time"
)

// A clock-only session-limit notice yields an inferred reset time; a
// structured provider reset time is explicit.
//
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.20
func TestClassifyMarksClockOnlyResetHintInferred(t *testing.T) {
	resetInjection()
	notice := "You've hit your session limit · resets 11:10am (Europe/Helsinki)"
	observed := time.Date(2026, time.October, 20, 0, 0, 0, 0, time.UTC)
	inferred := Classify(Input{ProviderID: "claude-acp", Phase: PhaseStreaming, Stderr: notice, OccurredAt: observed})
	if inferred.ResetHint == nil || !inferred.ResetHintInferred {
		t.Fatalf("classification = %+v, want an inferred reset hint", inferred)
	}
	structured := observed.Add(48 * time.Hour)
	explicit := Classify(Input{
		ProviderID: "claude-acp", Phase: PhaseStreaming, Stderr: notice,
		OccurredAt: observed, ResetHint: &structured,
	})
	if explicit.ResetHint == nil || !explicit.ResetHint.Equal(structured) || explicit.ResetHintInferred {
		t.Fatalf("classification = %+v, want the explicit structured reset hint", explicit)
	}
}
