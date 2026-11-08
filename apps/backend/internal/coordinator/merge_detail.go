package coordinator

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/coordinator/mergestate"
)

// MergeLive is the uncached provider reading shown on an opened merge proposal.
type MergeLive struct {
	State             string `json:"state"`
	HeadSHA           string `json:"head_sha"`
	Draft             string `json:"draft"`
	Pipeline          string `json:"pipeline"`
	Mergeable         string `json:"mergeable"`
	Conflict          string `json:"conflict"`
	UnresolvedThreads int    `json:"unresolved_threads"`
	Approvals         string `json:"approvals"`
}

// MergeDispatchView is what a manager sees of a recorded dispatch.
type MergeDispatchView struct {
	Outcome      string    `json:"outcome"`
	Queued       bool      `json:"queued"`
	DispatchedAt time.Time `json:"dispatched_at"`
	// CanCheck is true once the dispatch is old enough to read; CanRelease
	// once it has gone unconfirmed for more than 24 hours.
	CanCheck   bool `json:"can_check"`
	CanRelease bool `json:"can_release"`
}

// MergeDetail is the merge part of a proposal's detail.
type MergeDetail struct {
	ChangeKey    string             `json:"change_key"`
	RepositoryID string             `json:"repository_id"`
	HeadSHA      string             `json:"head_sha"`
	Method       string             `json:"method"`
	Automatic    bool               `json:"automatic"`
	ManagerOnly  bool               `json:"manager_only"`
	Note         string             `json:"note,omitempty"`
	LastReason   string             `json:"last_reason,omitempty"`
	Reason       string             `json:"reason,omitempty"`
	Live         *MergeLive         `json:"live,omitempty"`
	Dispatch     *MergeDispatchView `json:"dispatch,omitempty"`
}

func triName(t mergestate.Tri) string {
	switch t {
	case mergestate.Met:
		return "met"
	case mergestate.Unmet:
		return "unmet"
	}
	return "unavailable"
}

func conflictName(c mergestate.ConflictState) string {
	switch c {
	case mergestate.ConflictPresent:
		return "present"
	case mergestate.ConflictClear:
		return "clear"
	}
	return "unknown"
}

func liveOf(snap mergestate.Snapshot) *MergeLive {
	return &MergeLive{
		State: string(snap.State), HeadSHA: snap.HeadSHA, Draft: triName(snap.Draft), Pipeline: string(snap.Pipeline),
		Mergeable: triName(snap.Mergeable.Mergeable), Conflict: conflictName(snap.Conflict),
		UnresolvedThreads: snap.UnresolvedThreads, Approvals: triName(snap.Approvals),
	}
}

// OpenMergeDetail is the proposal detail read of a merge proposal. For a
// manager it runs one uncached provider read and settles a pending proposal
// that can no longer be merged; the returned proposal is then the settled one.
// It returns a nil detail for a proposal that is not a merge or while the
// merge flag is off.
func (s *Service) OpenMergeDetail(ctx context.Context, p *Proposal) (*Proposal, *MergeDetail) {
	if !s.merge || p.Kind != ProposalKindMerge {
		return p, nil
	}
	m, err := s.store.getMergeProposal(ctx, s.store.ro, p.ID)
	if err != nil {
		return p, nil
	}
	c, err := s.store.GetCoordinatorByID(ctx, p.CoordinatorID)
	if err != nil {
		return p, nil
	}
	detail := &MergeDetail{ChangeKey: m.ChangeKey, RepositoryID: m.RepositoryID, HeadSHA: m.HeadSHA, Method: m.Method,
		Automatic: m.AutomaticRevision != nil, ManagerOnly: m.ManagerOnly, Note: m.Note, LastReason: m.LastReason}
	if s.authz.AuthorizeWorkspaceScope(ctx, p.WorkspaceID, authz.ScopeWorkspaceManage) != nil {
		return p, detail
	}
	if p.Status == ProposalStatusPending {
		if s.sweepPendingMerge(ctx, c, p, m) {
			if latest, err := s.store.GetProposalAnyKind(ctx, p.WorkspaceID, p.CoordinatorID, p.ID); err == nil {
				p = latest
			}
		}
	}
	if reading, err := s.readMerge(ctx, c, m, targetOf(p), false); err == nil {
		detail.Live, detail.Reason = liveOf(reading.Snap), reading.Decision.Reason
	}
	if d, err := s.store.getDispatch(ctx, s.store.ro, p.ID); err == nil && d != nil {
		age := s.store.now().Sub(d.DispatchedAt)
		detail.Dispatch = &MergeDispatchView{Outcome: d.Outcome, Queued: d.Queued, DispatchedAt: d.DispatchedAt,
			CanCheck:   p.Status == ProposalStatusUncertain && age >= reconcileMinAge,
			CanRelease: p.Status == ProposalStatusUncertain && age > unconfirmedAfter}
	}
	return p, detail
}
