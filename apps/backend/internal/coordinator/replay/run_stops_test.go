package replay

import (
	"context"
	"errors"
	"math"
	"strings"
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

func TestBudgetStopStoresTheFlipsAndAttemptsFoundSoFar(t *testing.T) {
	w := newWorld()
	fewCases(w, 5)
	w.answer = scripted(func(side, id string) []string {
		if side == "cand" && id == "ta" {
			return nil // drops the approval the baseline reproduces
		}
		return []string{"move|good" + id[1:]}
	})
	w.pricing.OutputPerMillion = 1000
	ceiling := int64(1500)
	w.spend.CeilingSubcents = &ceiling
	res := mustRun(t, w, req())
	if res.Reason != ReasonBudget || res.CandidateScore != nil {
		t.Fatalf("got %+v", res)
	}
	if len(res.Flips) != 1 || res.Flips[0] != (Flip{TurnID: "ta", ProposalID: "ta-p1"}) {
		t.Fatalf("flips %v", res.Flips)
	}
	var rec *CaseRecord
	for i := range res.Cases {
		if res.Cases[i].TurnID == "ta" {
			rec = &res.Cases[i]
		}
	}
	if rec == nil || len(rec.Candidate) != 3 || len(rec.Baseline) != 3 {
		t.Fatalf("completed attempts not kept: %+v", rec)
	}
	for _, a := range rec.Candidate {
		if !a.OK || len(a.Keys) != 0 {
			t.Fatalf("candidate attempt %+v", a)
		}
	}
	if stored := w.rows[res.RowID].Result.Cases; len(stored) != len(res.Cases) {
		t.Fatalf("stored %d case records, result has %d", len(stored), len(res.Cases))
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
	if !strings.Contains(joinNotes(res.Notes), "cost_update_failed") {
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
	if got := b.claimUnwritten(); got != 300 {
		t.Fatalf("claimed %d", got)
	}
	b.settle(699, 10, 300, true) // a write of 310 succeeded, carrying the 300
	if got := b.claimUnwritten(); got != 0 {
		t.Fatalf("unwritten %d", got)
	}
}

func TestConcurrentChargesWriteACarriedCostOnce(t *testing.T) {
	w := newWorld()
	b := &budget{spend: w, clock: w}
	r := &run{h: &Harness{d: Deps{Results: w}}, bud: b}
	w.addCostErr = errBoom
	if err := b.reserve(context.Background(), 5); err != nil {
		t.Fatal(err)
	}
	r.charge(5, 7) // write fails: 7 stays unwritten
	w.addCostErr = nil
	for i := 0; i < 2; i++ {
		if err := b.reserve(context.Background(), 5); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); r.charge(5, 10) }()
	}
	wg.Wait()
	var sum int64
	for _, c := range w.costAdds {
		sum += c
	}
	if sum != 27 {
		t.Fatalf("row charged %d, want 7+10+10 once each", sum)
	}
	if got := b.claimUnwritten(); got != 0 || b.inflight != 0 {
		t.Fatalf("unwritten %d inflight %d", got, b.inflight)
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
	if res.Reason != ReasonCancelled {
		t.Fatalf("a replay cancelled before its first call is cancelled, got %q", res.Reason)
	}

	live := newWorld()
	fewCases(live, 5)
	live.spend.CeilingSubcents = &ceiling
	res, _ = live.harness().Run(context.Background(), req())
	if res.Reason != ReasonBudget {
		t.Fatalf("a replay refused by the ceiling is budget, got %q", res.Reason)
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
		w.answer = func(prompt string) (Reply, error) {
			if strings.Contains(prompt, "CAND") && n.Add(1) == 1 {
				time.Sleep(RunTimeout + time.Second)
				return Reply{Text: "[]"}, nil // too late: the context is done
			}
			return Reply{Text: "[]", PromptTokens: 1, ResponseTokens: 1}, nil
		}
		res, err := w.harness().Run(context.Background(), req())
		if err != nil || res.Reason == ReasonBudget {
			t.Fatalf("got %+v %v", res, err)
		}
		if len(res.Cases) != 1 || len(res.Cases[0].Candidate) != 3 {
			t.Fatalf("cases %+v", res.Cases)
		}
		failed := 0
		for _, a := range res.Cases[0].Candidate {
			if !a.OK {
				failed++
			}
		}
		if failed != 1 {
			t.Fatalf("exactly the timed-out attempt must be failed: %+v", res.Cases[0].Candidate)
		}
	})
}
