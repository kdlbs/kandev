package service_test

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/service"
)

func strp(s string) *string { return &s }

// AC-OFFICE-LOOP-LIVENESS-005.1/.2: classification is total over the
// cross-product of every persisted status this codebase writes
// (finished, failed, cancelled, timed_out, and an unknown status —
// runs.status is not the closed RunStatus enum) against every observed
// outcome (including the legacy no_agent_launched value and NULL),
// crossed with session_id present/absent.
func TestClassifyTerminalRun_CrossProduct(t *testing.T) {
	activation := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	requestedAt := activation.Add(time.Hour)

	statuses := []string{"finished", "failed", "cancelled", "timed_out", "some_unknown_status"}
	outcomes := []*string{
		nil,
		strp(service.RunOutcomeProcessed),
		strp(service.RunOutcomeIdleSkipped),
		strp(service.RunOutcomeBudgetBlocked),
		strp(service.RunOutcomeAgentInactive),
		strp(service.RunOutcomeTaskTreeHeld),
		strp("no_agent_launched"), // legacy value; not in current code
	}
	sessionIDs := []string{"", "sess-1"}

	// Closed-set membership, not just non-empty: an empty-string check
	// passes vacuously here because ClassifyTerminalRun's own default
	// branch always returns ShapeUnclassified rather than "" — a shape
	// outside this set (a typo, a new constant not returned by any
	// branch) would slip through an emptiness check silently (Review
	// round 1, should-fix AC-005.2).
	validShapes := map[service.TerminalShape]bool{
		service.ShapePreActivation:     true,
		service.ShapeLaunchedCompleted: true,
		service.ShapeLaunchedFailed:    true,
		service.ShapeSilentSuccess:     true,
		service.ShapeUnlaunchedSkipped: true,
		service.ShapeUnlaunchedFailed:  true,
		service.ShapeUnclassified:      true,
	}

	for _, status := range statuses {
		for _, outcome := range outcomes {
			for _, sessionID := range sessionIDs {
				shape := service.ClassifyTerminalRun(
					status, outcome, sessionID, requestedAt, activation, true,
				)
				if !validShapes[shape] {
					t.Fatalf("shape %q not in the closed set for status=%q outcome=%v session=%q",
						shape, status, outcome, sessionID)
				}
			}
		}
	}
}

func TestClassifyTerminalRun_PreActivation(t *testing.T) {
	activation := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Before the activation instant: pre_activation regardless of
	// otherwise-silent-success shape (AC-005.6).
	requestedBefore := activation.Add(-time.Minute)
	got := service.ClassifyTerminalRun(
		"finished", strp(service.RunOutcomeProcessed), "", requestedBefore, activation, true,
	)
	if got != service.ShapePreActivation {
		t.Fatalf("got %q, want pre_activation", got)
	}

	// Activation instant never published: always pre_activation.
	got = service.ClassifyTerminalRun(
		"finished", strp(service.RunOutcomeProcessed), "", requestedBefore.Add(10*time.Hour), activation, false,
	)
	if got != service.ShapePreActivation {
		t.Fatalf("got %q, want pre_activation when unpublished", got)
	}
}

func TestClassifyTerminalRun_NamedShapes(t *testing.T) {
	activation := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	requestedAt := activation.Add(time.Hour)

	cases := []struct {
		name      string
		status    string
		outcome   *string
		sessionID string
		want      service.TerminalShape
	}{
		{"launched_completed", "finished", strp(service.RunOutcomeProcessed), "sess-1", service.ShapeLaunchedCompleted},
		{"launched_failed/failed", "failed", nil, "sess-1", service.ShapeLaunchedFailed},
		{"launched_failed/timed_out", "timed_out", nil, "sess-1", service.ShapeLaunchedFailed},
		{"launched_failed/cancelled", "cancelled", nil, "sess-1", service.ShapeLaunchedFailed},
		{"silent_success", "finished", strp(service.RunOutcomeProcessed), "", service.ShapeSilentSuccess},
		{"unlaunched_skipped/idle", "finished", strp(service.RunOutcomeIdleSkipped), "", service.ShapeUnlaunchedSkipped},
		{"unlaunched_skipped/budget", "finished", strp(service.RunOutcomeBudgetBlocked), "", service.ShapeUnlaunchedSkipped},
		{"unlaunched_skipped/inactive", "finished", strp(service.RunOutcomeAgentInactive), "", service.ShapeUnlaunchedSkipped},
		{"unlaunched_skipped/tree_held", "finished", strp(service.RunOutcomeTaskTreeHeld), "", service.ShapeUnlaunchedSkipped},
		{"unlaunched_failed", "failed", nil, "", service.ShapeUnlaunchedFailed},
		{"unclassified/null_outcome_finished", "finished", nil, "", service.ShapeUnclassified},
		{"unclassified/legacy_no_agent_launched", "finished", strp("no_agent_launched"), "", service.ShapeUnclassified},
		// A skip outcome carrying a session id is not a real production
		// shape (skip outcomes are pre-launch guards) but the total
		// classification must still place it somewhere, and it must not
		// be reported as "unlaunched" while a session is on record
		// (AC-005.3) — it falls to unclassified.
		{"unclassified/skip_with_session", "finished", strp(service.RunOutcomeIdleSkipped), "sess-1", service.ShapeUnclassified},
		{"unclassified/unknown_status", "some_unknown_status", nil, "", service.ShapeUnclassified},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := service.ClassifyTerminalRun(tc.status, tc.outcome, tc.sessionID, requestedAt, activation, true)
			if got != tc.want {
				t.Fatalf("ClassifyTerminalRun(%q, %v, %q) = %q, want %q",
					tc.status, tc.outcome, tc.sessionID, got, tc.want)
			}
		})
	}
}
