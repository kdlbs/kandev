package coordinator

import "time"

// ProposalKindMerge is the proposal kind of a guarded merge of one pull or
// merge request.
const ProposalKindMerge = "merge"

// ProposalStatusUncertain is a merge whose provider outcome is not confirmed.
// It is open: it blocks any proposal for the same change and is never replayed.
const ProposalStatusUncertain ProposalStatus = "uncertain"

// ActionMerge is the activity class of a merge. It is not a policy action: a
// merge is governed by the repository policy, never by the action map.
const ActionMerge Action = "merge"

// MergeProposal is the merge detail of a proposal, keyed by the proposal ID.
type MergeProposal struct {
	ProposalID    string
	WorkspaceID   string
	CoordinatorID string
	RepositoryID  string
	ChangeKey     string
	HeadSHA       string
	Method        string
	// AutomaticRevision is the policy revision at which the repository read
	// automatic when the proposal was created; nil for any other policy.
	AutomaticRevision *int
	// ManagerOnly marks an automatic proposal a manager must decide.
	ManagerOnly bool
	// Note is the bounded text the card shows for ManagerOnly.
	Note string
	// LastReason is the last waiting reason an activity row was written for.
	LastReason string
	CreatedAt  time.Time
}

// Dispatch outcomes. An unsettled dispatch has an empty outcome.
const (
	dispatchMerged    = "merged"
	dispatchRefused   = "refused"
	dispatchReleased  = "released"
	dispatchConfirmed = dispatchMerged
)

// Lowering states of a dispatch.
const (
	loweringNone    = ""
	loweringPending = "pending"
	loweringDone    = "done"
	loweringSkipped = "skipped"
)

// MergeDispatch is the record written before the one provider merge request of
// a proposal. Its presence is what separates "never attempted" from
// "outcome unknown".
type MergeDispatch struct {
	ProposalID             string
	WorkspaceID            string
	ChangeKey              string
	HeadSHA                string
	ClaimToken             string
	Automatic              bool
	AutomaticSinceRevision int
	// Queued records a queued or in-progress provider response.
	Queued       bool
	Outcome      string
	MergeCommit  string
	Lowering     string
	DispatchedAt time.Time
	SettledAt    *time.Time
}
