package ledger

import (
	"testing"
	"time"
)

func TestReader_NeverReturnsDreamTurns(t *testing.T) {
	f := newFixture(t)
	base := time.Now().UTC().Add(-time.Hour)
	f.ledgerTurn("wake-1", "st1", base, true, VerdictActed)
	f.exec(`INSERT INTO coordinator_turns (id, coordinator_id, session_id, session_turn_id, "trigger", started_at, finished_at, outcome, verdict)
		VALUES ('dream-1', ?, ?, 'sd1', 'dream', ?, ?, 'completed', ?)`, f.coord.ID, testSession, base.Add(time.Minute), base.Add(2*time.Minute), VerdictActed)

	for name, args := range map[string]ListArgs{"unfiltered": {}, "trigger=dream": {Trigger: TriggerDream}} {
		p, err := f.l.List(t.Context(), f.coord.ID, args)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, turn := range p.Turns {
			if turn.ID == "dream-1" {
				t.Fatalf("%s: a dream turn was returned: %+v", name, p.Turns)
			}
		}
	}
	p, _ := f.l.List(t.Context(), f.coord.ID, ListArgs{})
	if len(p.Turns) != 1 || p.Turns[0].ID != "wake-1" {
		t.Fatalf("turns = %+v, want only the wake turn", p.Turns)
	}
}
