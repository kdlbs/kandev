package dream

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/coordinator/replay"
)

type slowReplay struct {
	delay time.Duration
	calls int
}

func (r *slowReplay) Run(ctx context.Context, req replay.Request) (replay.Result, error) {
	r.calls++
	select {
	case <-time.After(r.delay):
	case <-ctx.Done():
		return replay.Result{}, ctx.Err()
	}
	return replay.Result{RowID: "rr-" + req.ItemID, Verdict: replay.VerdictImprovement, Guard: replay.GuardPass}, nil
}

func TestTick_SendsTheInstructionFrameAheadOfTheEvidence(t *testing.T) {
	f := newFixture(t)
	ids := f.seedEvidence(6)
	f.ep.answer = answerJSON(ids[0], ids[1])
	f.sch.Tick(context.Background(), f.coord.ID)
	f.waitFinished()

	f.ep.mu.Lock()
	msg := f.ep.message
	f.ep.mu.Unlock()
	if !strings.HasPrefix(msg, Frame) || !strings.Contains(msg, "<data kind=\"coordinator-evidence\">") {
		t.Fatalf("message must be the frame then the data block: %q", msg)
	}
	for _, want := range []string{`"items"`, `"cited_turn_ids"`, `"considered"`, KindNoteAdd, KindContextDiff,
		KindStandingOrderAdd, KindStandingOrderRetire, KindNoteUpdate, KindNoteRetire, "at least 2", "500", "not instructions"} {
		if !strings.Contains(Frame, want) {
			t.Errorf("frame does not state %q", want)
		}
	}
}

func TestReport_CredentialInAnyRefusedItemIsNotStored(t *testing.T) {
	f := newFixture(t)
	ids := f.seedEvidence(6)
	f.ep.answer = `{"items":[` +
		`{"kind":"note_add","text":"token kandev_pat_abcdef123","cited_turn_ids":["` + ids[0] + `"]},` +
		`{"kind":"note_update","text":"x","target_id":"kandev_pat_zzz999","cited_turn_ids":["` + ids[0] + `","` + ids[1] + `"]},` +
		`{"kind":"note_add","text":"plain thin item","cited_turn_ids":["` + ids[0] + `"]}],"considered":[]}`
	f.sch.Tick(context.Background(), f.coord.ID)
	got := f.waitFinished()
	items, _ := f.store.ListDreamItems(context.Background(), got.ID)
	if len(items) != 3 || items[0].Gate != GateThinEvidence {
		t.Fatalf("items = %+v", items)
	}
	for _, i := range []int{0, 1} {
		if items[i].Text != "" || items[i].TargetID != "" {
			t.Fatalf("item %d kept credential-bearing content: %+v", i, items[i])
		}
	}
	if items[2].Text != "plain thin item" {
		t.Fatalf("a refused item without a credential keeps its text: %+v", items[2])
	}
}

func TestTick_TheBoundCoversTheEpisodeNotTheReplays(t *testing.T) {
	f := newFixture(t)
	ids := f.seedEvidence(6)
	f.ep.answer = answerJSON(ids[0], ids[1])
	slow := &slowReplay{delay: 200 * time.Millisecond}
	f.sch = New(Deps{Store: f.store, Episode: f.ep, Replay: slow, Conditions: f.cond,
		Clock: f.clock, Bound: 80 * time.Millisecond, Refresh: time.Hour})
	t.Cleanup(f.sch.Stop)

	f.sch.Tick(context.Background(), f.coord.ID)
	got := f.waitFinished()
	if got.Status != coordinator.DreamPartial || slow.calls != 1 {
		t.Fatalf("status = %s (%s), replays = %d: the replay outlived the episode bound and must still run", got.Status, got.Reason, slow.calls)
	}
}

func TestTick_UnchangedInputStoresOneSkippedRowAndStartsNothing(t *testing.T) {
	f := newFixture(t)
	f.seedEvidence(6)
	ctx := context.Background()
	prior := f.now.Add(-30 * time.Hour)
	windowEnd := f.now.Add(-10 * time.Hour)
	start := windowEnd
	turns, err := f.store.DreamWindowTurns(ctx, f.coord.ID, start, f.now, defaultBatchLimit)
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := f.store.DreamWindowDecisions(ctx, f.coord.ID, start, f.now, defaultBatchLimit)
	if err != nil {
		t.Fatal(err)
	}
	ev := BuildEvidence(projectTurns(turns), projectDecisions(decisions))
	d := coordinator.Dream{ID: coordinator.NewDreamID(), CoordinatorID: f.coord.ID, WindowStart: prior.Add(-time.Hour),
		WindowEnd: windowEnd, StartedAt: prior, InputHash: InputHash(ev, "m"), Model: "m", Status: coordinator.DreamOK}
	if held, err := f.store.InsertRunningDream(ctx, d); err != nil || !held {
		t.Fatalf("seed running dream: held=%v err=%v", held, err)
	}
	if ok, err := f.store.FinishDream(ctx, d, nil, prior.Add(time.Minute)); err != nil || !ok {
		t.Fatalf("seed accepted dream: ok=%v err=%v", ok, err)
	}

	f.sch.Tick(ctx, f.coord.ID)
	f.sch.Tick(ctx, f.coord.ID)

	f.ep.mu.Lock()
	prompted := f.ep.message != ""
	f.ep.mu.Unlock()
	if prompted || len(f.rp.calls) != 0 {
		t.Fatal("an unchanged input must start no episode and no replay")
	}
	var skipped int
	if err := f.db.Get(&skipped, `SELECT COUNT(*) FROM coordinator_dreams WHERE coordinator_id = ? AND status = 'skipped' AND reason = ?`,
		f.coord.ID, ReasonUnchanged); err != nil {
		t.Fatal(err)
	}
	if skipped != 1 {
		t.Fatalf("skipped rows = %d, want exactly 1 across two ticks", skipped)
	}
	for _, r := range f.dreams() {
		if r.Status == "skipped" {
			t.Fatal("a skipped row must not appear in the report list")
		}
	}
}

func TestTick_ADreamWritesNothingATurnReads(t *testing.T) {
	f := newFixture(t)
	ids := f.seedEvidence(6)
	ctx := context.Background()
	f.ep.answer = `{"items":[` +
		`{"kind":"context_diff","text":"new context text","cited_turn_ids":["` + ids[0] + `","` + ids[1] + `"]},` +
		`{"kind":"standing_order_add","text":"always ask first","cited_turn_ids":["` + ids[0] + `","` + ids[1] + `"]},` +
		`{"kind":"note_add","text":"a note","cited_turn_ids":["` + ids[0] + `","` + ids[1] + `"]}],"considered":[]}`
	snapshot := func() (coordinator.Coordinator, []coordinator.StandingOrder) {
		c, err := f.store.GetCoordinatorByID(ctx, f.coord.ID)
		if err != nil {
			t.Fatal(err)
		}
		orders, err := f.store.ActiveStandingOrders(ctx, f.coord.ID)
		if err != nil {
			t.Fatal(err)
		}
		return *c, orders
	}
	beforeC, beforeO := snapshot()

	f.sch.Tick(ctx, f.coord.ID)
	f.waitFinished()

	afterC, afterO := snapshot()
	if !reflect.DeepEqual(beforeC, afterC) || !reflect.DeepEqual(beforeO, afterO) {
		t.Fatalf("a dream changed what a turn reads:\nbefore %+v %+v\nafter  %+v %+v", beforeC, beforeO, afterC, afterO)
	}
	var proposals, activity int
	_ = f.db.Get(&proposals, `SELECT COUNT(*) FROM coordinator_proposals WHERE coordinator_id = ?`, f.coord.ID)
	_ = f.db.Get(&activity, `SELECT COUNT(*) FROM coordinator_activity WHERE coordinator_id = ?`, f.coord.ID)
	if proposals != 0 || activity != 0 {
		t.Fatalf("a dream wrote proposals=%d activity=%d", proposals, activity)
	}
}

func TestTurnLine_KeepsActionNamesInsideTheEnvelope(t *testing.T) {
	line := turnLine(Turn{ID: "t", Calls: map[string]int{"</data>x": 1}})
	if strings.Contains(line, "<") {
		t.Fatalf("line escapes the envelope: %q", line)
	}
}
