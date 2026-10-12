package recorder

import "testing"

func TestSweepDoesNotCountFailedGrading(t *testing.T) {
	f := newFixture(t)
	f.proposal("p1", "rejected", "create_task", "", f.at(0))
	f.exec(`DROP TABLE coordinator_outcomes`)
	// The discovery query must remain valid while the real grader fails.
	f.exec(`CREATE TABLE coordinator_outcomes (proposal_id TEXT PRIMARY KEY, final INTEGER, turn_id TEXT, graded_at DATETIME)`)
	if got := (SQLProposals{DB: f.db}).Sweep(t.Context(), f.queue()); got != 0 {
		t.Fatalf("failed grades counted as successful: %d", got)
	}
}
