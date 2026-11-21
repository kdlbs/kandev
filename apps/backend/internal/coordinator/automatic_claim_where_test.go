package coordinator

import (
	"context"
	"testing"
	"time"
)

func TestClaimProposalRawTx_AutomaticClaimTakesOnlyUntouchedPending(t *testing.T) {
	store, c, _, _, _ := automaticFixture(t)
	ctx := context.Background()
	now := time.Now()
	claim := func(id string, automatic bool) bool {
		var at *time.Time
		if automatic {
			at = &now
		}
		ok, err := store.ClaimProposalRawTx(ctx, store.db, id, "tok", `{}`, "u", now, at)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	pending := insertKind(t, store, c, ProposalKindCreateTask)
	if !claim(pending.ID, true) || claim(pending.ID, true) {
		t.Fatal("an automatic claim must take a pending row exactly once")
	}
	failed := insertKind(t, store, c, ProposalKindCreateTask)
	mustExec(t, store, `UPDATE coordinator_proposals SET status = 'failed' WHERE id = ?`, failed.ID)
	if claim(failed.ID, true) {
		t.Fatal("an automatic claim took a failed proposal")
	}
	if !claim(failed.ID, false) {
		t.Fatal("a manager claim must still retry a failed proposal")
	}
	touched := insertKind(t, store, c, ProposalKindCreateTask)
	mustExec(t, store, `UPDATE coordinator_proposals SET decided_automatically = 1 WHERE id = ?`, touched.ID)
	if claim(touched.ID, true) {
		t.Fatal("an automatic claim took a proposal an automatic attempt already decided")
	}
}
