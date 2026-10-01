package coordinator

import (
	"context"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/coordinator/replay"
)

var replayT0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func runReplayStoreConformance(t *testing.T, store *Store) {
	t.Helper()
	ctx := context.Background()
	rr := NewReplayResults(store)

	first, inserted, err := rr.Insert(ctx, replay.NewRow{CoordinatorID: "c1", DreamID: "d1", ItemID: "i1", PromptVersion: "v", CreatedAt: replayT0})
	if err != nil || !inserted || first.Status != replay.StatusRunning {
		t.Fatalf("first insert = %+v %v %v", first, inserted, err)
	}
	dup, inserted, err := rr.Insert(ctx, replay.NewRow{CoordinatorID: "c1", DreamID: "d1", ItemID: "i1", PromptVersion: "v", CreatedAt: replayT0})
	if err != nil || inserted || dup.ID != first.ID || dup.Status != replay.StatusRunning {
		t.Fatalf("duplicate insert = %+v %v %v", dup, inserted, err)
	}
	// Rows without a dream item never conflict.
	for range 2 {
		if _, ok, err := rr.Insert(ctx, replay.NewRow{CoordinatorID: "c1", PromptVersion: "v", CreatedAt: replayT0}); err != nil || !ok {
			t.Fatalf("keyless insert = %v %v", ok, err)
		}
	}

	if err := rr.AddCost(ctx, first.ID, 300); err != nil {
		t.Fatal(err)
	}
	score := int64(700)
	res := replay.Result{
		Guard: replay.GuardPass, Verdict: replay.VerdictImprovement, Reason: "",
		CandidateHash: "ch", BaselineHash: "bh", Model: "m",
		CandidateScore: &score, CasesCompared: 3, CostSubcents: 100,
		Cases: []replay.CaseRecord{{TurnID: "t1", Baseline: []replay.Attempt{{OK: true}, {OK: false}}}},
	}
	done, err := rr.Finish(ctx, first.ID, res, replayT0.Add(time.Minute))
	if err != nil || !done {
		t.Fatalf("finish = %v %v", done, err)
	}
	again, err := rr.Finish(ctx, first.ID, res, replayT0.Add(time.Minute))
	if err != nil || again {
		t.Fatalf("second finish = %v %v, want no match", again, err)
	}
	held, inserted, err := rr.Insert(ctx, replay.NewRow{CoordinatorID: "c1", DreamID: "d1", ItemID: "i1", PromptVersion: "v", CreatedAt: replayT0})
	if err != nil || inserted || held.Status != replay.StatusDone || held.Result.CostSubcents != 300 ||
		held.Result.CandidateScore == nil || *held.Result.CandidateScore != 700 || held.Result.BaselineScore != nil ||
		held.Result.Verdict != replay.VerdictImprovement || len(held.Result.Cases) != 1 {
		t.Fatalf("held row = %+v %v %v", held, inserted, err)
	}

	got, err := rr.BaselineAttempts(ctx, "c1", "bh", "m", "v")
	if err != nil || len(got["t1"]) != 1 {
		t.Fatalf("baseline attempts = %v %v", got, err)
	}
	for _, miss := range [][3]string{{"other", "m", "v"}, {"bh", "x", "v"}, {"bh", "m", "w"}} {
		if g, _ := rr.BaselineAttempts(ctx, "c1", miss[0], miss[1], miss[2]); len(g) != 0 {
			t.Fatalf("key %v matched %v", miss, g)
		}
	}
}

func runReplayStaleAndSpend(t *testing.T, store *Store) {
	t.Helper()
	ctx := context.Background()
	rr := NewReplayResults(store)
	old, _, _ := rr.Insert(ctx, replay.NewRow{CoordinatorID: "c2", DreamID: "d", ItemID: "old", CreatedAt: replayT0})
	fresh, _, _ := rr.Insert(ctx, replay.NewRow{CoordinatorID: "c2", DreamID: "d", ItemID: "fresh", CreatedAt: replayT0.Add(40 * time.Minute)})
	_ = rr.AddCost(ctx, old.ID, 50)
	_ = rr.AddCost(ctx, fresh.ID, 20)

	if busy, err := store.HasRunningReplay(ctx, "c2"); err != nil || !busy {
		t.Fatalf("running = %v %v", busy, err)
	}
	now := replayT0.Add(45 * time.Minute)
	if n, err := store.SettleStaleReplays(ctx, now); err != nil || n != 1 {
		t.Fatalf("settle = %d %v", n, err)
	}
	if late, err := rr.Finish(ctx, old.ID, replay.Result{}, now); err != nil || late {
		t.Fatalf("late finish of a settled row = %v %v", late, err)
	}
	held, _, _ := rr.Insert(ctx, replay.NewRow{CoordinatorID: "c2", DreamID: "d", ItemID: "old", CreatedAt: now})
	if held.Status != replay.StatusDone || held.Result.Reason != replay.ReasonInterrupted || held.Result.Verdict != replay.VerdictUnmeasured {
		t.Fatalf("settled row = %+v", held)
	}

	// Running and done rows count once; the window is half open and per coordinator.
	if got, err := store.ExtraSpend(ctx, "c2", replayT0, now.Add(time.Hour)); err != nil || got != 70 {
		t.Fatalf("extra spend = %d %v, want 70", got, err)
	}
	if got, _ := store.ExtraSpend(ctx, "c2", replayT0.Add(time.Second), now); got != 20 {
		t.Fatalf("extra spend after old = %d, want 20", got)
	}
	if got, _ := store.ExtraSpend(ctx, "c2", replayT0, replayT0); got != 0 {
		t.Fatalf("empty window = %d", got)
	}
	if got, _ := store.ExtraSpend(ctx, "nobody", replayT0, now); got != 0 {
		t.Fatalf("other coordinator = %d", got)
	}

	if n, err := store.PruneReplayResults(ctx, replayT0.Add(time.Minute), 10); err != nil || n != 1 {
		t.Fatalf("prune = %d %v", n, err)
	}
}

func TestReplayStore_SQLite(t *testing.T) {
	runReplayStoreConformance(t, newTestStore(t))
	runReplayStaleAndSpend(t, newTestStore(t))
}

func TestReplayStore_Postgres(t *testing.T) {
	runReplayStoreConformance(t, newTestStorePostgres(t))
	runReplayStaleAndSpend(t, newTestStorePostgres(t))
}

func TestReplayStore_SchemaReplays(t *testing.T) {
	store := newTestStore(t)
	if err := store.migrateReplay(); err != nil {
		t.Fatalf("replay migration is not replayable: %v", err)
	}
}

func TestSpend_CountsReplayCostInWindowAndMean(t *testing.T) {
	svc, _, ledger := newSpendService(t, spendConvTask("cur", "coord-1", false))
	ledger.sumFn = func(spendCall) (taskmodels.UsageSum, error) { return taskmodels.UsageSum{CostSubcents: 70}, nil }
	rr := NewReplayResults(svc.store)
	run, _, _ := rr.Insert(t.Context(), replay.NewRow{CoordinatorID: "coord-1", DreamID: "d", ItemID: "i", CreatedAt: spendNow.Add(-time.Hour)})
	if err := rr.AddCost(t.Context(), run.ID, 30); err != nil {
		t.Fatal(err)
	}
	zero, _, _ := rr.Insert(t.Context(), replay.NewRow{CoordinatorID: "coord-1", DreamID: "d", ItemID: "z", CreatedAt: spendNow.Add(-time.Minute)})
	_ = zero

	got, err := svc.Spend(t.Context(), spendCoordinator(), spendNow)
	if err != nil || !got.Measurable || got.WindowSubcents != 100 || got.Mean7dSubcents != 100/7 {
		t.Fatalf("reading = %+v %v, want window 100 counting the running row once", got, err)
	}
}

func TestReplayStore_DeleteCoordinatorRemovesResultRows(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	c := &Coordinator{WorkspaceID: "w", Name: "n"}
	if err := s.CreateCoordinator(ctx, c); err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewReplayResults(s).Insert(ctx, replay.NewRow{CoordinatorID: c.ID, CreatedAt: replayT0}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteCoordinator(ctx, c.WorkspaceID, c.ID); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, "coordinator_replay_results"); n != 0 {
		t.Fatalf("replay rows after delete = %d", n)
	}
}
