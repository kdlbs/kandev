package coordinator

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func (f *kindsFixture) insertStartingMove(t *testing.T) *Proposal {
	t.Helper()
	p := f.insertMove(t)
	if _, err := f.store.db.Exec(f.store.db.Rebind(`UPDATE coordinator_proposals SET starts_agent = 1 WHERE id = ?`), p.ID); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestMoveApprove_StartsAgentRefusedByPolicy(t *testing.T) {
	cases := []struct {
		name      string
		overrides map[string]string
		onDest    bool
		want      Action
	}{
		{"start_agent denied", map[string]string{"move": "requires_approval"}, false, ActionStartAgent},
		{"start_agent denied task already on destination", map[string]string{"move": "requires_approval"}, true, ActionStartAgent},
		{"move and start_agent denied", map[string]string{"move": "denied"}, false, ActionMove},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newKindsFixture(t)
			p := f.insertStartingMove(t)
			mustSave(t, f.svc, f.c.WorkspaceID, f.c.ID, policyBody(tc.overrides))
			if tc.onDest {
				f.undo.tasks["task-0"].WorkflowStepID = "manual-step"
			}
			_, err := f.approve(p, nil)
			var denied *PolicyDeniedError
			if !errors.As(err, &denied) || denied.Action != tc.want {
				t.Fatalf("err = %v, want policy_denied %s", err, tc.want)
			}
			if got := statusOf(t, f.store, p.ID); got != string(ProposalStatusPending) || len(f.undo.moves) != 0 {
				t.Fatalf("status = %q moves = %d, want pending and no move", got, len(f.undo.moves))
			}
		})
	}
}

func TestResumeAndMessageExecute_TargetFailures(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name   string
		kind   string
		spec   string
		target func() *TargetTask
		err    error
		want   string
	}{
		{"resume archived", ProposalKindResume, `{"task_id":"task-0"}`, func() *TargetTask { tt := idleSession(); tt.ArchivedAt = &now; return tt }, nil, failTaskArchived},
		{"resume deleted", ProposalKindResume, `{"task_id":"task-0"}`, func() *TargetTask { return nil }, ErrTaskNotFound, failTaskArchived},
		{"resume no session", ProposalKindResume, `{"task_id":"task-0"}`, func() *TargetTask { tt := idleSession(); tt.Primary = nil; return tt }, nil, failNotResumable},
		{"resume completed", ProposalKindResume, `{"task_id":"task-0"}`, func() *TargetTask { tt := idleSession(); tt.Primary.State = "COMPLETED"; return tt }, nil, failNotResumable},
		{"message archived", ProposalKindMessage, `{"task_id":"task-0","text":"x"}`, func() *TargetTask { tt := idleSession(); tt.ArchivedAt = &now; return tt }, nil, failTaskArchived},
		{"message deleted", ProposalKindMessage, `{"task_id":"task-0","text":"x"}`, func() *TargetTask { return nil }, ErrTaskNotFound, failTaskArchived},
		{"message no session", ProposalKindMessage, `{"task_id":"task-0","text":"x"}`, func() *TargetTask { tt := idleSession(); tt.Primary = nil; return tt }, nil, failNotAccepting},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newKindsFixture(t)
			res, msg := &fakeResumer{started: true}, &fakeMessenger{}
			f.svc.SetKindDeps(KindDeps{Tasks: &fakeKindTasks{target: tc.target(), err: tc.err}, Resumer: res, Messenger: msg})
			p := f.insertKind(t, tc.kind, tc.spec)
			got, err := f.approve(p, nil)
			if err != nil || got.Status != ProposalStatusFailed || f.errorOf(t, p.ID) != tc.want {
				t.Fatalf("got=%+v err=%v error=%q, want failed %q", got, err, f.errorOf(t, p.ID), tc.want)
			}
			if res.calls != 0 || len(msg.prompts) != 0 {
				t.Fatalf("resume calls = %d prompts = %d, want nothing dispatched", res.calls, len(msg.prompts))
			}
		})
	}
}

func TestApprove_FailedNonCreateCardTakesANewClaim(t *testing.T) {
	f := newKindsFixture(t)
	msg := &fakeMessenger{err: ErrMessageQueueFull}
	f.svc.SetKindDeps(KindDeps{Tasks: &fakeKindTasks{target: idleSession()}, Messenger: msg})
	p := f.insertKind(t, ProposalKindMessage, `{"task_id":"task-0","text":"x"}`)
	if got, _ := f.approve(p, nil); got.Status != ProposalStatusFailed || f.errorOf(t, p.ID) != failQueueFull {
		t.Fatalf("first approve: %+v", got)
	}
	msg.err = nil
	got, err := f.approve(p, nil)
	if err != nil || got.Status != ProposalStatusApproved || len(msg.prompts) != 2 {
		t.Fatalf("retry: got=%+v err=%v prompts=%d, want approved after a second delivery", got, err, len(msg.prompts))
	}
	if f.errorOf(t, p.ID) != "" {
		t.Fatalf("error = %q, want it cleared on the new claim", f.errorOf(t, p.ID))
	}
}

func TestProposeKind_MessageAndMoveDedupe(t *testing.T) {
	f := proposeFixture(t)
	for _, tc := range []struct{ kind, args string }{
		{ProposalKindMessage, `{"task_id":"task-0","text":"hi"}`},
		{ProposalKindMove, `{"task_id":"task-0","step_id":"manual-step"}`},
	} {
		first, dup, err := f.propose(tc.kind, tc.args)
		if err != nil || dup {
			t.Fatalf("%s first: dup=%v err=%v", tc.kind, dup, err)
		}
		again, dup, err := f.propose(tc.kind, tc.args)
		if err != nil || !dup || again.ID != first.ID {
			t.Fatalf("%s again: %+v dup=%v err=%v, want the open row deduplicated", tc.kind, again, dup, err)
		}
	}
}

func TestProposeKind_OpenCapCountsNewKinds(t *testing.T) {
	f := proposeFixture(t)
	kinds := []string{ProposalKindResume, ProposalKindMessage, ProposalKindMove}
	for i := 0; i < maxOpenProposals; i++ {
		k := kinds[i%len(kinds)]
		spec := fmt.Sprintf(`{"task_id":"task-%d","text":"x","step_id":"manual-step"}`, i+100)
		f.seq = i + 99
		p := f.insertKind(t, k, spec)
		if i%2 == 1 {
			f.forceFailed(t, p.ID)
		}
	}
	_, _, err := f.propose(ProposalKindMessage, `{"task_id":"task-0","text":"one more"}`)
	if !errors.Is(err, ErrCoordinatorProposalCapReached) {
		t.Fatalf("err = %v, want ErrCoordinatorProposalCapReached at %d open new-kind rows", err, maxOpenProposals)
	}
}

func TestMessageApprove_RetryAfterFailureKeepsEditedFlag(t *testing.T) {
	f := newKindsFixture(t)
	msg := &fakeMessenger{err: ErrMessageQueueFull}
	f.svc.SetKindDeps(KindDeps{Tasks: &fakeKindTasks{target: idleSession()}, Resumer: &fakeResumer{}, Messenger: msg})
	p := f.insertKind(t, ProposalKindMessage, `{"task_id":"task-0","text":"A"}`)
	if got, err := f.approve(p, ApproveProposalRequest{"text": []byte(`"B"`)}); err != nil || got.Status != ProposalStatusFailed {
		t.Fatalf("first approve: got=%+v err=%v, want failed", got, err)
	}
	msg.err = nil
	if got, err := f.approve(p, nil); err != nil || got.Status != ProposalStatusApproved {
		t.Fatalf("retry: got=%+v err=%v, want approved", got, err)
	}
	var approved []ActivityRow
	for _, r := range listActivity(t, f.store, f.c.ID) {
		if r.Outcome == ActivityApproved {
			approved = append(approved, r)
		}
	}
	if len(approved) != 1 || !approved[0].Edited {
		t.Fatalf("approved rows = %+v, want one with edited=true", approved)
	}
}

func TestSettleStaleKind_LostRaceIsAConflictWithPhaseTwoOff(t *testing.T) {
	f := newKindsFixture(t)
	p := f.insertMove(t)
	f.forceApproving(t, p, time.Now().Add(-time.Hour))
	snapshot, err := f.store.GetProposal(context.Background(), "ws-1", f.c.ID, p.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	f.svc.runApprovalSweepPass(context.Background(), time.Now().Add(time.Hour))
	f.svc.phase2 = false
	_, err = f.svc.settleStaleKind(context.Background(), snapshot, f.svc.kindExecutor(ProposalKindMove), time.Now())
	var conflict *ProposalConflictError
	if !errors.As(err, &conflict) || conflict.Proposal.Status != ProposalStatusFailed {
		t.Fatalf("err = %v, want a conflict carrying the failed row", err)
	}
}
