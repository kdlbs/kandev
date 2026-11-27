package replay

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func req() Request {
	return Request{CoordinatorID: "c1", Candidate: Candidate{Kind: KindContextDiff, Text: "new context"}}
}

// twentyFive adds 25 cases that each approve one proposal and reject another.
func twentyFive(w *world) {
	for i := 1; i <= 25; i++ {
		w.addCase(fmt.Sprintf("t%02d", i),
			caseProp{fmt.Sprintf("good%02d", i), DecisionApproved},
			caseProp{fmt.Sprintf("bad%02d", i), DecisionRejected})
	}
}

func mustRun(t *testing.T, w *world, r Request) Result {
	t.Helper()
	res, err := w.harness().Run(context.Background(), r)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return res
}

func caseNum(id string) string { return id[1:] }

func TestRunImprovement(t *testing.T) {
	w := newWorld()
	twentyFive(w)
	w.answer = scripted(func(side, id string) []string {
		n := caseNum(id)
		if side == "cand" { // reproduces the approval, avoids the rejected one
			return []string{"move|good" + n}
		}
		return []string{"move|good" + n, "move|bad" + n} // baseline repeats the rejected one
	})
	res := mustRun(t, w, req())
	if res.Guard != GuardPass || res.Verdict != VerdictImprovement || res.Reason != "" {
		t.Fatalf("got %s/%s/%s", res.Guard, res.Verdict, res.Reason)
	}
	if *res.CandidateScore != 1000 || *res.BaselineScore != 500 {
		t.Fatalf("scores %d/%d", *res.CandidateScore, *res.BaselineScore)
	}
	if *res.HeldOutCandidateScore != 1000 || res.HeldOutCompared != 25 || res.CasesCompared != 25 {
		t.Fatalf("held out %d, compared %d", res.HeldOutCompared, res.CasesCompared)
	}
	if res.CasesRanCandidate != 25 || res.CasesRanBaseline != 25 || w.calls != 150 {
		t.Fatalf("ran %d/%d calls %d", res.CasesRanCandidate, res.CasesRanBaseline, w.calls)
	}
	if res.UnmatchedBaseline != 0 || res.CostSubcents == 0 || res.CostSubcents != w.totalAdded() {
		t.Fatalf("unmatched %d cost %d added %d", res.UnmatchedBaseline, res.CostSubcents, w.totalAdded())
	}
	if res.CandidateHash == "" || res.BaselineHash != "bh" || res.Model != "model-x" {
		t.Fatalf("hashes/model %+v", res)
	}
	if row := w.rows[res.RowID]; row.Status != StatusDone || row.Result.Verdict != VerdictImprovement {
		t.Fatalf("stored row %+v", row)
	}
}

func TestRunFlipBlocksWhateverTheScore(t *testing.T) {
	w := newWorld()
	twentyFive(w)
	w.answer = scripted(func(side, id string) []string {
		n := caseNum(id)
		if side == "cand" {
			if id == "t01" {
				return nil // drops the one approval, avoids everything else
			}
			return []string{"move|good" + n}
		}
		return []string{"move|good" + n, "move|bad" + n}
	})
	res := mustRun(t, w, req())
	if res.Guard != GuardBlocked || res.Verdict != VerdictNotAnImprovement {
		t.Fatalf("got %s/%s", res.Guard, res.Verdict)
	}
	if len(res.Flips) != 1 || res.Flips[0] != (Flip{TurnID: "t01", ProposalID: "t01-p1"}) {
		t.Fatalf("flips %v", res.Flips)
	}
	if res.HeldOutCandidateScore == nil || *res.HeldOutCandidateScore-*res.HeldOutBaselineScore < MinGainThousandths {
		t.Fatal("the guard-only gain must still be recorded")
	}
}

func TestRunTooFewHeldOut(t *testing.T) {
	w := newWorld()
	twentyFive(w)
	w.answer = scripted(func(side, id string) []string { return []string{"move|good" + caseNum(id)} })
	r := req()
	for i := 1; i <= 6; i++ {
		r.Candidate.CitedTurnIDs = append(r.Candidate.CitedTurnIDs, fmt.Sprintf("t%02d", i))
	}
	res := mustRun(t, w, r)
	if res.HeldOutCompared != 19 || res.Verdict != VerdictUnmeasured || res.Reason != ReasonTooFew || res.Guard != GuardPass {
		t.Fatalf("got %+v", res)
	}
	if res.CandidateScore == nil || res.HeldOutCandidateScore == nil {
		t.Fatal("too_few keeps every score")
	}
}

func TestRunDeterministic(t *testing.T) {
	var out [2]Result
	for i := range out {
		w := newWorld()
		twentyFive(w)
		w.answer = scripted(func(side, id string) []string {
			if side == "cand" && id < "t10" {
				return nil
			}
			return []string{"move|good" + caseNum(id)}
		})
		out[i] = mustRun(t, w, req())
	}
	a, b := out[0], out[1]
	if *a.CandidateScore != *b.CandidateScore || *a.BaselineScore != *b.BaselineScore || len(a.Flips) != len(b.Flips) || a.Verdict != b.Verdict {
		t.Fatalf("two replays differ: %+v vs %+v", a, b)
	}
	for i := range a.Flips {
		if a.Flips[i] != b.Flips[i] {
			t.Fatal("flips differ")
		}
	}
}

func TestRunIdempotentPerDreamItem(t *testing.T) {
	w := newWorld()
	twentyFive(w)
	r := req()
	r.DreamID, r.ItemID = "d1", "i1"
	first := mustRun(t, w, r)
	calls := w.calls
	r.Candidate.Text = "a different candidate"
	second, err := w.harness().Run(context.Background(), r)
	if err != nil || w.calls != calls {
		t.Fatalf("second run made calls or failed: %v %d/%d", err, w.calls, calls)
	}
	if second.RowID != first.RowID || second.Verdict != first.Verdict {
		t.Fatalf("second %+v first %+v", second, first)
	}
}

func TestRunRunningRowReportsRunning(t *testing.T) {
	w := newWorld()
	w.rows["pre"] = &Stored{ID: "pre", Status: StatusRunning}
	w.byKey["d1/i1"] = "pre"
	r := req()
	r.DreamID, r.ItemID = "d1", "i1"
	res, err := w.harness().Run(context.Background(), r)
	if !errors.Is(err, ErrReplayRunning) || res.RowID != "pre" || w.calls != 0 {
		t.Fatalf("got %+v %v calls %d", res, err, w.calls)
	}
}

func TestRunValidationErrorsWriteNoRow(t *testing.T) {
	cases := map[string]func(*Request){
		"empty coordinator":  func(r *Request) { r.CoordinatorID = "" },
		"unknown kind":       func(r *Request) { r.Candidate.Kind = "improvement" },
		"dream without item": func(r *Request) { r.DreamID = "d" },
		"item without dream": func(r *Request) { r.ItemID = "i" },
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			w := newWorld()
			r := req()
			mut(&r)
			if _, err := w.harness().Run(context.Background(), r); err == nil || len(w.rows) != 0 {
				t.Fatalf("err %v rows %d", err, len(w.rows))
			}
		})
	}
	h := New(Deps{})
	if _, err := h.Run(context.Background(), req()); err == nil {
		t.Fatal("nil dependency must error")
	}
}

func TestRunInsertFailureStartsNothing(t *testing.T) {
	w := newWorld()
	twentyFive(w)
	w.insertErr = errBoom
	if _, err := w.harness().Run(context.Background(), req()); err == nil || w.calls != 0 {
		t.Fatalf("err %v calls %d", err, w.calls)
	}
}

func TestRunEarlyStops(t *testing.T) {
	cases := []struct {
		name   string
		setup  func(*world)
		reason string
	}{
		{"auto approve", func(w *world) { w.profile.AutoApprove = true }, ReasonProfileUnsafe},
		{"command prefix", func(w *world) { w.profile.Prefix = "sudo" }, ReasonProfileUnsafe},
		{"enabled flag", func(w *world) { w.profile.EnabledFlags = []string{"--x"} }, ReasonProfileUnsafe},
		{"no price", func(w *world) { w.pricing = nil }, ReasonCostUnknown},
		{"price error", func(w *world) { w.priceErr = errBoom }, ReasonCostUnknown},
		{"empty model", func(w *world) { w.profile.Model = "" }, ReasonCostUnknown},
		{"no target", func(w *world) { w.renderErr = ErrNoTarget }, ReasonNoTarget},
		{"render error", func(w *world) { w.renderErr = errBoom }, ReasonReadFailed},
		{"profile error", func(w *world) { w.profileErr = errBoom }, ReasonReadFailed},
		{"select error", func(w *world) { w.selectErr = errBoom }, ReasonReadFailed},
		{"proposal read error", func(w *world) { w.proposalErr = errBoom }, ReasonReadFailed},
		{"baseline read error", func(w *world) { w.baselineErr = errBoom }, ReasonReadFailed},
		{"no cases", func(w *world) { w.turns = nil }, ReasonNoCases},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := newWorld()
			twentyFive(w)
			c.setup(w)
			res := mustRun(t, w, req())
			if res.Guard != GuardUnmeasured || res.Verdict != VerdictUnmeasured || res.Reason != c.reason {
				t.Fatalf("got %s/%s/%s", res.Guard, res.Verdict, res.Reason)
			}
			if res.CandidateScore != nil || res.HeldOutCandidateScore != nil {
				t.Fatal("an unmeasured replay has no scores")
			}
			if w.calls != 0 || res.CostSubcents != 0 {
				t.Fatalf("calls %d cost %d", w.calls, res.CostSubcents)
			}
			if w.rows[res.RowID].Status != StatusDone {
				t.Fatal("the row must be finished")
			}
		})
	}
}

func TestRunAllSkippedAndNoCompared(t *testing.T) {
	w := newWorld()
	w.addCase("a", caseProp{"x", DecisionReturned}) // returned: no expectation
	res := mustRun(t, w, req())
	if res.Reason != ReasonAllSkipped || len(res.Skipped) != 1 || res.Skipped[0] != (Skip{TurnID: "a", Reason: SkipNoExpectation}) {
		t.Fatalf("got %+v", res)
	}

	w = newWorld()
	twentyFive(w)
	w.answer = func(string) (Reply, error) { return Reply{Text: "not json"}, nil }
	res = mustRun(t, w, req())
	if res.Reason != ReasonNoCompared || res.CasesCompared != 0 || res.CasesRanCandidate != 0 {
		t.Fatalf("got %+v", res)
	}
	if res.CandidateScore != nil || res.Guard != GuardUnmeasured {
		t.Fatal("no_compared stores no scores")
	}
}

func TestRunFailedAttemptIsNotAMiss(t *testing.T) {
	w := newWorld()
	twentyFive(w)
	inner := scripted(func(side, id string) []string { return []string{"move|good" + caseNum(id)} })
	// every call to the candidate fails for t01 after the first attempt
	count := map[string]int{}
	w.answer = func(prompt string) (Reply, error) {
		if strings.Contains(prompt, "CAND") && strings.Contains(prompt, "CASE:t01") {
			w.mu.Lock()
			count["t01"]++
			n := count["t01"]
			w.mu.Unlock()
			if n > 1 {
				return Reply{}, errBoom
			}
		}
		return inner(prompt)
	}
	res := mustRun(t, w, req())
	if res.CasesRanCandidate != 24 || res.CasesCompared != 24 || res.CasesRanBaseline != 25 {
		t.Fatalf("ran %d/%d compared %d", res.CasesRanCandidate, res.CasesRanBaseline, res.CasesCompared)
	}
	if len(res.Flips) != 0 {
		t.Fatalf("a case that did not run must leave the guard: %v", res.Flips)
	}
}

func TestRunCountsStrayKeysOncePerSuccessfulAttemptOfComparedCases(t *testing.T) {
	w := newWorld()
	twentyFive(w)
	inner := scripted(func(side, id string) []string {
		keys := []string{"move|good" + caseNum(id)}
		if id == "t03" && side == "base" || id == "t04" && side == "base" || id == "t01" && side == "cand" {
			keys = append(keys, "move|stray")
		}
		return keys
	})
	var mu sync.Mutex
	t02 := 0
	w.answer = func(prompt string) (Reply, error) {
		switch {
		case strings.Contains(prompt, "CAND") && strings.Contains(prompt, "CASE:t04"):
			return Reply{}, errBoom // t04 never runs on the candidate, so it is not compared
		case strings.Contains(prompt, "CAND") && strings.Contains(prompt, "CASE:t02"):
			mu.Lock()
			t02++
			n := t02
			mu.Unlock()
			r, err := inner(prompt)
			if n <= 2 { // the stray key shows in two of three attempts
				r.Text = r.Text[:len(r.Text)-1] + `,{"kind":"move","target_task_id":"stray"}]`
			}
			return r, err
		}
		return inner(prompt)
	}
	res := mustRun(t, w, req())
	if res.CasesCompared != 24 {
		t.Fatalf("compared %d", res.CasesCompared)
	}
	if res.UnmatchedCandidate != 5 || res.UnmatchedBaseline != 3 {
		t.Fatalf("unmatched %d/%d, want 5/3", res.UnmatchedCandidate, res.UnmatchedBaseline)
	}
}
