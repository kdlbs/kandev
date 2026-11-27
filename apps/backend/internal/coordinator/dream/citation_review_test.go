package dream

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestDreamCitationsResolveOnlyRenderedEvidence(t *testing.T) {
	f := newFixture(t)
	ids := f.seedEvidence(10)
	for _, id := range ids {
		for i := range 100 {
			f.exec(`INSERT INTO coordinator_turn_calls (turn_id, action, allowed, recorded_at) VALUES (?, ?, ?, ?)`,
				id, fmt.Sprintf("tool_%03d_%s", i, strings.Repeat("x", 110)), true, f.now.Add(-time.Hour))
		}
	}
	f.exec(`UPDATE coordinator_turns SET started_at = ?, finished_at = ?`, f.now.Add(-2*time.Hour), f.now.Add(-time.Hour))
	p, ok := f.sch.gather(t.Context(), f.coord, f.now.Add(-24*time.Hour), f.now)
	if !ok || len(p.evidence.TurnIDs) == 0 || len(p.evidence.TurnIDs) >= len(ids) {
		t.Fatalf("bounded evidence not gathered: ok=%v rendered=%d", ok, len(p.evidence.TurnIDs))
	}
	if len(p.turns) != len(p.evidence.TurnIDs) {
		t.Errorf("citation resolver admits %d turns but rendered evidence contains %d", len(p.turns), len(p.evidence.TurnIDs))
	}
	for _, id := range p.evidence.TurnIDs {
		if _, exists := p.turns[id]; !exists {
			t.Errorf("rendered citation %q does not resolve", id)
		}
	}
}
