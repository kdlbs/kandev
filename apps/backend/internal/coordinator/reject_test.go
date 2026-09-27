package coordinator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/authz"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

func strPtr(s string) *string { return &s }

// TestRejectProposal_ForbiddenRequiresManageScope proves the reject route
// authorizes with workspace.manage (not read) before touching the store, and
// that a denial propagates without a write
// (docs/plans/workspace-coordinator/task-07-proposals-backend.md's "a reader
// gets 403").
func TestRejectProposal_ForbiddenRequiresManageScope(t *testing.T) {
	store, c, _, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	svc.authz = &fakeWorkspaceAuthorizer{err: taskservice.ErrForbidden}

	_, err := svc.RejectProposal(context.Background(), "ws-1", c.ID, p.ID, RejectProposalRequest{})
	if !errors.Is(err, taskservice.ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
	assertLastScope(t, svc, authz.ScopeWorkspaceManage)

	reread, rerr := store.GetProposal(context.Background(), "ws-1", c.ID, p.ID)
	if rerr != nil {
		t.Fatalf("GetProposal: %v", rerr)
	}
	if reread.Status != ProposalStatusPending {
		t.Fatalf("Status = %q, want pending (no write should have happened)", reread.Status)
	}
}

func TestRejectProposal_PendingNoReason(t *testing.T) {
	store, c, _, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())

	got, err := svc.RejectProposal(context.Background(), "ws-1", c.ID, p.ID, RejectProposalRequest{})
	if err != nil {
		t.Fatalf("RejectProposal: %v", err)
	}
	if got.Status != ProposalStatusRejected {
		t.Fatalf("Status = %q, want rejected", got.Status)
	}
	if got.RejectReason != nil {
		t.Fatalf("RejectReason = %v, want nil", got.RejectReason)
	}
}

func TestRejectProposal_PendingWithReasonTrimsAndStores(t *testing.T) {
	store, c, _, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())

	got, err := svc.RejectProposal(context.Background(), "ws-1", c.ID, p.ID, RejectProposalRequest{Reason: strPtr("  Not needed  ")})
	if err != nil {
		t.Fatalf("RejectProposal: %v", err)
	}
	if got.Status != ProposalStatusRejected {
		t.Fatalf("Status = %q, want rejected", got.Status)
	}
	if got.RejectReason == nil || *got.RejectReason != "Not needed" {
		t.Fatalf("RejectReason = %v, want \"Not needed\"", got.RejectReason)
	}
}

func TestRejectProposal_WhitespaceOnlyReasonStoresNil(t *testing.T) {
	store, c, _, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())

	got, err := svc.RejectProposal(context.Background(), "ws-1", c.ID, p.ID, RejectProposalRequest{Reason: strPtr("   ")})
	if err != nil {
		t.Fatalf("RejectProposal: %v", err)
	}
	if got.RejectReason != nil {
		t.Fatalf("RejectReason = %v, want nil", got.RejectReason)
	}
}

func TestRejectProposal_FailedIsAllowed(t *testing.T) {
	store, c, _, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	claimDirectly(t, store, p, "tok", sampleSpec(), time.Now())
	if _, err := store.FailProposal(context.Background(), p.ID, "tok", "boom", time.Now()); err != nil {
		t.Fatalf("FailProposal: %v", err)
	}

	got, err := svc.RejectProposal(context.Background(), "ws-1", c.ID, p.ID, RejectProposalRequest{})
	if err != nil {
		t.Fatalf("RejectProposal: %v", err)
	}
	if got.Status != ProposalStatusRejected {
		t.Fatalf("Status = %q, want rejected", got.Status)
	}
}

func TestRejectProposal_ApprovedIsConflict(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	tasks.createResult = createdResult("task-new")
	tasks.settled = true
	if _, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{}); err != nil {
		t.Fatalf("ApproveProposal (setup): %v", err)
	}

	_, err := svc.RejectProposal(context.Background(), "ws-1", c.ID, p.ID, RejectProposalRequest{})
	_ = assertConflict(t, err, ProposalStatusApproved)
}

func TestRejectProposal_ApprovingIsConflict(t *testing.T) {
	store, c, _, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	claimDirectly(t, store, p, "tok", sampleSpec(), time.Now())

	_, err := svc.RejectProposal(context.Background(), "ws-1", c.ID, p.ID, RejectProposalRequest{})
	_ = assertConflict(t, err, ProposalStatusApproving)
}

func TestRejectProposal_AlreadyRejectedIsConflict(t *testing.T) {
	store, c, _, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	if _, err := svc.RejectProposal(context.Background(), "ws-1", c.ID, p.ID, RejectProposalRequest{}); err != nil {
		t.Fatalf("RejectProposal (setup): %v", err)
	}

	_, err := svc.RejectProposal(context.Background(), "ws-1", c.ID, p.ID, RejectProposalRequest{})
	_ = assertConflict(t, err, ProposalStatusRejected)
}

func TestRejectProposal_ReasonTooLongReturns400(t *testing.T) {
	store, c, _, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	reason := strings.Repeat("a", proposalRejectReasonMaxRunes+1)

	_, err := svc.RejectProposal(context.Background(), "ws-1", c.ID, p.ID, RejectProposalRequest{Reason: &reason})
	assertFieldError(t, err, "reason")

	reread, rerr := store.GetProposal(context.Background(), "ws-1", c.ID, p.ID)
	if rerr != nil {
		t.Fatalf("GetProposal: %v", rerr)
	}
	if reread.Status != ProposalStatusPending {
		t.Fatalf("Status = %q, want pending (no write should have happened)", reread.Status)
	}
}

func TestRejectProposal_ReasonAtMaxLengthIsValid(t *testing.T) {
	store, c, _, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	reason := strings.Repeat("a", proposalRejectReasonMaxRunes)

	got, err := svc.RejectProposal(context.Background(), "ws-1", c.ID, p.ID, RejectProposalRequest{Reason: &reason})
	if err != nil {
		t.Fatalf("RejectProposal: %v", err)
	}
	if got.RejectReason == nil || *got.RejectReason != reason {
		t.Fatalf("RejectReason length = %d, want %d", len(derefOrEmpty(got.RejectReason)), len(reason))
	}
}

func TestRejectProposal_UnknownProposalIsNotFound(t *testing.T) {
	_, c, _, svc := approveFixture(t)

	_, err := svc.RejectProposal(context.Background(), "ws-1", c.ID, "missing-id", RejectProposalRequest{})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func derefOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
