package service_test

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/service"
)

func strp(s string) *string { return &s }

// crossProductCase pins one (status, outcome, session presence) cell of
// the AC-005.2 cross-product to the exact shape the spec's predicate
// table assigns it — a literal expectation, not a second computation of
// ClassifyTerminalRun's own branching, so a misclassification (not just
// an out-of-set value) fails the test.
type crossProductCase struct {
	status    string
	outcome   *string
	sessionID string
	want      service.TerminalShape
}

// crossProductCases enumerates all 5 statuses x 7 outcomes x 2 session
// states (70 cells) this codebase's runs.status/outcome/session_id can
// hold, each mapped to its expected shape by hand from the spec table.
func crossProductCases() []crossProductCase {
	outcomeCases := []*string{
		nil,
		strp(service.RunOutcomeProcessed),
		strp(service.RunOutcomeIdleSkipped),
		strp(service.RunOutcomeBudgetBlocked),
		strp(service.RunOutcomeAgentInactive),
		strp(service.RunOutcomeTaskTreeHeld),
		strp("no_agent_launched"), // legacy value; not in current code
	}

	cases := []crossProductCase{
		// status=finished: processed+session decides launched_completed
		// vs. silent_success; a skip outcome decides unlaunched_skipped
		// only without a session; everything else is unclassified.
		{"finished", nil, "", service.ShapeUnclassified},
		{"finished", nil, "sess-1", service.ShapeUnclassified},
		{"finished", strp(service.RunOutcomeProcessed), "", service.ShapeSilentSuccess},
		{"finished", strp(service.RunOutcomeProcessed), "sess-1", service.ShapeLaunchedCompleted},
		{"finished", strp(service.RunOutcomeIdleSkipped), "", service.ShapeUnlaunchedSkipped},
		{"finished", strp(service.RunOutcomeIdleSkipped), "sess-1", service.ShapeUnclassified},
		{"finished", strp(service.RunOutcomeBudgetBlocked), "", service.ShapeUnlaunchedSkipped},
		{"finished", strp(service.RunOutcomeBudgetBlocked), "sess-1", service.ShapeUnclassified},
		{"finished", strp(service.RunOutcomeAgentInactive), "", service.ShapeUnlaunchedSkipped},
		{"finished", strp(service.RunOutcomeAgentInactive), "sess-1", service.ShapeUnclassified},
		{"finished", strp(service.RunOutcomeTaskTreeHeld), "", service.ShapeUnlaunchedSkipped},
		{"finished", strp(service.RunOutcomeTaskTreeHeld), "sess-1", service.ShapeUnclassified},
		{"finished", strp("no_agent_launched"), "", service.ShapeUnclassified},
		{"finished", strp("no_agent_launched"), "sess-1", service.ShapeUnclassified},

		// status=some_unknown_status: never finished, never a recognized
		// failure status, so always unclassified regardless of outcome
		// or session.
		{"some_unknown_status", nil, "", service.ShapeUnclassified},
		{"some_unknown_status", nil, "sess-1", service.ShapeUnclassified},
		{"some_unknown_status", strp(service.RunOutcomeProcessed), "", service.ShapeUnclassified},
		{"some_unknown_status", strp(service.RunOutcomeProcessed), "sess-1", service.ShapeUnclassified},
		{"some_unknown_status", strp(service.RunOutcomeIdleSkipped), "", service.ShapeUnclassified},
		{"some_unknown_status", strp(service.RunOutcomeIdleSkipped), "sess-1", service.ShapeUnclassified},
		{"some_unknown_status", strp(service.RunOutcomeBudgetBlocked), "", service.ShapeUnclassified},
		{"some_unknown_status", strp(service.RunOutcomeBudgetBlocked), "sess-1", service.ShapeUnclassified},
		{"some_unknown_status", strp(service.RunOutcomeAgentInactive), "", service.ShapeUnclassified},
		{"some_unknown_status", strp(service.RunOutcomeAgentInactive), "sess-1", service.ShapeUnclassified},
		{"some_unknown_status", strp(service.RunOutcomeTaskTreeHeld), "", service.ShapeUnclassified},
		{"some_unknown_status", strp(service.RunOutcomeTaskTreeHeld), "sess-1", service.ShapeUnclassified},
		{"some_unknown_status", strp("no_agent_launched"), "", service.ShapeUnclassified},
		{"some_unknown_status", strp("no_agent_launched"), "sess-1", service.ShapeUnclassified},
	}

	// status IN (failed, cancelled, timed_out): the spec's predicate
	// table names all three under one row and never reads outcome for
	// them, so every outcome yields the same pair of shapes for each —
	// unlaunched_failed without a session, launched_failed with one.
	for _, status := range []string{"failed", "cancelled", "timed_out"} {
		for _, outcome := range outcomeCases {
			cases = append(cases,
				crossProductCase{status, outcome, "", service.ShapeUnlaunchedFailed},
				crossProductCase{status, outcome, "sess-1", service.ShapeLaunchedFailed},
			)
		}
	}

	return cases
}

// AC-OFFICE-LOOP-LIVENESS-005.1/.2: classification is total over the
// cross-product of every persisted status this codebase writes
// (finished, failed, cancelled, timed_out, and an unknown status —
// runs.status is not the closed RunStatus enum) against every observed
// outcome (including the legacy no_agent_launched value and NULL),
// crossed with session_id present/absent — pinned to the exact shape
// each of the 70 cells must produce, not just closed-set membership.
func TestClassifyTerminalRun_CrossProduct(t *testing.T) {
	activation := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	requestedAt := activation.Add(time.Hour)

	cases := crossProductCases()
	if len(cases) != 5*7*2 {
		t.Fatalf("cross-product case count = %d, want %d (5 statuses x 7 outcomes x 2 session states)",
			len(cases), 5*7*2)
	}

	for _, tc := range cases {
		got := service.ClassifyTerminalRun(
			tc.status, tc.outcome, tc.sessionID, requestedAt, activation, true,
		)
		if got != tc.want {
			t.Errorf("ClassifyTerminalRun(status=%q, outcome=%v, session=%q) = %q, want %q",
				tc.status, tc.outcome, tc.sessionID, got, tc.want)
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
