package coordinator

import (
	"context"
	"testing"
	"time"
)

var dreamT0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func newDream(id, coord string, at time.Time) Dream {
	return Dream{ID: id, CoordinatorID: coord, WindowStart: at.Add(-24 * time.Hour), WindowEnd: at, InputHash: "h-" + id, Model: "m", StartedAt: at}
}

func runDreamStoreConformance(t *testing.T, store *Store) {
	t.Helper()
	ctx := context.Background()

	ok, err := store.InsertRunningDream(ctx, newDream("d1", "c1", dreamT0))
	if err != nil || !ok {
		t.Fatalf("first running insert = %v %v", ok, err)
	}
	ok, err = store.InsertRunningDream(ctx, newDream("d2", "c1", dreamT0.Add(time.Minute)))
	if err != nil || ok {
		t.Fatalf("second running insert for one coordinator = %v %v, want refused", ok, err)
	}
	if ok, err = store.InsertRunningDream(ctx, newDream("d3", "c2", dreamT0)); err != nil || !ok {
		t.Fatalf("other coordinator's running insert = %v %v", ok, err)
	}

	if ok, _ := store.RefreshDream(ctx, "d1", dreamT0.Add(time.Minute)); !ok {
		t.Fatal("refresh of a running row must match")
	}
	expired, err := store.ExpireStaleDreams(ctx, "c1", dreamT0.Add(2*time.Minute), dreamT0.Add(10*time.Minute))
	if err != nil || len(expired) != 1 || expired[0].ID != "d1" || expired[0].Reason != "lease_lost" {
		t.Fatalf("expire = %+v %v", expired, err)
	}
	if ok, _ := store.RefreshDream(ctx, "d1", dreamT0.Add(11*time.Minute)); ok {
		t.Fatal("refresh after expiry must match nothing")
	}
	late := newDream("d1", "c1", dreamT0)
	late.Status, late.Reason = DreamClean, ""
	if ok, err := store.FinishDream(ctx, late, []DreamItem{{ID: "i-late", Position: 0, Kind: "note_add", Gate: "pass"}}, dreamT0.Add(12*time.Minute)); err != nil || ok {
		t.Fatalf("late completion = %v %v, want no change", ok, err)
	}
	if items, _ := store.ListDreamItems(ctx, "d1"); len(items) != 0 {
		t.Fatal("late completion stored an item")
	}
	if ok, _ := store.FailDream(ctx, "d1", "paused", dreamT0); ok {
		t.Fatal("fail of a non-running row must match nothing")
	}

	if ok, err := store.SetDreamEpisodeTask(ctx, "d3", "task-1"); err != nil || !ok {
		t.Fatalf("set task = %v %v", ok, err)
	}
	if ok, err := store.SetDreamEpisodeSession(ctx, "d3", "sess-1"); err != nil || !ok {
		t.Fatalf("set session = %v %v", ok, err)
	}
	if ok, _ := store.FailDream(ctx, "d3", "paused", dreamT0.Add(time.Minute)); !ok {
		t.Fatal("fail of a running row must match")
	}
	open, err := store.DreamsWithOpenEpisode(ctx, "c2")
	if err != nil || len(open) != 1 || open[0].EpisodeTaskID != "task-1" || open[0].EpisodeSessionID != "sess-1" {
		t.Fatalf("open episodes = %+v %v", open, err)
	}
	ids, _ := store.DreamCoordinatorIDs(ctx)
	if len(ids) != 1 || ids[0] != "c2" {
		// c1's expired row names no episode task, so only c2 needs a visit.
		t.Fatalf("visit ids = %v", ids)
	}
	if err := store.MarkDreamEpisodeArchived(ctx, "d3", dreamT0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if open, _ = store.DreamsWithOpenEpisode(ctx, "c2"); len(open) != 0 {
		t.Fatalf("archived episode still open: %+v", open)
	}
	if ids, _ = store.DreamCoordinatorIDs(ctx); len(ids) != 0 {
		t.Fatalf("visit ids after archive = %v", ids)
	}

	runDreamReportConformance(t, store)
}

func runDreamReportConformance(t *testing.T, store *Store) {
	t.Helper()
	ctx := context.Background()
	at := dreamT0.Add(48 * time.Hour)
	if ok, err := store.InsertRunningDream(ctx, newDream("d4", "c3", at)); err != nil || !ok {
		t.Fatalf("running insert = %v %v", ok, err)
	}
	cost := int64(420)
	fin := newDream("d4", "c3", at)
	fin.Status, fin.TurnIDs, fin.Considered, fin.CostSubcents, fin.Model = DreamPartial, []string{"t1", "t2"}, []string{"raise it"}, &cost, "m2"
	items := []DreamItem{
		{ID: "i1", Position: 0, Kind: "note_add", Text: "a", CitedTurnIDs: []string{"t1", "t2"}, Gate: "pass", Verdict: "improvement"},
		{ID: "i2", Position: 1, Kind: "context_diff", Text: "b", CitedTurnIDs: []string{"t1"}, Gate: "thin_evidence"},
	}
	if ok, err := store.FinishDream(ctx, fin, items, at.Add(time.Minute)); err != nil || !ok {
		t.Fatalf("finish = %v %v", ok, err)
	}
	if ok, _ := store.FailDream(ctx, "d4", "x", at); ok {
		t.Fatal("a finished row must not change again")
	}
	got, err := store.GetDream(ctx, "c3", "d4")
	if err != nil || got.Status != DreamPartial || got.CostSubcents == nil || *got.CostSubcents != 420 || len(got.TurnIDs) != 2 || got.Considered[0] != "raise it" {
		t.Fatalf("get = %+v %v", got, err)
	}
	if _, err := store.GetDream(ctx, "other", "d4"); err != ErrDreamNotFound {
		t.Fatalf("foreign get = %v, want not found", err)
	}
	last, _ := store.LastAcceptedDream(ctx, "c3")
	if last == nil || last.ID != "d4" {
		t.Fatalf("last accepted = %+v", last)
	}

	// Ratings: one per item and manager, newest across managers wins, ties by user id.
	if err := store.RateDreamItem(ctx, "c3", "d4", "i1", "u1", "useful"); err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return at.Add(time.Hour) }
	if err := store.RateDreamItem(ctx, "c3", "d4", "i1", "u2", "harmful"); err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return at.Add(2 * time.Hour) }
	if err := store.RateDreamItem(ctx, "c3", "d4", "i1", "u1", "not_useful"); err != nil {
		t.Fatal(err)
	}
	its, _ := store.ListDreamItems(ctx, "d4")
	if len(its) != 2 || its[0].Rating != "not_useful" || its[1].Rating != "" {
		t.Fatalf("ratings = %+v", its)
	}
	store.now = func() time.Time { return at.Add(2 * time.Hour) }
	if err := store.RateDreamItem(ctx, "c3", "d4", "i1", "u0", "useful"); err != nil {
		t.Fatal(err)
	}
	if its, _ = store.ListDreamItems(ctx, "d4"); its[0].Rating != "useful" {
		t.Fatalf("tie must go to the smaller user id, got %q", its[0].Rating)
	}
	if err := store.RateDreamItem(ctx, "c3", "other-dream", "i1", "u1", "useful"); err != ErrDreamNotFound {
		t.Fatalf("item of another dream = %v, want not found", err)
	}
	if err := store.RateDreamItem(ctx, "wrong", "d4", "i1", "u1", "useful"); err != ErrDreamNotFound {
		t.Fatalf("foreign coordinator = %v, want not found", err)
	}

	// Agreement counts a rated item with a measured verdict only.
	if err := store.RateDreamItem(ctx, "c3", "d4", "i2", "u1", "useful"); err != nil {
		t.Fatal(err)
	}
	rated, agreeing, err := store.DreamAgreement(ctx, "c3", at.Add(-time.Hour), at.Add(time.Hour))
	if err != nil || rated != 1 || agreeing != 1 {
		t.Fatalf("agreement = %d/%d %v (i1 improvement+useful agrees; i2 has no verdict)", rated, agreeing, err)
	}

	// Skipped rows: one per hash, hidden from the list.
	skip := newDream("d5", "c3", at.Add(time.Hour))
	if ok, _ := store.InsertSkippedDream(ctx, skip); !ok {
		t.Fatal("first skip must store")
	}
	skip.ID = "d6"
	if ok, _ := store.InsertSkippedDream(ctx, skip); ok {
		t.Fatal("second skip with the same hash must not store")
	}
	list, _ := store.ListDreams(ctx, "c3", nil, 20)
	if len(list) != 1 || list[0].ID != "d4" {
		t.Fatalf("list = %+v", list)
	}
	if ok, _ := store.AcceptedDreamWithHash(ctx, "c3", "h-d4"); !ok {
		t.Fatal("accepted hash not found")
	}

	// Cursor ordering: newest first, ties by id descending.
	for _, id := range []string{"e1", "e2"} {
		if ok, _ := store.InsertRunningDream(ctx, newDream(id, "c4", at)); !ok {
			t.Fatal(id)
		}
		f := newDream(id, "c4", at)
		f.Status = DreamOK
		if ok, err := store.FinishDream(ctx, f, nil, at); err != nil || !ok {
			t.Fatal(id, err)
		}
	}
	page, _ := store.ListDreams(ctx, "c4", nil, 1)
	if page[0].ID != "e2" {
		t.Fatalf("first page = %s, want e2", page[0].ID)
	}
	page, _ = store.ListDreams(ctx, "c4", &DreamCursor{StartedAt: at, ID: "e2"}, 1)
	if len(page) != 1 || page[0].ID != "e1" {
		t.Fatalf("second page = %+v", page)
	}

	// A dream's episode task resolves to its coordinator, like a conversation task.
	if id, ok, err := store.CoordinatorForConversationTask(ctx, "task-1"); err != nil || !ok || id != "c2" {
		t.Fatalf("episode task lookup = %q %v %v", id, ok, err)
	}
	if isDream, err := store.IsDreamEpisodeTask(ctx, "task-1"); err != nil || !isDream {
		t.Fatalf("IsDreamEpisodeTask = %v %v", isDream, err)
	}
	if isDream, _ := store.IsDreamEpisodeTask(ctx, "other-task"); isDream {
		t.Fatal("an unrelated task is not a dream task")
	}

	// Evidence reads: the oldest turn start scans as a time on both dialects.
	if first, err := store.FirstLedgerTurnAt(ctx, "c5"); err != nil || first != nil {
		t.Fatalf("first turn of an empty ledger = %v %v", first, err)
	}
	for i, id := range []string{"lt2", "lt1"} {
		if _, err := store.db.ExecContext(ctx, store.db.Rebind(`INSERT INTO coordinator_turns
			(id, coordinator_id, session_id, session_turn_id, "trigger", started_at, finished_at) VALUES (?, 'c5', 's', ?, 'wake', ?, ?)`),
			id, id, at.Add(time.Duration(-i)*time.Hour), at); err != nil {
			t.Fatal(err)
		}
	}
	if first, err := store.FirstLedgerTurnAt(ctx, "c5"); err != nil || first == nil || !first.Equal(at.Add(-time.Hour)) {
		t.Fatalf("first turn = %v %v, want %v", first, err, at.Add(-time.Hour))
	}

	if n, err := store.PruneDreams(ctx, at.Add(24*time.Hour), 100); err != nil || n < 2 {
		t.Fatalf("prune = %d %v", n, err)
	}
	if _, err := store.GetDream(ctx, "c3", "d4"); err != ErrDreamNotFound {
		t.Fatal("pruned dream still readable")
	}
}

func TestDreamStore_SQLite(t *testing.T) { runDreamStoreConformance(t, newTestStore(t)) }

func TestDreamStore_Postgres(t *testing.T) { runDreamStoreConformance(t, newTestStorePostgres(t)) }

func TestDreamAgrees(t *testing.T) {
	for _, c := range []struct {
		verdict, rating string
		want            bool
	}{
		{"improvement", "useful", true}, {"improvement", "not_useful", false}, {"improvement", "harmful", false},
		{"blocked", "not_useful", true}, {"blocked", "harmful", true}, {"blocked", "useful", false},
		{"not_an_improvement", "harmful", true}, {"not_an_improvement", "useful", false},
	} {
		if got := DreamAgrees(c.verdict, c.rating); got != c.want {
			t.Errorf("DreamAgrees(%s,%s) = %v", c.verdict, c.rating, got)
		}
	}
}
