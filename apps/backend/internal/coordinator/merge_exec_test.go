package coordinator

import (
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/coordinator/mergestate"
)

func TestApproveMergesOnceWithExpectedHeadAndMethod(t *testing.T) {
	e := newMergeExec(t)
	id := *e.propose(t, "h1").ProposalID
	if _, err := e.approve(id); err != nil {
		t.Fatal(err)
	}
	if e.merger.mergeCalls() != 1 {
		t.Fatalf("merge calls = %d", e.merger.mergeCalls())
	}
	c := e.merger.calls[0]
	if c.Head != "h1" || c.Method != "squash" || c.Key != testKey {
		t.Fatalf("call = %+v", c)
	}
	if st, _ := e.status(t, id); st != "approved" {
		t.Fatalf("status = %s", st)
	}
}

func TestApproveRefusesWhenHeadMoved(t *testing.T) {
	e := newMergeExec(t)
	id := *e.propose(t, "h1").ProposalID
	e.merger.set(func(f *fakeMerger) { f.snap = greenSnapshot("h2") })
	_, _ = e.approve(id)
	if e.merger.mergeCalls() != 0 {
		t.Fatal("merged a moved head")
	}
	if st, code := e.status(t, id); st != "rejected" && st != "failed" || code == "" {
		t.Fatalf("status = %s/%s", st, code)
	}
}

func TestProviderRejectIsRefusedNeverFailed(t *testing.T) {
	e := newMergeExec(t)
	id := *e.propose(t, "h1").ProposalID
	e.merger.set(func(f *fakeMerger) { f.result = MergeResult{Outcome: MergeRejected, Reason: "no"} })
	_, _ = e.approve(id)
	if st, _ := e.status(t, id); st == "failed" || st == "approved" {
		t.Fatalf("status = %s", st)
	}
}

func TestProviderStaleHeadIsRefused(t *testing.T) {
	e := newMergeExec(t)
	id := *e.propose(t, "h1").ProposalID
	e.merger.set(func(f *fakeMerger) { f.result = MergeResult{Outcome: MergeStaleHead} })
	_, _ = e.approve(id)
	if st, _ := e.status(t, id); st == "failed" || st == "approved" {
		t.Fatalf("status = %s", st)
	}
}

func TestQueuedResponseStaysUncertain(t *testing.T) {
	e := newMergeExec(t)
	id := *e.propose(t, "h1").ProposalID
	e.merger.set(func(f *fakeMerger) { f.result = MergeResult{Outcome: MergeQueued} })
	_, _ = e.approve(id)
	if st, _ := e.status(t, id); st != "uncertain" {
		t.Fatalf("status = %s", st)
	}
}

func TestTransportErrorIsUncertainAndNotReplayed(t *testing.T) {
	e := newMergeExec(t)
	id := *e.propose(t, "h1").ProposalID
	e.merger.set(func(f *fakeMerger) { f.mergeErr = errors.New("timeout") })
	_, _ = e.approve(id)
	if st, _ := e.status(t, id); st != "uncertain" {
		t.Fatalf("status = %s", st)
	}
	_, _ = e.approve(id)
	if e.merger.mergeCalls() != 1 {
		t.Fatalf("an uncertain merge was replayed: %d calls", e.merger.mergeCalls())
	}
}

func TestConcurrentApprovalsSendOneRequest(t *testing.T) {
	e := newMergeExec(t)
	id := *e.propose(t, "h1").ProposalID
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = e.approve(id) }()
	}
	wg.Wait()
	if n := e.merger.mergeCalls(); n != 1 {
		t.Fatalf("merge calls = %d", n)
	}
}

func TestRefusedPreflightNeverCallsMerge(t *testing.T) {
	cases := map[string]func(*fakeMerger){
		"pipeline red": func(f *fakeMerger) { f.snap.Pipeline = mergestate.PipelineRed },
		"draft":        func(f *fakeMerger) { f.snap.Draft = mergestate.Unmet },
		"unresolved":   func(f *fakeMerger) { f.snap.UnresolvedThreads = 2 },
	}
	for name, fn := range cases {
		t.Run(name, func(t *testing.T) {
			e := newMergeExec(t)
			id := *e.propose(t, "h1").ProposalID
			e.merger.set(fn)
			_, _ = e.approve(id)
			if e.merger.mergeCalls() != 0 {
				t.Fatal("merged despite a failing recheck")
			}
			if st, _ := e.status(t, id); st == "approved" || st == "failed" {
				t.Fatalf("status = %s", st)
			}
		})
	}
}

func TestPolicyDeniedAfterProposeBlocksMerge(t *testing.T) {
	e := newMergeExec(t)
	id := *e.propose(t, "h1").ProposalID
	e.setPolicy(t, RepoDenied, 0)
	_, _ = e.approve(id)
	if e.merger.mergeCalls() != 0 {
		t.Fatal("merged under a denied policy")
	}
}
