package shared_test

import (
	"testing"

	"github.com/kandev/kandev/internal/office/shared"
)

// TestClassifyRunProvenance_AttendedAllowlist pins AC-OFFICE-BUDGET-007.2's
// exact 20-literal attended allowlist (16 current + 4 legacy).
func TestClassifyRunProvenance_AttendedAllowlist(t *testing.T) {
	attended := []string{
		"task_assigned",
		"task_comment",
		"task_review_requested",
		"task_changes_requested",
		"task_blockers_resolved",
		"task_children_completed",
		"approval_resolved",
		"routine_dispatch_event",
		"manual_resume_after_failure",
		"task_mentioned",
		"task_reopened",
		"task_reopened_via_comment",
		"task_unblocked",
		"task_ready_to_close",
		"stage_pending",
		"stage_changes_requested",
		"review_started",
		"approval_started",
		"blockers_resolved",
		"children_completed",
	}
	if len(attended) != 20 {
		t.Fatalf("test fixture drift: want 20 attended literals, got %d", len(attended))
	}
	for _, reason := range attended {
		if got := shared.ClassifyRunProvenance(reason); got != shared.RunProvenanceAttended {
			t.Errorf("ClassifyRunProvenance(%q) = %v, want RunProvenanceAttended", reason, got)
		}
	}
}

// TestClassifyRunProvenance_UnattendedExplicitList pins AC-OFFICE-BUDGET-007.4's
// exact 6-literal unattended list.
func TestClassifyRunProvenance_UnattendedExplicitList(t *testing.T) {
	unattended := []string{
		"agent_error",
		"budget_alert",
		"heartbeat",
		"routine_dispatch",
		"routine_dispatch_cron",
		"routine_trigger",
	}
	if len(unattended) != 6 {
		t.Fatalf("test fixture drift: want 6 unattended literals, got %d", len(unattended))
	}
	for _, reason := range unattended {
		if got := shared.ClassifyRunProvenance(reason); got != shared.RunProvenanceUnattended {
			t.Errorf("ClassifyRunProvenance(%q) = %v, want RunProvenanceUnattended", reason, got)
		}
	}
}

// TestClassifyRunProvenance_UnknownReasonIsUnattended pins AC-OFFICE-BUDGET-007.2's
// safe default: a value the build has never seen classifies unattended, never
// as a third state and never exempt from a gate.
func TestClassifyRunProvenance_UnknownReasonIsUnattended(t *testing.T) {
	for _, reason := range []string{"", "some_future_reason_nobody_has_written_yet"} {
		if got := shared.ClassifyRunProvenance(reason); got != shared.RunProvenanceUnattended {
			t.Errorf("ClassifyRunProvenance(%q) = %v, want RunProvenanceUnattended (safe default)", reason, got)
		}
	}
}

// TestClassifyRunProvenance_PolarityDiffersFromIsPeriodicTasklessWake pins the
// glossary's polarity warning: routine_dispatch is non-periodic for the
// idle-skip gate but unattended for budget enforcement — the same literal,
// opposite safe answer. A shared implementation would invert one of them.
func TestClassifyRunProvenance_PolarityDiffersFromIsPeriodicTasklessWake(t *testing.T) {
	reason := shared.RunReasonRoutineDispatch
	if shared.IsPeriodicTasklessWake(reason) {
		t.Fatalf("test assumption violated: IsPeriodicTasklessWake(%q) should be false", reason)
	}
	if got := shared.ClassifyRunProvenance(reason); got != shared.RunProvenanceUnattended {
		t.Errorf("ClassifyRunProvenance(%q) = %v, want RunProvenanceUnattended even though "+
			"IsPeriodicTasklessWake treats it as non-periodic", reason, got)
	}
}

func TestRunProvenance_Attended(t *testing.T) {
	if !shared.RunProvenanceAttended.Attended() {
		t.Error("RunProvenanceAttended.Attended() = false, want true")
	}
	if shared.RunProvenanceUnattended.Attended() {
		t.Error("RunProvenanceUnattended.Attended() = true, want false")
	}
}
