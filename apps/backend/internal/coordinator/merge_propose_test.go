package coordinator

import (
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/coordinator/mergestate"
)

func TestProposeCreatesPendingProposalFrozenOnHeadAndMethod(t *testing.T) {
	e := newMergeExec(t)
	res := e.propose(t, "h1")
	if res.ProposalID == nil || res.Status != "pending" || res.CreationCode != "" {
		t.Fatalf("result = %+v", res)
	}
	m, err := e.store.getMergeProposal(e.ctx, e.store.db, *res.ProposalID)
	if err != nil || m.HeadSHA != "h1" || m.Method != "squash" || m.ChangeKey != testChangeKey || m.RepositoryID != "r1" {
		t.Fatalf("merge row = %+v, %v", m, err)
	}
	if e.merger.mergeCalls() != 0 {
		t.Fatal("proposing merged")
	}
}

func TestProposeRefusalCodes(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*testing.T, *mergeExec)
		head  string
		code  string
	}{
		{"change unknown", func(t *testing.T, e *mergeExec) { e.svc.SetChangeLinks(fakeChangeLinks{}) }, "h1", createChangeUnknown},
		{"policy denied", func(t *testing.T, e *mergeExec) { e.setPolicy(t, RepoDenied, 0) }, "h1", createPolicyDenied},
		{"method unknown", func(t *testing.T, e *mergeExec) { e.merger.set(func(f *fakeMerger) { f.methods = MethodSet{} }) }, "h1", createMethod},
		{"method read fails", func(t *testing.T, e *mergeExec) {
			e.merger.set(func(f *fakeMerger) { f.methodsErr = errors.New("boom") })
		}, "h1", createMethod},
		{"state unavailable", func(t *testing.T, e *mergeExec) { e.merger.set(func(f *fakeMerger) { f.snapErr = errors.New("boom") }) }, "h1", createUnavailable},
		{"change closed", func(t *testing.T, e *mergeExec) {
			e.merger.set(func(f *fakeMerger) { f.snap.State = mergestate.StateClosed })
		}, "h1", createUnavailable},
		{"head mismatch", func(t *testing.T, e *mergeExec) {}, "other", createHeadMismatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newMergeExec(t)
			tc.setup(t, e)
			res := e.propose(t, tc.head)
			if res.ProposalID != nil || res.Status != "refused" || res.CreationCode != tc.code {
				t.Fatalf("result = %+v", res)
			}
			if n := e.activity(t, ActivityRefused, tc.code); n != 1 {
				t.Fatalf("refusal activity rows = %d", n)
			}
			var stored int
			_ = e.store.db.QueryRow(`SELECT COUNT(*) FROM coordinator_proposals WHERE kind = 'merge'`).Scan(&stored)
			if stored != 0 {
				t.Fatal("a refused creation stored a proposal")
			}
			if e.merger.mergeCalls() != 0 {
				t.Fatal("a refused creation merged")
			}
		})
	}
}

func TestCreationRefusalOrder(t *testing.T) {
	e := newMergeExec(t)
	e.setPolicy(t, RepoDenied, 0)
	e.merger.set(func(f *fakeMerger) { f.methods = MethodSet{}; f.snapErr = errors.New("boom") })
	want := []string{createPolicyDenied, createMethod, createUnavailable, createHeadMismatch}
	fixes := []func(){
		func() { e.setPolicy(t, RepoRequiresApproval, 0) },
		func() {
			e.merger.set(func(f *fakeMerger) { f.methods = MethodSet{Allowed: []string{"squash"}, Default: "squash"} })
		},
		func() { e.merger.set(func(f *fakeMerger) { f.snapErr = nil }) },
	}
	for i, code := range want {
		if res := e.propose(t, "other"); res.CreationCode != code {
			t.Fatalf("step %d: code = %q, want %q", i, res.CreationCode, code)
		}
		if i < len(fixes) {
			fixes[i]()
		}
	}
}

func TestChangeUnknownBeforePolicyDenied(t *testing.T) {
	e := newMergeExec(t)
	e.setPolicy(t, RepoDenied, 0)
	e.svc.SetChangeLinks(fakeChangeLinks{})
	if res := e.propose(t, "h1"); res.CreationCode != createChangeUnknown {
		t.Fatalf("code = %q", res.CreationCode)
	}
}

func TestPredecessorRefusesCreation(t *testing.T) {
	e := newMergeExec(t)
	e.fix.addTask(t, "ta", "wf-a")
	e.svc.SetChangeLinks(fakeChangeLinks{byChange: map[string][]string{"github|github.com|r1|1": {"ta"}, testChangeKey: {"tm"}}})
	if _, err := e.svc.SaveDependencies(e.ctx, e.fix.c.WorkspaceID, e.fix.c.ID, "tm", true, 0, []DepRef{{Task: "ta"}}); err != nil {
		t.Fatal(err)
	}
	if res := e.propose(t, "h1"); res.CreationCode != createPredecessor {
		t.Fatalf("code = %q", res.CreationCode)
	}
}

func TestDedupeReturnsOpenProposal(t *testing.T) {
	e := newMergeExec(t)
	first := e.propose(t, "h1")
	second := e.propose(t, "h1")
	if second.ProposalID == nil || *second.ProposalID != *first.ProposalID || !second.Deduplicated {
		t.Fatalf("second = %+v", second)
	}
}

func TestDedupeSupersedesPendingWithOtherHead(t *testing.T) {
	e := newMergeExec(t)
	first := e.propose(t, "h1")
	e.merger.set(func(f *fakeMerger) { f.snap = greenSnapshot("h2") })
	second := e.propose(t, "h2")
	if second.ProposalID == nil || *second.ProposalID == *first.ProposalID || second.Deduplicated {
		t.Fatalf("second = %+v", second)
	}
	if st, code := e.status(t, *first.ProposalID); st != "rejected" || code != refuseStaleHead {
		t.Fatalf("old proposal = %s/%s", st, code)
	}
}

func TestDedupeReturnsApprovingUnchanged(t *testing.T) {
	for _, status := range []string{"approving", "uncertain"} {
		t.Run(status, func(t *testing.T) {
			e := newMergeExec(t)
			first := e.propose(t, "h1")
			mustExec(t, e.store, `UPDATE coordinator_proposals SET status = '`+status+`' WHERE id = '`+*first.ProposalID+`'`)
			e.merger.set(func(f *fakeMerger) { f.snap = greenSnapshot("h2") })
			second := e.propose(t, "h2")
			if second.ProposalID == nil || *second.ProposalID != *first.ProposalID || !second.Deduplicated {
				t.Fatalf("second = %+v", second)
			}
			if st, _ := e.status(t, *first.ProposalID); st != status {
				t.Fatalf("status = %s", st)
			}
		})
	}
}

func TestProposeFlagOffIsDenied(t *testing.T) {
	e := newMergeExec(t)
	e.svc.merge = false
	_, err := e.svc.ProposeMerge(e.ctx, e.fix.c.ID, []byte(`{"task_id":"tm","expected_head_sha":"h1"}`))
	var denied *PolicyDeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("err = %v", err)
	}
}

func TestProposeRejectsUnknownFieldsAndUnwatchedTask(t *testing.T) {
	e := newMergeExec(t)
	for _, body := range []string{`{"task_id":"tm","expected_head_sha":"h1","extra":1}`, `{"expected_head_sha":"h1"}`, `{"task_id":"tm"}`, `{"task_id":"nope","expected_head_sha":"h1"}`} {
		var field *FieldError
		if _, err := e.svc.ProposeMerge(e.ctx, e.fix.c.ID, []byte(body)); !errors.As(err, &field) {
			t.Fatalf("%s: err = %v", body, err)
		}
	}
}
