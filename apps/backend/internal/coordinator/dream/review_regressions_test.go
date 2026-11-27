package dream

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/coordinator"
)

func TestHealthSpacingExposesNextDreamTime(t *testing.T) {
	in := okInput()
	in.DebtSince = ago(time.Hour)
	in.NextAfter = ahead(time.Hour)
	got := Health(in)
	if got.Condition != WaitSpacing || got.Detail != in.NextAfter.UTC().Format(time.RFC3339) {
		t.Fatalf("spacing state = %+v; want next time %s", got, in.NextAfter.Format(time.RFC3339))
	}
}

func TestDebtSinceWaitsForQualifyingDecision(t *testing.T) {
	f := newFixture(t)
	f.seedEvidence(MinTurns)
	decisionAt := f.now.Add(-30 * time.Minute)
	f.exec(`UPDATE coordinator_outcomes SET decided_at = ?`, decisionAt)
	got, err := f.sch.debtSince(t.Context(), f.coord.ID, nil, f.now)
	if err != nil || got == nil || !got.Equal(decisionAt) {
		t.Fatalf("debt starts at %v, err %v; want %s", got, err, decisionAt)
	}
}

func TestDebtSinceWaitsForLastRequiredCompletion(t *testing.T) {
	f := newFixture(t)
	ids := f.seedEvidence(MinTurns)
	completedAt := f.now.Add(-10 * time.Minute)
	f.exec(`UPDATE coordinator_turns SET finished_at = ? WHERE id = ?`, completedAt, ids[0])
	got, err := f.sch.debtSince(t.Context(), f.coord.ID, nil, f.now)
	if err != nil || got == nil || !got.Equal(completedAt) {
		t.Fatalf("debt starts at %v, err %v; want final required completion %s", got, err, completedAt)
	}
}

func TestReportDropsCredentialBearingCitations(t *testing.T) {
	f := newFixture(t)
	ids := f.seedEvidence(MinTurns)
	rows, err := f.store.DreamWindowTurns(t.Context(), f.coord.ID, f.now.Add(-24*time.Hour), f.now, 20)
	if err != nil {
		t.Fatal(err)
	}
	turns := make(map[string]coordinator.DreamTurn, len(rows))
	for _, row := range rows {
		turns[row.ID] = row
	}
	p := plan{dream: coordinator.Dream{ID: "report", WindowStart: f.now.Add(-24 * time.Hour), WindowEnd: f.now}, turns: turns}
	secret := "kandev_pat_do_not_store"
	ans := Answer{Items: []Item{{Kind: KindNoteAdd, Text: "safe", CitedTurnIDs: []string{ids[0], secret}}}}
	items, reason := f.sch.report(context.Background(), f.coord, p, ans)
	if reason != "" || len(items) != 1 {
		t.Fatalf("report = %v, reason %s", items, reason)
	}
	for _, id := range items[0].CitedTurnIDs {
		if strings.Contains(id, secret) {
			t.Fatalf("report retains credential-bearing citation: %+v", items[0])
		}
	}
}

func TestDreamAdmissionIncludesOverrideEvidence(t *testing.T) {
	f := newFixture(t)
	f.seedEvidence(MinTurns)
	f.exec(`DELETE FROM coordinator_outcomes`)
	f.exec(`INSERT INTO coordinator_feedback (id, coordinator_id, kind, proposal_id, user_id, reason_code, proposal_kind, created_at)
		VALUES ('feedback-1', ?, 'undone', 'override-proposal', 'manager', 'none', 'create_task', ?)`, f.coord.ID, f.now.Add(-30*time.Minute))
	c, err := f.store.GetCoordinatorByID(t.Context(), f.coord.ID)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := f.sch.admit(t.Context(), c, f.now)
	if !ok || !strings.Contains(p.evidence.Message, "override-proposal") || !strings.Contains(p.evidence.Message, "undone") {
		t.Fatalf("override admitted without its evidence: admitted=%v, evidence=%s", ok, p.evidence.Message)
	}
}
