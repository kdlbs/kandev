package coordinator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/authz"
)

func TestImprovementApproveStoresPendingChangeOnly(t *testing.T) {
	f := newImprovementFixture(t)
	got, change := f.approved(t)
	if got.Status != ProposalStatusApproved {
		t.Fatalf("status = %s", got.Status)
	}
	if f.context(t) != "old instructions" {
		t.Fatalf("approve changed the context to %q", f.context(t))
	}
	if change.Status != ChangeStatusPending || change.BaseValue != "old instructions" || change.NewValue != "new instructions" || change.Field != "context" {
		t.Fatalf("unexpected change: %+v", change)
	}
	if change.ProposalTitle != "Shorter wake prompts" {
		t.Fatalf("title = %q", change.ProposalTitle)
	}
}

func TestImprovementApproveRefusesEdits(t *testing.T) {
	f := newImprovementFixture(t)
	p := f.mustPropose(t)
	_, err := f.approve(p, ApproveProposalRequest{"context": []byte(`"other"`)})
	var fe *FieldError
	if !errors.As(err, &fe) {
		t.Fatalf("err = %v, want FieldError", err)
	}
	if statusOf(t, f.store, p.ID) != string(ProposalStatusPending) {
		t.Fatalf("proposal left pending state")
	}
	if f.changeRows(t) != 0 {
		t.Fatalf("change row written on refused edit")
	}
}

func TestImprovementRepeatedApproveWritesOneChange(t *testing.T) {
	f := newImprovementFixture(t)
	p, _ := f.approved(t)
	if _, err := f.approve(p, nil); err == nil {
		t.Fatalf("second approve succeeded")
	}
	if f.changeRows(t) != 1 {
		t.Fatalf("rows = %d, want 1", f.changeRows(t))
	}
}

func TestImprovementApproveActivityClass(t *testing.T) {
	f := newImprovementFixture(t)
	f.approved(t)
	var found bool
	for _, row := range listActivity(t, f.store, f.c.ID) {
		if row.ActionClass == ActionImprovement && row.Outcome == ActivityApproved {
			found = true
			if row.TargetTaskID != nil {
				t.Fatalf("improvement activity carries a target task: %v", *row.TargetTaskID)
			}
		}
	}
	if !found {
		t.Fatalf("no executed improvement activity row")
	}
}

func TestImprovementRejectWritesNoChange(t *testing.T) {
	f := newImprovementFixture(t)
	p := f.mustPropose(t)
	if _, err := f.svc.RejectProposal(context.Background(), "ws-1", f.c.ID, p.ID, RejectProposalRequest{}); err != nil {
		t.Fatal(err)
	}
	if statusOf(t, f.store, p.ID) != string(ProposalStatusRejected) || f.changeRows(t) != 0 {
		t.Fatalf("reject left state behind")
	}
}

func TestImprovementApplyWritesContext(t *testing.T) {
	f := newImprovementFixture(t)
	f.svc.SetConversationHooks(nil, nil)
	_, change := f.approved(t)
	before, _ := f.store.GetCoordinatorByID(context.Background(), f.c.ID)
	got, err := f.apply(change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Context != "new instructions" || f.context(t) != "new instructions" {
		t.Fatalf("context = %q", got.Context)
	}
	if got.ConfigRevision != before.ConfigRevision+1 {
		t.Fatalf("revision %d -> %d", before.ConfigRevision, got.ConfigRevision)
	}
	changes, _ := f.svc.ListPendingChanges(context.Background(), "ws-1", f.c.ID)
	if len(changes) != 0 {
		t.Fatalf("applied change still listed: %+v", changes)
	}
}

func TestImprovementApplyContextChangedConflicts(t *testing.T) {
	f := newImprovementFixture(t)
	_, change := f.approved(t)
	f.setContext(t, "someone edited")
	_, err := f.apply(change.ID)
	var conflict *ChangeConflictError
	if !errors.As(err, &conflict) || conflict.Reason != ChangeReasonContextChanged {
		t.Fatalf("err = %v", err)
	}
	if f.context(t) != "someone edited" {
		t.Fatalf("conflict overwrote the context")
	}
	changes, _ := f.svc.ListPendingChanges(context.Background(), "ws-1", f.c.ID)
	if len(changes) != 1 || changes[0].Status != ChangeStatusPending {
		t.Fatalf("change not left pending: %+v", changes)
	}
}

func TestImprovementApplyInvalidStoredValueLeavesPending(t *testing.T) {
	f := newImprovementFixture(t)
	_, change := f.approved(t)
	mustExec(t, f.store, `UPDATE coordinator_pending_changes SET new_value = ? WHERE id = ?`, strings.Repeat("x", contextMaxRunes+1), change.ID)
	if _, err := f.apply(change.ID); err == nil {
		t.Fatalf("apply of an invalid stored value succeeded")
	}
	if f.context(t) != "old instructions" {
		t.Fatalf("context changed")
	}
	var status string
	if err := f.store.db.Get(&status, f.store.db.Rebind(`SELECT status FROM coordinator_pending_changes WHERE id = ?`), change.ID); err != nil || status != ChangeStatusPending {
		t.Fatalf("status = %q err=%v", status, err)
	}
}

func TestImprovementApplySettledChangeConflictsWithStatus(t *testing.T) {
	f := newImprovementFixture(t)
	_, change := f.approved(t)
	if _, err := f.apply(change.ID); err != nil {
		t.Fatal(err)
	}
	f.setContext(t, "old instructions")
	_, err := f.apply(change.ID)
	var conflict *ChangeConflictError
	if !errors.As(err, &conflict) || conflict.Change == nil || conflict.Change.Status != ChangeStatusApplied || conflict.Reason != "" {
		t.Fatalf("err = %v", err)
	}
	if f.context(t) != "old instructions" {
		t.Fatalf("second apply rewrote the context")
	}
	if _, err := f.svc.DiscardPendingChange(context.Background(), "ws-1", f.c.ID, change.ID); !errors.As(err, &conflict) {
		t.Fatalf("discard of applied change err = %v", err)
	}
}

func TestImprovementDiscardSettlesChange(t *testing.T) {
	f := newImprovementFixture(t)
	_, change := f.approved(t)
	got, err := f.svc.DiscardPendingChange(context.Background(), "ws-1", f.c.ID, change.ID)
	if err != nil || got.Status != ChangeStatusDiscarded {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if f.context(t) != "old instructions" {
		t.Fatalf("discard changed the context")
	}
	var conflict *ChangeConflictError
	if _, err := f.apply(change.ID); !errors.As(err, &conflict) || conflict.Change.Status != ChangeStatusDiscarded {
		t.Fatalf("apply after discard err = %v", err)
	}
}

func TestImprovementChangeNotFoundCases(t *testing.T) {
	f := newImprovementFixture(t)
	_, change := f.approved(t)
	if _, err := f.apply("missing"); !errors.Is(err, ErrChangeNotFound) {
		t.Fatalf("unknown change err = %v", err)
	}
	if _, err := f.svc.DiscardPendingChange(context.Background(), "ws-1", f.c.ID, "missing"); !errors.Is(err, ErrChangeNotFound) {
		t.Fatalf("discard unknown err = %v", err)
	}
	if _, err := f.svc.ApplyPendingChange(context.Background(), "ws-1", "other", change.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other coordinator err = %v", err)
	}
	mustExec(t, f.store, `UPDATE coordinator_proposals SET status = 'pending' WHERE id = ?`, change.ProposalID)
	if _, err := f.apply(change.ID); !errors.Is(err, ErrChangeNotFound) {
		t.Fatalf("unapproved proposal apply err = %v", err)
	}
	if _, err := f.svc.DiscardPendingChange(context.Background(), "ws-1", f.c.ID, change.ID); !errors.Is(err, ErrChangeNotFound) {
		t.Fatalf("unapproved proposal discard err = %v", err)
	}
}

func TestImprovementChangesHiddenWhenPhase3Off(t *testing.T) {
	f := newImprovementFixture(t)
	_, change := f.approved(t)
	f.svc.phase3 = false
	if _, err := f.svc.ListPendingChanges(context.Background(), "ws-1", f.c.ID); !errors.Is(err, ErrChangeNotFound) {
		t.Fatalf("list err = %v", err)
	}
	if _, err := f.apply(change.ID); !errors.Is(err, ErrChangeNotFound) {
		t.Fatalf("apply err = %v", err)
	}
	if _, err := f.svc.DiscardPendingChange(context.Background(), "ws-1", f.c.ID, change.ID); !errors.Is(err, ErrChangeNotFound) {
		t.Fatalf("discard err = %v", err)
	}
	if f.context(t) != "old instructions" {
		t.Fatalf("context changed while hidden")
	}
}

func TestImprovementChangeAuthorizationScopes(t *testing.T) {
	f := newImprovementFixture(t)
	_, change := f.approved(t)
	fake := f.svc.authz.(*fakeWorkspaceAuthorizer)
	if _, err := f.svc.ListPendingChanges(context.Background(), "ws-1", f.c.ID); err != nil {
		t.Fatal(err)
	}
	assertLastScope(t, f.svc, authz.ScopeWorkspaceRead)
	if _, err := f.apply(change.ID); err != nil {
		t.Fatal(err)
	}
	assertLastScope(t, f.svc, authz.ScopeWorkspaceManage)
	fake.err = errors.New("denied")
	f.setContext(t, "old instructions")
	if _, err := f.apply(change.ID); err == nil || f.context(t) != "old instructions" {
		t.Fatalf("forbidden apply err = %v", err)
	}
	if _, err := f.svc.DiscardPendingChange(context.Background(), "ws-1", f.c.ID, change.ID); err == nil {
		t.Fatalf("forbidden discard succeeded")
	}
}
