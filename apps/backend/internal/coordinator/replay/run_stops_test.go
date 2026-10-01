package replay

import (
	"context"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func fewCases(w *world, n int) {
	for i := 1; i <= n; i++ {
		w.addCase("t"+string(rune('a'+i-1)), caseProp{"good" + string(rune('a'+i-1)), DecisionApproved})
	}
	w.answer = func(prompt string) (Reply, error) {
		return Reply{Text: `[{"kind":"move","target_task_id":"x"}]`, PromptTokens: 100, ResponseTokens: 10}, nil
	}
}

func TestBudgetUnmeasurableStopsBeforeAnyCall(t *testing.T) {
	for name, set := range map[string]func(*world){
		"not measurable": func(w *world) { w.spend.Measurable = false },
		"read error":     func(w *world) { w.spendErr = errBoom },
	} {
		t.Run(name, func(t *testing.T) {
			w := newWorld()
			fewCases(w, 3)
			set(w)
			res := mustRun(t, w, req())
			if res.Reason != ReasonBudget || res.Verdict != VerdictUnmeasured || w.calls != 0 || res.CandidateScore != nil {
				t.Fatalf("got %+v calls %d", res, w.calls)
			}
		})
	}
}

func TestBudgetCeilingStopsAPrefixAndKeepsSpend(t *testing.T) {
	w := newWorld()
	fewCases(w, 5)
	w.pricing.OutputPerMillion = 1000 // a call's bound is about 140 subcents, its real cost 100
	ceiling := int64(1000)
	w.spend.CeilingSubcents = &ceiling
	res := mustRun(t, w, req())
	if res.Reason != ReasonBudget || res.Guard != GuardUnmeasured || res.Verdict != VerdictUnmeasured {
		t.Fatalf("got %s/%s/%s", res.Guard, res.Verdict, res.Reason)
	}
	if res.CandidateScore != nil || res.HeldOutCandidateScore != nil || res.CostSubcents != w.totalAdded() {
		t.Fatalf("scores must be nil and the cost kept: %+v", res)
	}
	if w.calls == 0 || w.calls >= 30 {
		t.Fatalf("expected a prefix of the calls, got %d", w.calls)
	}
}

func TestNoCeilingAdmitsEveryRun(t *testing.T) {
	w := newWorld()
	fewCases(w, 3)
	w.spend.WindowSubcents = math.MaxInt64 / 2
	if res := mustRun(t, w, req()); res.Reason == ReasonBudget {
		t.Fatalf("no ceiling must admit: %+v", res)
	}
}

func TestConcurrencyIsBounded(t *testing.T) {
	w := newWorld()
	fewCases(w, 8)
	var cur, peak atomic.Int64
	reached, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	w.answer = func(string) (Reply, error) {
		n := cur.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		if n == Concurrency {
			once.Do(func() { close(reached) })
		}
		<-release
		cur.Add(-1)
		return Reply{Text: "[]", PromptTokens: 1, ResponseTokens: 1}, nil
	}
	go func() {
		<-reached
		close(release)
	}()
	mustRun(t, w, req())
	if peak.Load() != Concurrency {
		t.Fatalf("peak concurrency %d, want %d", peak.Load(), Concurrency)
	}
}

func TestBaselineAttemptsAreReusedWithTheirIndexes(t *testing.T) {
	w := newWorld()
	fewCases(w, 1)
	w.baseline["ta"] = []Attempt{
		{Index: 1, OK: true, Keys: []string{"move|x"}},
		{Index: 2},
		{Index: 3, OK: true, Keys: []string{"move|x"}},
	}
	res := mustRun(t, w, req())
	if w.calls != 3+1 {
		t.Fatalf("calls %d: three candidate attempts and one missing baseline attempt", w.calls)
	}
	var idx []int
	for _, a := range res.Cases[0].Baseline {
		idx = append(idx, a.Index)
	}
	if len(idx) != 3 || idx[0] != 1 || idx[1] != 2 || idx[2] != 3 {
		t.Fatalf("baseline attempt indexes %v: the rerun must take the index of the failed one", idx)
	}
}

func TestFullBaselineMakesNoBaselineCalls(t *testing.T) {
	w := newWorld()
	fewCases(w, 1)
	w.baseline["ta"] = []Attempt{{Index: 1, OK: true}, {Index: 2, OK: true}, {Index: 3, OK: true}}
	mustRun(t, w, req())
	if w.calls != 3 {
		t.Fatalf("calls %d", w.calls)
	}
}

func TestZeroTokenRunIsPricedFromEstimates(t *testing.T) {
	w := newWorld()
	fewCases(w, 1)
	w.answer = func(string) (Reply, error) { return Reply{Text: `[{"kind":"move","target_task_id":"x"}]`}, nil }
	res := mustRun(t, w, req())
	// 1 subcent per token: each call costs promptBytes/3 + responseBytes/3.
	var want int64
	for _, p := range w.prompts {
		want += estimateTokens(len(p)) + estimateTokens(len(`[{"kind":"move","target_task_id":"x"}]`))
	}
	if res.CostSubcents != want || want == 0 {
		t.Fatalf("cost %d want %d", res.CostSubcents, want)
	}
}

func TestReportedTokensArePricedAsReported(t *testing.T) {
	w := newWorld()
	fewCases(w, 1)
	res := mustRun(t, w, req())
	if res.CostSubcents != 6*110 {
		t.Fatalf("cost %d", res.CostSubcents)
	}
}

func TestPricingOverflowExhaustsTheCeiling(t *testing.T) {
	w := newWorld()
	fewCases(w, 2)
	w.pricing.OutputPerMillion = math.MaxInt64 / 2
	w.pricing.InputPerMillion = 1
	ceiling := int64(math.MaxInt64 - 1)
	w.spend.CeilingSubcents = &ceiling
	res := mustRun(t, w, req())
	if res.Reason != ReasonBudget {
		t.Fatalf("got %+v", res)
	}
}

func TestCostUpdateFailureKeepsCostInTheReadingAndCarries(t *testing.T) {
	w := newWorld()
	fewCases(w, 2)
	w.addCostErr = errBoom
	res := mustRun(t, w, req())
	if !contains(joinNotes(res.Notes), "cost_update_failed") {
		t.Fatalf("notes %v", res.Notes)
	}
	if res.CostSubcents != 12*110 {
		t.Fatalf("in-memory total must still be reported: %d", res.CostSubcents)
	}
}

func joinNotes(n []string) string {
	out := ""
	for _, s := range n {
		out += s + ","
	}
	return out
}

func TestBudgetCountsUnwrittenCost(t *testing.T) {
	b := &budget{spend: &world{spend: SpendReading{Measurable: true, CeilingSubcents: ptr(1000)}}, clock: &world{}}
	if err := b.reserve(context.Background(), 400); err != nil {
		t.Fatal(err)
	}
	b.settle(400, 300, 0, false) // write failed: 300 stays unwritten
	if err := b.reserve(context.Background(), 701); !errors.Is(err, errBudget) {
		t.Fatalf("300 unwritten + 600 + 0 must not reach... got %v", err)
	}
	if err := b.reserve(context.Background(), 699); err != nil {
		t.Fatalf("300+699 < 1000 admits: %v", err)
	}
	b.settle(699, 10, 300, true) // a write of 310 succeeded, carrying the 300
	if b.unwrittenNow() != 0 {
		t.Fatalf("unwritten %d", b.unwrittenNow())
	}
}

func ptr(v int64) *int64 { return &v }

func TestFinalWriteFailureReturnsErrResultNotStored(t *testing.T) {
	w := newWorld()
	fewCases(w, 2)
	w.finishErr = errBoom
	res, err := w.harness().Run(context.Background(), req())
	if !errors.Is(err, ErrResultNotStored) || res.RowID == "" || res.CasesCompared != 2 {
		t.Fatalf("got %+v %v", res, err)
	}
}

func TestFinalWriteMatchingNoRowReturnsErrResultNotStored(t *testing.T) {
	w := newWorld()
	fewCases(w, 2)
	w.answer = func(string) (Reply, error) {
		w.mu.Lock()
		for _, r := range w.rows {
			r.Status = StatusDone // settled stale while the replay ran
		}
		w.mu.Unlock()
		return Reply{Text: "[]"}, nil
	}
	if _, err := w.harness().Run(context.Background(), req()); !errors.Is(err, ErrResultNotStored) {
		t.Fatalf("err %v", err)
	}
}

func TestCancelledReplayKeepsSpendAndStoresCancelled(t *testing.T) {
	w := newWorld()
	fewCases(w, 5)
	ctx, cancel := context.WithCancel(context.Background())
	var once sync.Once
	w.answer = func(string) (Reply, error) {
		once.Do(cancel)
		return Reply{Text: "[]", PromptTokens: 10, ResponseTokens: 1}, nil
	}
	res, err := w.harness().Run(ctx, req())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != ReasonCancelled || res.Verdict != VerdictUnmeasured || res.CandidateScore != nil {
		t.Fatalf("got %+v", res)
	}
	if w.rows[res.RowID].Status != StatusDone || res.CostSubcents == 0 || res.CostSubcents != w.totalAdded() {
		t.Fatalf("the spend must be kept and the row finished: %+v", res)
	}
}

func TestCancelWinsOnlyWhenFirst(t *testing.T) {
	w := newWorld()
	fewCases(w, 5)
	ceiling := int64(1)
	w.spend.CeilingSubcents = &ceiling
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, _ := w.harness().Run(ctx, req())
	if res.Reason != ReasonCancelled && res.Reason != ReasonBudget {
		t.Fatalf("reason %q", res.Reason)
	}
}

func TestTimeBoundStopsBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := newWorld()
		fewCases(w, 3)
		w.answer = func(string) (Reply, error) {
			time.Sleep(ReplayTimeout) // every call outlasts the replay bound
			return Reply{Text: "[]"}, nil
		}
		w.profile.Model = "model-x"
		res, err := w.harness().Run(context.Background(), req())
		if err != nil || res.Reason != ReasonBudget || res.Verdict != VerdictUnmeasured || res.CandidateScore != nil {
			t.Fatalf("got %+v %v", res, err)
		}
	})
}

func TestRunTimeoutFailsTheAttemptNotTheReplay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := newWorld()
		fewCases(w, 1)
		var n atomic.Int64
		w.answer = func(string) (Reply, error) {
			if n.Add(1) == 1 {
				time.Sleep(RunTimeout + time.Second)
				return Reply{Text: "[]"}, nil // too late: the context is done
			}
			return Reply{Text: "[]", PromptTokens: 1, ResponseTokens: 1}, nil
		}
		res, err := w.harness().Run(context.Background(), req())
		if err != nil || res.Reason == ReasonBudget {
			t.Fatalf("got %+v %v", res, err)
		}
	})
}
