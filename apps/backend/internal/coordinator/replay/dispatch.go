package replay

import (
	"context"
	"sort"
	"sync"

	"github.com/kandev/kandev/internal/coordinator/replay/stub"
)

// liveCase is a case that is not skipped, with the record slot it fills.
type liveCase struct {
	state *caseState
	input caseInput
	rec   int
}

type job struct {
	lc     *liveCase
	s      side
	index  int
	prompt string
	bound  int64
}

// callResult is one finished call; discarded when the replay was stopped while
// it ran.
type callResult struct {
	attempt   Attempt
	discarded bool
}

// freeIndexes are the attempt indexes 1..Attempts not held by a kept attempt,
// lowest first.
func freeIndexes(held []Attempt) []int {
	taken := map[int]bool{}
	for _, a := range held {
		taken[a.Index] = true
	}
	var free []int
	for k := 1; k <= Attempts; k++ {
		if !taken[k] {
			free = append(free, k)
		}
	}
	return free
}

// reusable keeps a baseline's stored successful attempts of a case, one per
// index, so a fresh attempt takes an index no kept one holds.
func reusable(stored []Attempt) []Attempt {
	var out []Attempt
	seen := map[int]bool{}
	for _, a := range stored {
		if a.OK && a.Index >= 1 && a.Index <= Attempts && !seen[a.Index] {
			seen[a.Index] = true
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out
}

// jobsFor lists the calls in case-major order: for each case the candidate's
// attempts, then the baseline's missing ones.
func (r *run) jobsFor(renders Renders, live []*liveCase, reuse map[string][]Attempt) []job {
	var jobs []job
	for _, lc := range live {
		lc.state.baseline = reusable(reuse[lc.state.id])
		candPrompt := buildPrompt(renders.CandidateText, lc.input)
		basePrompt := buildPrompt(renders.BaselineText, lc.input)
		for k := 1; k <= Attempts; k++ {
			jobs = append(jobs, job{lc: lc, s: candidateSide, index: k, prompt: candPrompt, bound: callBound(r.pricing, len(candPrompt))})
		}
		for _, k := range freeIndexes(lc.state.baseline) {
			jobs = append(jobs, job{lc: lc, s: baselineSide, index: k, prompt: basePrompt, bound: callBound(r.pricing, len(basePrompt))})
		}
	}
	return jobs
}

// dispatch runs the calls, at most Concurrency at once, each admitted by the
// budget before it starts. A stop leaves a prefix of the jobs; calls in flight
// finish and their cost is kept.
func (r *run) dispatch(ctx, runCtx context.Context, renders Renders, live []*liveCase, reuse map[string][]Attempt) {
	jobs := r.jobsFor(renders, live, reuse)
	results := make([]callResult, len(jobs))
	started := make([]bool, len(jobs))
	sem := make(chan struct{}, Concurrency)
	var wg sync.WaitGroup
loop:
	for i := range jobs {
		select {
		case sem <- struct{}{}:
		case <-runCtx.Done():
			break loop
		}
		if runCtx.Err() != nil {
			<-sem
			break
		}
		if err := r.bud.reserve(runCtx, jobs[i].bound); err != nil {
			<-sem
			r.stop(ReasonBudget)
			break
		}
		started[i] = true
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = r.call(runCtx, jobs[i])
		}(i)
	}
	wg.Wait()
	if runCtx.Err() != nil {
		if ctx.Err() != nil {
			r.stop(ReasonCancelled)
		} else {
			r.stop(ReasonBudget)
		}
	}
	for i, j := range jobs {
		if !started[i] || results[i].discarded {
			continue
		}
		if j.s == candidateSide {
			j.lc.state.candidate = append(j.lc.state.candidate, results[i].attempt)
		} else {
			j.lc.state.baseline = append(j.lc.state.baseline, results[i].attempt)
		}
	}
}

// call runs one prompt, charges its cost to the row and returns the attempt.
func (r *run) call(runCtx context.Context, j job) callResult {
	callCtx, cancel := context.WithTimeout(runCtx, RunTimeout)
	reply, err := r.h.d.Prompts.Run(callCtx, r.profile.ID, j.prompt)
	cancel()
	r.charge(j.bound, callCost(r.pricing, reply, len(j.prompt)))
	if err != nil {
		return callResult{attempt: Attempt{Index: j.index}, discarded: runCtx.Err() != nil}
	}
	keys, perr := stub.Parse(reply.Text)
	if perr != nil {
		return callResult{attempt: Attempt{Index: j.index}}
	}
	return callResult{attempt: Attempt{Index: j.index, OK: true, Keys: keys}}
}

// charge adds a call's cost to the row on a context detached from the
// replay's cancellation, along with any cost an earlier write left unwritten.
func (r *run) charge(bound, cost int64) {
	r.mu.Lock()
	r.spent = saturatingAdd(r.spent, cost)
	r.mu.Unlock()
	carried := r.bud.claimUnwritten()
	total := saturatingAdd(cost, carried)
	written := true
	if total > 0 {
		wctx, cancel := detached(FinalWriteTimeout)
		written = r.h.d.Results.AddCost(wctx, r.rowID, total) == nil
		cancel()
	}
	if !written {
		r.note("cost_update_failed")
	}
	r.bud.settle(bound, cost, carried, written)
}

func (r *run) note(n string) {
	r.mu.Lock()
	r.result.Notes = append(r.result.Notes, n)
	r.mu.Unlock()
}

// conclude turns the finished attempts into counts, flips, scores, the guard
// result and the verdict.
func (r *run) conclude(live []*liveCase) {
	states := make([]*caseState, len(live))
	for i, lc := range live {
		sort.Slice(lc.state.candidate, func(a, b int) bool { return lc.state.candidate[a].Index < lc.state.candidate[b].Index })
		sort.Slice(lc.state.baseline, func(a, b int) bool { return lc.state.baseline[a].Index < lc.state.baseline[b].Index })
		r.result.Cases[lc.rec].Candidate = lc.state.candidate
		r.result.Cases[lc.rec].Baseline = lc.state.baseline
		states[i] = lc.state
		if lc.state.ran(candidateSide) {
			r.result.CasesRanCandidate++
		}
		if lc.state.ran(baselineSide) {
			r.result.CasesRanBaseline++
		}
	}
	compared := comparedOnly(states)
	r.result.CasesCompared = len(compared)
	r.result.Flips = guardFlips(compared)
	if r.result.Reason != "" {
		return
	}
	if len(compared) == 0 {
		r.stop(ReasonNoCompared)
		return
	}
	r.result.UnmatchedCandidate = unmatched(compared, candidateSide)
	r.result.UnmatchedBaseline = unmatched(compared, baselineSide)
	r.score(compared)
}

func (r *run) score(compared []*caseState) {
	cand, _ := sideScore(compared, candidateSide)
	base, _ := sideScore(compared, baselineSide)
	r.result.CandidateScore, r.result.BaselineScore = &cand, &base
	r.result.Guard = GuardPass
	if len(r.result.Flips) > 0 {
		r.result.Guard = GuardBlocked
	}
	cited := map[string]bool{}
	for _, id := range r.result.CitedTurnIDs {
		cited[id] = true
	}
	var held []*caseState
	for _, c := range compared {
		if !cited[c.id] {
			held = append(held, c)
		}
	}
	r.result.HeldOutCompared = len(held)
	var hc, hb int64
	if len(held) > 0 {
		hc, _ = sideScore(held, candidateSide)
		hb, _ = sideScore(held, baselineSide)
		r.result.HeldOutCandidateScore, r.result.HeldOutBaselineScore = &hc, &hb
	}
	r.result.Verdict, r.result.Reason = judge(r.result.Guard, len(held), hc, hb)
}
