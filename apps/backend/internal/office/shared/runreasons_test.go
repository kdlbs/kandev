package shared_test

import (
	"testing"

	"github.com/kandev/kandev/internal/office/shared"
)

// TestIsPeriodicTasklessWake_LegacyRoutineDispatchIsNonSkippable pins the
// WO-46.1 Finding 2 fix: the legacy "routine_dispatch" literal was written
// for cron, manual, and webhook fires alike before
// RunReasonRoutineDispatchCron existed, so a persisted run row cannot say
// which trigger produced it. Treating it as periodic would risk silently
// skipping a pre-upgrade manual/webhook fire — see
// docs/specs/office/scheduler.md ("Event-triggered wakeups always proceed").
func TestIsPeriodicTasklessWake_LegacyRoutineDispatchIsNonSkippable(t *testing.T) {
	if shared.IsPeriodicTasklessWake(shared.RunReasonRoutineDispatch) {
		t.Errorf("IsPeriodicTasklessWake(%q) = true, want false — the legacy "+
			"literal is ambiguous and must default to non-skippable",
			shared.RunReasonRoutineDispatch)
	}
}

// TestIsPeriodicTasklessWake_CronConstantIsSkippable pins the new
// cron-only constant as the sole "definitely periodic" routine-dispatch
// value going forward.
func TestIsPeriodicTasklessWake_CronConstantIsSkippable(t *testing.T) {
	if !shared.IsPeriodicTasklessWake(shared.RunReasonRoutineDispatchCron) {
		t.Errorf("IsPeriodicTasklessWake(%q) = false, want true",
			shared.RunReasonRoutineDispatchCron)
	}
}

// TestRoutineDispatchReason_SourceMapping pins RoutineDispatchReason's
// source→reason mapping so RunReasonRoutineDispatchCron and
// RunReasonRoutineDispatchEvent cannot silently drift apart from
// RoutineSourceCron / non-cron classification.
func TestRoutineDispatchReason_SourceMapping(t *testing.T) {
	if got := shared.RoutineDispatchReason(shared.RoutineSourceCron); got != shared.RunReasonRoutineDispatchCron {
		t.Errorf("RoutineDispatchReason(cron) = %q, want %q", got, shared.RunReasonRoutineDispatchCron)
	}
	if got := shared.RoutineDispatchReason("manual"); got != shared.RunReasonRoutineDispatchEvent {
		t.Errorf("RoutineDispatchReason(manual) = %q, want %q", got, shared.RunReasonRoutineDispatchEvent)
	}
	if got := shared.RoutineDispatchReason("webhook"); got != shared.RunReasonRoutineDispatchEvent {
		t.Errorf("RoutineDispatchReason(webhook) = %q, want %q", got, shared.RunReasonRoutineDispatchEvent)
	}
}
