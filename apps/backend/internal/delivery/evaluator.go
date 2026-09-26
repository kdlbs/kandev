package delivery

import (
	"strings"
	"time"
)

// Snapshot is one task_session_git_snapshots row read for a candidate
// pair. HeadCommit and Ahead are the raw column values; Classify performs
// the normalization spec "Classification" requires (a NULL/empty/
// whitespace-only head is not a distinct value; a NULL/negative ahead
// reads as 0) so those rules are exercised as pure Go logic rather than
// trusted to whichever caller built the slice.
type Snapshot struct {
	SessionID  string
	Branch     string
	HeadCommit string
	Ahead      *int
	CreatedAt  time.Time
}

// ProviderRequest is one provider pull/merge request row read for a
// candidate pair, already normalized to the "Provider predicates" table
// (Merged/Detached are the per-provider SQL predicates evaluated at the
// read layer, since the three provider tables share no schema).
type ProviderRequest struct {
	Provider      string // ProviderAzureDevOps | ProviderGitHub | ProviderGitLab
	Merged        bool
	Detached      bool
	MergeInstant  time.Time
	RequestNumber int
	URL           string
	BaseBranch    string
	ScopeValue    string // github: owner+"/"+repo; gitlab: project_path; azure: azure_repository_id
}

// AncestryOutcome is the result of an ancestry check attempted this
// evaluation, or the zero value when no check was attempted (precondition
// not met, or the caller decided not to run one).
type AncestryOutcome struct {
	Attempted bool
	Errored   bool
	Positive  bool
	Commit    string // the head commit submitted to the check
}

// PairInput is every input Classify reads for one (task, repository)
// pair, gathered by the caller before calling Classify. DefaultBranch =="
// (already normalized from NULL) triggers Degraded evaluation.
type PairInput struct {
	DefaultBranch string
	Snapshots     []Snapshot
	Providers     []ProviderRequest
	Ancestry      AncestryOutcome
}

// Classification is the result of evaluating one pair once. It is a pure
// function of PairInput — re-running Classify on unchanged inputs
// produces a byte-identical result (spec "Idempotency").
type Classification struct {
	Outcome Outcome
	Basis   Basis
	Ref     *string
	Rank    int

	// ReachedDefault reports whether THIS evaluation produced a
	// default-branch observation (by any basis), independent of whether a
	// write-once column already holds an earlier one. Rule 3 reads this
	// value, never the stored reached_default_at (spec "Classification",
	// "What a predicate may read").
	ReachedDefault bool
	ReachedBasis   ReachedBasis
	ReachedRef     string

	// ObservedBranchCommits is nil when the pair has no snapshots at all
	// (spec "Ordering": the incoming value is NULL, not 0, for an empty
	// snapshot set).
	ObservedBranchCommits *int
}

// AncestryPrecondition reports whether an ancestry check should be
// attempted for this pair, per spec "Default-branch observation §
// Ancestry precondition": at least one session shows two or more distinct
// non-empty head_commit values among its own snapshots, evaluated across
// every branch (not restricted to the default branch).
func AncestryPrecondition(snapshots []Snapshot) bool {
	return len(movingSessions(snapshots, nil)) > 0
}

// SelectAncestryHead picks the commit to submit to the ancestry check.
//
// Deviation from the literal spec text, recorded as a documented Build
// decision (R5-F1 in the plan): the spec's "Which commit" paragraph
// selects pair-wide, across every session's snapshots, not restricted to
// the session(s) that satisfied the precondition. That is unsafe on a
// multi-session pair — an idle session sitting on the default branch can
// supply a newer, unrelated snapshot whose inherited head trivially
// satisfies --is-ancestor, permanently misattributing a write-once
// column. This implementation restricts selection to the precondition-
// satisfying session(s), which is the fix the spec's own precondition
// paragraph describes wanting. No existing scenario in the spec
// contradicts this (both ancestry head-selection scenarios there are
// single-session).
func SelectAncestryHead(snapshots []Snapshot) string {
	moving := movingSessions(snapshots, nil)
	if len(moving) == 0 {
		return ""
	}
	return selectHeadCommit(snapshots, func(s Snapshot) bool {
		return moving[s.SessionID]
	})
}

// Classify evaluates one (task, repository) pair once, applying the
// six-rule precedence, degraded evaluation, and the default-branch
// observation basis precedence, all as pure computation over the already-
// gathered inputs (spec "Classification", "Default-branch observation").
func Classify(in PairInput) Classification {
	degraded := in.DefaultBranch == ""

	reached, reachedBasis, reachedRef := computeDefaultBranchObservation(in, degraded)
	outcome, basis, ref := matchRules(in, degraded, reached)

	if degraded {
		basis = BasisDefaultBranchUnknown
		// delivery_ref is NULL for every degraded row, not just rules 3-6
		// (spec "Classification": degraded is its own category regardless of
		// which rule matched) — rule 1 still runs unconditionally above and
		// would otherwise leak a real PR URL into a degraded row.
		ref = ""
	}

	return Classification{
		Outcome:               outcome,
		Basis:                 basis,
		Ref:                   refPtr(ref),
		Rank:                  Rank(outcome, basis),
		ReachedDefault:        reached,
		ReachedBasis:          reachedBasis,
		ReachedRef:            reachedRef,
		ObservedBranchCommits: observedBranchCommits(in.Snapshots),
	}
}

// computeDefaultBranchObservation applies the basis precedence table in
// spec "Default-branch observation": provider_pr_merged (1, strongest),
// default_branch_commit (2), ancestor_of_default (3). All three steps are
// suspended when degraded (spec "Degraded evaluation").
func computeDefaultBranchObservation(in PairInput, degraded bool) (bool, ReachedBasis, string) {
	if degraded {
		return false, "", ""
	}
	if ref, ok := selectProviderRef(in.Providers, func(p ProviderRequest) bool {
		return p.BaseBranch == in.DefaultBranch
	}); ok {
		return true, ReachedBasisProviderPRMerged, ref
	}
	if ref := selectDefaultBranchCommitRef(in.Snapshots, in.DefaultBranch); ref != "" {
		return true, ReachedBasisDefaultBranchCommit, ref
	}
	if in.Ancestry.Attempted && !in.Ancestry.Errored && in.Ancestry.Positive {
		return true, ReachedBasisAncestorOfDefault, in.Ancestry.Commit
	}
	return false, "", ""
}

// matchRules applies the six classification rules in order; the first
// match wins. Rule 1 runs even when degraded (whether a pull request
// merged does not depend on knowing the default branch); rules 2 and the
// reached-based half of rule 3 naturally fall through when degraded
// because branchMatches never matches an empty DefaultBranch and reached
// is forced false by computeDefaultBranchObservation.
func matchRules(in PairInput, degraded bool, reached bool) (Outcome, Basis, string) {
	if ref, ok := selectProviderRef(in.Providers, nil); ok {
		return OutcomePRMerge, BasisProviderPRMerged, ref
	}
	if !degraded {
		if ref := selectDefaultBranchCommitRef(in.Snapshots, in.DefaultBranch); ref != "" {
			return OutcomeDirectCommit, BasisDefaultBranchCommit, ref
		}
	}
	if len(in.Snapshots) > 0 {
		if maxAhead(in.Snapshots) > 0 || anySessionHasMovingHead(in.Snapshots, nil) {
			if reached {
				return OutcomeUnknown, BasisReachedDefaultUnattributed, ""
			}
			return OutcomeUnknown, BasisBranchCommitsUnmerged, ""
		}
		return OutcomeNoDeliveryObserved, BasisNoCommitsObserved, ""
	}
	if len(in.Providers) > 0 {
		return OutcomeUnknown, BasisProviderPRUnmerged, ""
	}
	return OutcomeUnknown, BasisNoObservations, ""
}

// selectDefaultBranchCommitRef implements rule 2's predicate and the
// Ordering "direct_commit ref" rule together: at least one session with
// >=2 distinct non-empty heads on the default branch, restricted (R5-F5,
// same reasoning as SelectAncestryHead) to the session(s) that earned it,
// then greatest created_at, tie broken by lexicographically greatest
// head_commit.
func selectDefaultBranchCommitRef(snapshots []Snapshot, defaultBranch string) string {
	moving := movingSessions(snapshots, &defaultBranch)
	if len(moving) == 0 {
		return ""
	}
	return selectHeadCommit(snapshots, func(s Snapshot) bool {
		return branchMatches(s.Branch, defaultBranch) && moving[s.SessionID]
	})
}

// branchMatches is the exact, case-sensitive branch comparison spec
// "Classification" requires: an empty defaultBranch never matches any
// branch, even a snapshot whose own branch column is also empty.
func branchMatches(branch, defaultBranch string) bool {
	return defaultBranch != "" && branch == defaultBranch
}

// normalizeHead applies the document-wide head_commit normalization: NULL
// (never reaches Go), empty, or whitespace-only is not a distinct value.
func normalizeHead(raw string) string {
	return strings.TrimSpace(raw)
}

// normalizeAhead floors a NULL or negative ahead value at 0.
func normalizeAhead(raw *int) int {
	if raw == nil || *raw < 0 {
		return 0
	}
	return *raw
}

// distinctNonEmptyHeadsPerSession groups snapshots by SessionID and,
// when branchFilter is non-nil, restricts to snapshots whose Branch
// equals *branchFilter (via branchMatches, so an empty branchFilter
// matches nothing). Returns the set of distinct non-empty HeadCommit
// values per session.
func distinctNonEmptyHeadsPerSession(snapshots []Snapshot, branchFilter *string) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, s := range snapshots {
		if branchFilter != nil && !branchMatches(s.Branch, *branchFilter) {
			continue
		}
		head := normalizeHead(s.HeadCommit)
		if head == "" {
			continue
		}
		if out[s.SessionID] == nil {
			out[s.SessionID] = map[string]bool{}
		}
		out[s.SessionID][head] = true
	}
	return out
}

// movingSessions returns the set of session IDs with two or more distinct
// non-empty heads (optionally restricted to branchFilter) — the
// precondition-satisfying sessions spec "Head movement is a per-session
// fact" describes. Distinct heads are never pooled across sessions.
func movingSessions(snapshots []Snapshot, branchFilter *string) map[string]bool {
	out := map[string]bool{}
	for sid, heads := range distinctNonEmptyHeadsPerSession(snapshots, branchFilter) {
		if len(heads) >= 2 {
			out[sid] = true
		}
	}
	return out
}

// anySessionHasMovingHead is movingSessions's boolean form, used by rule
// 3's predicate.
func anySessionHasMovingHead(snapshots []Snapshot, branchFilter *string) bool {
	return len(movingSessions(snapshots, branchFilter)) > 0
}

// selectHeadCommit picks the ref commit per Ordering: among snapshots
// satisfying keep, those with non-empty HeadCommit (filtered before the
// created_at comparison, so a newer empty-headed snapshot can never
// displace a real commit), greatest CreatedAt, ties broken by
// lexicographically greatest HeadCommit. Returns "" if no snapshot
// qualifies.
func selectHeadCommit(snapshots []Snapshot, keep func(Snapshot) bool) string {
	var bestHead string
	var bestAt time.Time
	found := false
	for _, s := range snapshots {
		if !keep(s) {
			continue
		}
		head := normalizeHead(s.HeadCommit)
		if head == "" {
			continue
		}
		if !found || s.CreatedAt.After(bestAt) || (s.CreatedAt.Equal(bestAt) && head > bestHead) {
			bestHead, bestAt, found = head, s.CreatedAt, true
		}
	}
	return bestHead
}

// selectProviderRef implements the shared provider tiebreak (spec
// "Ordering", pr_merge ref rule / default-branch observation ref rule)
// over the subset of merged, non-detached rows satisfying filter (nil =
// no filter, i.e. rule 1's ref selection). Earliest merge instant, then
// request number ascending, then provider name ascending
// (azure_devops, github, gitlab), then that provider's scope column
// ascending.
func selectProviderRef(prs []ProviderRequest, filter func(ProviderRequest) bool) (string, bool) {
	var best ProviderRequest
	found := false
	for _, p := range prs {
		if !p.Merged || p.Detached {
			continue
		}
		if filter != nil && !filter(p) {
			continue
		}
		if !found || providerLess(p, best) {
			best, found = p, true
		}
	}
	return best.URL, found
}

func providerLess(a, b ProviderRequest) bool {
	if !a.MergeInstant.Equal(b.MergeInstant) {
		return a.MergeInstant.Before(b.MergeInstant)
	}
	if a.RequestNumber != b.RequestNumber {
		return a.RequestNumber < b.RequestNumber
	}
	if providerOrder[a.Provider] != providerOrder[b.Provider] {
		return providerOrder[a.Provider] < providerOrder[b.Provider]
	}
	return a.ScopeValue < b.ScopeValue
}

// maxAhead returns the greatest normalized ahead across snapshots, or 0
// for an empty slice (callers must not treat that 0 as "no snapshots" —
// they check len(in.Snapshots) separately, matching observedBranchCommits
// which returns nil, not 0, for an empty set).
func maxAhead(snapshots []Snapshot) int {
	max := 0
	for _, s := range snapshots {
		if a := normalizeAhead(s.Ahead); a > max {
			max = a
		}
	}
	return max
}

// observedBranchCommits is the incoming value for the high-water column:
// the greatest normalized ahead, or nil when the pair has no snapshots at
// all (spec "Ordering": the greatest of an empty set is not zero).
func observedBranchCommits(snapshots []Snapshot) *int {
	if len(snapshots) == 0 {
		return nil
	}
	v := maxAhead(snapshots)
	return &v
}

func refPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
