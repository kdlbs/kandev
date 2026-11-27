package coordinator

import "testing"

func testAutomaticPauseAtClaim(t *testing.T, store *Store) {
	t.Helper()
	c, svc := newAutomaticServiceOn(t, store)
	tasks := svc.decisionTasks.(*fakeDecisionTaskService)
	az := svc.authz.(*identityAuthorizer)
	az.onCall = func() {
		changed, err := store.SetPaused(t.Context(), c.ID, true, testRaiser)
		if err != nil || !changed {
			t.Fatalf("pause during raiser check: %t %v", changed, err)
		}
	}
	p, result := proposeAuto(t, store, c, svc)
	if result == nil || result.Status != ProposalStatusPending || result.Note != pausedNote {
		t.Fatalf("paused claim result: %+v", result)
	}
	fresh := reload(t, store, c, p.ID)
	if fresh.Status != ProposalStatusPending || fresh.ClaimedAutomatically || len(tasks.createCalls) != 0 {
		t.Fatalf("pause committed before claim created work: %+v, creates=%d", fresh, len(tasks.createCalls))
	}
	count, err := store.CountAutomaticDecidedTx(t.Context(), store.db, c.ID, automaticNow.Add(-automaticLimitWindow))
	if err != nil || count != 0 {
		t.Fatalf("paused proposal consumed automatic limit: %d %v", count, err)
	}
}

func TestAutomaticPauseAtClaimSQLite(t *testing.T) { testAutomaticPauseAtClaim(t, newTestStore(t)) }
func TestAutomaticPauseAtClaimPostgres(t *testing.T) {
	testAutomaticPauseAtClaim(t, newMultiConnStorePostgres(t))
}
