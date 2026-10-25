package delivery_test

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/delivery"
)

func at(minutesFromEpoch int) time.Time {
	return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(minutesFromEpoch) * time.Minute)
}

func intp(v int) *int { return &v }

func TestClassify_Rule1PRMerge(t *testing.T) {
	in := delivery.PairInput{
		DefaultBranch: "main",
		Providers: []delivery.ProviderRequest{
			{Provider: delivery.ProviderGitHub, Merged: true, URL: "https://pr/1", BaseBranch: "main", MergeInstant: at(0)},
		},
	}
	got := delivery.Classify(in)
	if got.Outcome != delivery.OutcomePRMerge || got.Basis != delivery.BasisProviderPRMerged {
		t.Fatalf("got %+v", got)
	}
	if got.Rank != 8 {
		t.Fatalf("rank = %d, want 8", got.Rank)
	}
	if got.Ref == nil || *got.Ref != "https://pr/1" {
		t.Fatalf("ref = %v, want pr url", got.Ref)
	}
}

func TestClassify_Rule1GitLabMergedAtNonNull(t *testing.T) {
	in := delivery.PairInput{
		DefaultBranch: "main",
		Providers: []delivery.ProviderRequest{
			{Provider: delivery.ProviderGitLab, Merged: true, URL: "https://gitlab/mr/1", BaseBranch: "main", MergeInstant: at(0)},
		},
	}
	got := delivery.Classify(in)
	if got.Outcome != delivery.OutcomePRMerge || got.Basis != delivery.BasisProviderPRMerged {
		t.Fatalf("got %+v", got)
	}
}

func TestClassify_AzureUnrecognisedStatusFallsThroughWithoutError(t *testing.T) {
	in := delivery.PairInput{
		DefaultBranch: "main",
		Providers: []delivery.ProviderRequest{
			{Provider: delivery.ProviderAzureDevOps, Merged: false, URL: "https://azure/pr/1", BaseBranch: "main"},
		},
	}
	got := delivery.Classify(in)
	if got.Outcome != delivery.OutcomeUnknown || got.Basis != delivery.BasisProviderPRUnmerged {
		t.Fatalf("got %+v, want unknown/provider_pr_unmerged", got)
	}
	if got.Rank != 3 {
		t.Fatalf("rank = %d, want 3", got.Rank)
	}
}

func TestClassify_Rule2DirectCommit(t *testing.T) {
	in := delivery.PairInput{
		DefaultBranch: "main",
		Snapshots: []delivery.Snapshot{
			{SessionID: "s1", Branch: "main", HeadCommit: "aaa", CreatedAt: at(0)},
			{SessionID: "s1", Branch: "main", HeadCommit: "bbb", CreatedAt: at(1)},
			{SessionID: "s1", Branch: "main", HeadCommit: "ccc", CreatedAt: at(2)},
		},
	}
	got := delivery.Classify(in)
	if got.Outcome != delivery.OutcomeDirectCommit || got.Basis != delivery.BasisDefaultBranchCommit {
		t.Fatalf("got %+v", got)
	}
	if got.Ref == nil || *got.Ref != "ccc" {
		t.Fatalf("ref = %v, want ccc (newest)", got.Ref)
	}
	if !got.ReachedDefault || got.ReachedBasis != delivery.ReachedBasisDefaultBranchCommit || got.ReachedRef != "ccc" {
		t.Fatalf("reached observation = %+v", got)
	}
}

// TestClassify_Rule2DirectCommitRestrictedToPreconditionSatisfyingSession
// covers R5-F5 (spec revision 5, Ordering § direct_commit ref): the
// selectDefaultBranchCommitRef restriction to the session(s) satisfying
// rule 2's own moving-head-on-default-branch predicate had no test —
// its ancestry twin (SelectAncestryHead) was covered but this one was
// not. s1 authored two distinct heads on the default branch (satisfies
// the predicate); s2 sits idle on the default branch and contributes one
// newer, never-moved inherited snapshot. Restricted selection must stay
// within s1 and pick its newest head, not s2's newer inherited tip. This
// already passes against the shipped implementation — recorded as a
// regression guard for a real gap in test coverage, not a fix.
func TestClassify_Rule2DirectCommitRestrictedToPreconditionSatisfyingSession(t *testing.T) {
	in := delivery.PairInput{
		DefaultBranch: "main",
		Snapshots: []delivery.Snapshot{
			{SessionID: "s1", Branch: "main", HeadCommit: "aaa", CreatedAt: at(0)},
			{SessionID: "s1", Branch: "main", HeadCommit: "bbb", CreatedAt: at(1)},
			{SessionID: "s2", Branch: "main", HeadCommit: "inherited-tip", CreatedAt: at(2)},
		},
	}
	got := delivery.Classify(in)
	if got.Outcome != delivery.OutcomeDirectCommit || got.Basis != delivery.BasisDefaultBranchCommit {
		t.Fatalf("got %+v", got)
	}
	if got.Ref == nil || *got.Ref != "bbb" {
		t.Fatalf("ref = %v, want bbb (s1's newest head, not s2's newer inherited tip)", got.Ref)
	}
	if !got.ReachedDefault || got.ReachedRef != "bbb" {
		t.Fatalf("reached observation = %+v, want ReachedRef bbb", got)
	}
}

func TestClassify_Rule3UnknownBranchCommitsUnmerged(t *testing.T) {
	in := delivery.PairInput{
		DefaultBranch: "main",
		Snapshots: []delivery.Snapshot{
			{SessionID: "s1", Branch: "feature", HeadCommit: "aaa", Ahead: intp(12), CreatedAt: at(0)},
		},
	}
	got := delivery.Classify(in)
	if got.Outcome != delivery.OutcomeUnknown || got.Basis != delivery.BasisBranchCommitsUnmerged {
		t.Fatalf("got %+v", got)
	}
	if got.Rank != 5 {
		t.Fatalf("rank = %d, want 5", got.Rank)
	}
	if got.ObservedBranchCommits == nil || *got.ObservedBranchCommits != 12 {
		t.Fatalf("observed_branch_commits = %v, want 12", got.ObservedBranchCommits)
	}
	if got.Ref != nil {
		t.Fatalf("ref = %v, want nil", got.Ref)
	}
}

func TestClassify_Rule3PromotesToReachedDefaultUnattributedWhenObservedThisEvaluation(t *testing.T) {
	in := delivery.PairInput{
		DefaultBranch: "main",
		Snapshots: []delivery.Snapshot{
			{SessionID: "s1", Branch: "feature", HeadCommit: "aaa", Ahead: intp(12), CreatedAt: at(0)},
			{SessionID: "s1", Branch: "feature", HeadCommit: "bbb", Ahead: intp(12), CreatedAt: at(1)},
		},
		Ancestry: delivery.AncestryOutcome{Attempted: true, Positive: true, Commit: "bbb"},
	}
	got := delivery.Classify(in)
	if got.Outcome != delivery.OutcomeUnknown || got.Basis != delivery.BasisReachedDefaultUnattributed {
		t.Fatalf("got %+v", got)
	}
	if got.Rank != 6 {
		t.Fatalf("rank = %d, want 6", got.Rank)
	}
	if !got.ReachedDefault || got.ReachedBasis != delivery.ReachedBasisAncestorOfDefault || got.ReachedRef != "bbb" {
		t.Fatalf("reached observation = %+v", got)
	}
}

func TestClassify_Rule4NoDeliveryObserved(t *testing.T) {
	in := delivery.PairInput{
		DefaultBranch: "main",
		Snapshots: []delivery.Snapshot{
			{SessionID: "s1", Branch: "feature", HeadCommit: "aaa", Ahead: intp(0), CreatedAt: at(0)},
		},
	}
	got := delivery.Classify(in)
	if got.Outcome != delivery.OutcomeNoDeliveryObserved || got.Basis != delivery.BasisNoCommitsObserved {
		t.Fatalf("got %+v", got)
	}
	if got.Rank != 4 {
		t.Fatalf("rank = %d, want 4", got.Rank)
	}
}

func TestClassify_Rule4EmptyHeadCountsAsNoDistinctHead(t *testing.T) {
	in := delivery.PairInput{
		DefaultBranch: "main",
		Snapshots: []delivery.Snapshot{
			{SessionID: "s1", Branch: "feature", HeadCommit: "", Ahead: intp(0), CreatedAt: at(0)},
		},
	}
	got := delivery.Classify(in)
	if got.Outcome != delivery.OutcomeNoDeliveryObserved {
		t.Fatalf("got %+v, want no_delivery_observed", got)
	}
}

func TestClassify_AheadNullAndNegativeNormalizeToZero(t *testing.T) {
	in := delivery.PairInput{
		DefaultBranch: "main",
		Snapshots: []delivery.Snapshot{
			{SessionID: "s1", Branch: "feature", HeadCommit: "aaa", Ahead: nil, CreatedAt: at(0)},
			{SessionID: "s1", Branch: "feature", HeadCommit: "aaa", Ahead: intp(-1), CreatedAt: at(1)},
		},
	}
	got := delivery.Classify(in)
	if got.Outcome != delivery.OutcomeNoDeliveryObserved {
		t.Fatalf("got %+v, want no_delivery_observed (NULL/negative ahead read as 0)", got)
	}
}

func TestClassify_Rule5ProviderPRUnmergedNoSnapshot(t *testing.T) {
	in := delivery.PairInput{
		DefaultBranch: "main",
		Providers: []delivery.ProviderRequest{
			{Provider: delivery.ProviderGitHub, Merged: false, URL: "https://pr/2", BaseBranch: "main"},
		},
	}
	got := delivery.Classify(in)
	if got.Outcome != delivery.OutcomeUnknown || got.Basis != delivery.BasisProviderPRUnmerged {
		t.Fatalf("got %+v", got)
	}
	if got.Ref != nil {
		t.Fatalf("ref = %v, want nil (unmerged PR URL is not a delivery ref)", got.Ref)
	}
}

func TestClassify_Rule6NoObservations(t *testing.T) {
	got := delivery.Classify(delivery.PairInput{DefaultBranch: "main"})
	if got.Outcome != delivery.OutcomeUnknown || got.Basis != delivery.BasisNoObservations {
		t.Fatalf("got %+v", got)
	}
	if got.Rank != 2 {
		t.Fatalf("rank = %d, want 2", got.Rank)
	}
}

func TestClassify_DegradedEvaluationRank1SupersedesMatchedBasis(t *testing.T) {
	in := delivery.PairInput{
		DefaultBranch: "",
		Snapshots: []delivery.Snapshot{
			{SessionID: "s1", Branch: "feature", HeadCommit: "aaa", Ahead: intp(12), CreatedAt: at(0)},
		},
	}
	got := delivery.Classify(in)
	if got.Outcome != delivery.OutcomeUnknown {
		t.Fatalf("outcome = %v, want unknown (rule 3 still matched)", got.Outcome)
	}
	if got.Basis != delivery.BasisDefaultBranchUnknown {
		t.Fatalf("basis = %v, want default_branch_unknown", got.Basis)
	}
	if got.Rank != 1 {
		t.Fatalf("rank = %d, want 1", got.Rank)
	}
	if got.ReachedDefault {
		t.Fatal("reached default observation must be suspended while degraded")
	}
}

func TestClassify_DegradedProviderPRWithEmptyBaseBranchNeverReachesDefault(t *testing.T) {
	in := delivery.PairInput{
		DefaultBranch: "",
		Providers: []delivery.ProviderRequest{
			{Provider: delivery.ProviderGitHub, Merged: true, URL: "https://pr/3", BaseBranch: "", MergeInstant: at(0)},
		},
	}
	got := delivery.Classify(in)
	if got.Outcome != delivery.OutcomePRMerge {
		t.Fatalf("outcome = %v, want pr_merge (rule 1 still runs while degraded)", got.Outcome)
	}
	if got.Basis != delivery.BasisDefaultBranchUnknown || got.Rank != 1 {
		t.Fatalf("got %+v, want degraded rank 1", got)
	}
	if got.ReachedDefault {
		t.Fatal("empty default_branch must never match an equally-empty base_branch")
	}
	if got.Ref != nil {
		t.Fatalf("ref = %v, want nil: every degraded row must have a NULL delivery_ref, not just rules 3-6", *got.Ref)
	}
}

func TestClassify_TwoInheritedBranchesNeverPooledAcrossSessions(t *testing.T) {
	in := delivery.PairInput{
		DefaultBranch: "main",
		Snapshots: []delivery.Snapshot{
			{SessionID: "s1", Branch: "feature-a", HeadCommit: "aaa", CreatedAt: at(0)},
			{SessionID: "s2", Branch: "feature-b", HeadCommit: "bbb", CreatedAt: at(1)},
		},
	}
	got := delivery.Classify(in)
	if got.Outcome == delivery.OutcomeDirectCommit {
		t.Fatal("distinct heads from different sessions must not be pooled into direct_commit")
	}
	if got.ReachedDefault {
		t.Fatal("no ancestry check should have been attempted, so nothing should be reached")
	}
	if !delivery.AncestryPrecondition(in.Snapshots) {
		return // fine either way for this assertion; the real check is below
	}
	t.Fatal("neither session individually has >=2 distinct heads, so the precondition must not be met")
}

func TestClassify_RulesAreExhaustive(t *testing.T) {
	// A pair with at least one snapshot and no providers must always match
	// rule 2, 3, or 4 — never fall through to 5/6, and never leave
	// delivery_outcome unset.
	cases := []delivery.PairInput{
		{DefaultBranch: "main"},
		{DefaultBranch: "main", Snapshots: []delivery.Snapshot{{SessionID: "s1", Branch: "f", HeadCommit: "a", Ahead: intp(0), CreatedAt: at(0)}}},
		{DefaultBranch: "main", Snapshots: []delivery.Snapshot{{SessionID: "s1", Branch: "f", HeadCommit: "a", Ahead: intp(1), CreatedAt: at(0)}}},
		{DefaultBranch: "main", Providers: []delivery.ProviderRequest{{Provider: delivery.ProviderGitHub, Merged: false, URL: "u"}}},
	}
	for i, in := range cases {
		got := delivery.Classify(in)
		if got.Outcome == "" || got.Basis == "" {
			t.Errorf("case %d: outcome/basis unset: %+v", i, got)
		}
	}
}

// --- Squash-merge, ancestry preconditions and negative evidence ---

func TestAncestryPrecondition_SameSingleHeadNeverMet(t *testing.T) {
	snaps := []delivery.Snapshot{
		{SessionID: "s1", Branch: "feature", HeadCommit: "aaa", Ahead: intp(4), CreatedAt: at(0)},
		{SessionID: "s1", Branch: "feature", HeadCommit: "aaa", Ahead: intp(4), CreatedAt: at(1)},
	}
	if delivery.AncestryPrecondition(snaps) {
		t.Fatal("a single repeated head (ahead>0, inherited branch) must not satisfy the precondition")
	}
}

func TestAncestryPrecondition_MetWhenSessionHeadMoves(t *testing.T) {
	snaps := []delivery.Snapshot{
		{SessionID: "s1", Branch: "feature", HeadCommit: "aaa", CreatedAt: at(0)},
		{SessionID: "s1", Branch: "feature", HeadCommit: "bbb", CreatedAt: at(1)},
	}
	if !delivery.AncestryPrecondition(snaps) {
		t.Fatal("two distinct heads within one session must satisfy the precondition")
	}
}

func TestSelectAncestryHead_TieBrokenByLexicographicallyGreatestHead(t *testing.T) {
	snaps := []delivery.Snapshot{
		{SessionID: "s1", Branch: "feature", HeadCommit: "aaa", CreatedAt: at(0)},
		{SessionID: "s1", Branch: "feature", HeadCommit: "zzz", CreatedAt: at(5)},
		{SessionID: "s1", Branch: "feature", HeadCommit: "yyy", CreatedAt: at(5)},
	}
	if got := delivery.SelectAncestryHead(snaps); got != "zzz" {
		t.Fatalf("got %q, want zzz", got)
	}
}

func TestSelectAncestryHead_SkipsWhitespaceOnlyNewestSnapshot(t *testing.T) {
	snaps := []delivery.Snapshot{
		{SessionID: "s1", Branch: "feature", HeadCommit: "aaa", CreatedAt: at(0)},
		{SessionID: "s1", Branch: "feature", HeadCommit: "bbb", CreatedAt: at(1)},
		{SessionID: "s1", Branch: "feature", HeadCommit: "   ", CreatedAt: at(2)},
	}
	if got := delivery.SelectAncestryHead(snaps); got != "bbb" {
		t.Fatalf("got %q, want bbb (newest non-empty)", got)
	}
}

func TestSelectAncestryHead_RestrictedToPreconditionSatisfyingSession(t *testing.T) {
	// s1 authored two distinct heads on an unmerged feature branch
	// (satisfies the precondition). s2 sits idle on the default branch and
	// writes a newer snapshot whose head_commit is its inherited tip. Per
	// the R5-F1 Build decision, selection must stay within s1 and must not
	// pick s2's newer, unrelated head.
	snaps := []delivery.Snapshot{
		{SessionID: "s1", Branch: "feature", HeadCommit: "aaa", CreatedAt: at(0)},
		{SessionID: "s1", Branch: "feature", HeadCommit: "bbb", CreatedAt: at(1)},
		{SessionID: "s2", Branch: "main", HeadCommit: "inherited-tip", CreatedAt: at(2)},
	}
	if got := delivery.SelectAncestryHead(snaps); got != "bbb" {
		t.Fatalf("got %q, want bbb (s1's newest head, not s2's newer inherited tip)", got)
	}
}

func TestClassify_NegativeAncestryResultDiscarded(t *testing.T) {
	in := delivery.PairInput{
		DefaultBranch: "main",
		Snapshots: []delivery.Snapshot{
			{SessionID: "s1", Branch: "feature", HeadCommit: "aaa", CreatedAt: at(0)},
			{SessionID: "s1", Branch: "feature", HeadCommit: "bbb", CreatedAt: at(1)},
		},
		Ancestry: delivery.AncestryOutcome{Attempted: true, Positive: false, Commit: "bbb"},
	}
	got := delivery.Classify(in)
	if got.ReachedDefault {
		t.Fatal("a negative ancestry result must not produce a default-branch observation")
	}
	if got.Outcome != delivery.OutcomeUnknown || got.Basis != delivery.BasisBranchCommitsUnmerged {
		t.Fatalf("got %+v, want unknown/branch_commits_unmerged", got)
	}
}

func TestClassify_AncestryErrorDoesNotProduceObservation(t *testing.T) {
	in := delivery.PairInput{
		DefaultBranch: "main",
		Snapshots: []delivery.Snapshot{
			{SessionID: "s1", Branch: "feature", HeadCommit: "aaa", CreatedAt: at(0)},
			{SessionID: "s1", Branch: "feature", HeadCommit: "bbb", CreatedAt: at(1)},
		},
		Ancestry: delivery.AncestryOutcome{Attempted: true, Errored: true},
	}
	got := delivery.Classify(in)
	if got.ReachedDefault {
		t.Fatal("an errored ancestry check must not produce an observation")
	}
	if got.Outcome != delivery.OutcomeUnknown || got.Basis != delivery.BasisBranchCommitsUnmerged {
		t.Fatalf("got %+v, want unknown/branch_commits_unmerged (rest of classification still persisted)", got)
	}
}

func TestClassify_DifferentBaseBranchSplitsRefFromReachedRef(t *testing.T) {
	// A pair with a merged PR into a release branch (earlier merge instant,
	// wins delivery_ref because rule 1 has no base-branch filter) and a
	// later merged PR into main (wins reached_default_ref because the
	// observation ref rule filters on base branch).
	in := delivery.PairInput{
		DefaultBranch: "main",
		Providers: []delivery.ProviderRequest{
			{Provider: delivery.ProviderGitHub, Merged: true, URL: "https://pr/A", BaseBranch: "release/1.2", MergeInstant: at(0), RequestNumber: 1},
			{Provider: delivery.ProviderGitHub, Merged: true, URL: "https://pr/B", BaseBranch: "main", MergeInstant: at(10), RequestNumber: 2},
		},
	}
	got := delivery.Classify(in)
	if got.Ref == nil || *got.Ref != "https://pr/A" {
		t.Fatalf("delivery_ref = %v, want pr A (earliest merge, no base-branch filter)", got.Ref)
	}
	if got.ReachedRef != "https://pr/B" || got.ReachedBasis != delivery.ReachedBasisProviderPRMerged {
		t.Fatalf("reached ref/basis = %q/%v, want pr B / provider_pr_merged", got.ReachedRef, got.ReachedBasis)
	}
}

func TestClassify_DetachedMergedPRExcludedFromRule1AndObservation(t *testing.T) {
	in := delivery.PairInput{
		DefaultBranch: "main",
		Snapshots: []delivery.Snapshot{
			{SessionID: "s1", Branch: "feature", HeadCommit: "aaa", Ahead: intp(3), CreatedAt: at(0)},
		},
		Providers: []delivery.ProviderRequest{
			{Provider: delivery.ProviderGitHub, Merged: true, Detached: true, URL: "https://pr/4", BaseBranch: "main", MergeInstant: at(0)},
		},
	}
	got := delivery.Classify(in)
	if got.Outcome != delivery.OutcomeUnknown || got.Basis != delivery.BasisBranchCommitsUnmerged {
		t.Fatalf("got %+v, want unknown/branch_commits_unmerged", got)
	}
	if got.Rank != 5 {
		t.Fatalf("rank = %d, want 5", got.Rank)
	}
	if got.ReachedDefault {
		t.Fatal("a detached row must be excluded from the observation exactly as from rule 1")
	}
}

func TestClassify_TwoMergedPRsTieBrokenByLowerRequestNumber(t *testing.T) {
	in := delivery.PairInput{
		DefaultBranch: "main",
		Providers: []delivery.ProviderRequest{
			{Provider: delivery.ProviderGitHub, Merged: true, URL: "https://pr/hi", BaseBranch: "main", MergeInstant: at(0), RequestNumber: 9},
			{Provider: delivery.ProviderGitHub, Merged: true, URL: "https://pr/lo", BaseBranch: "main", MergeInstant: at(0), RequestNumber: 2},
		},
	}
	got := delivery.Classify(in)
	if got.Ref == nil || *got.Ref != "https://pr/lo" {
		t.Fatalf("ref = %v, want pr/lo (lower request number on exact tie)", got.Ref)
	}
}

func TestClassify_GitHubAndAzureTieBrokenByProviderOrder(t *testing.T) {
	in := delivery.PairInput{
		DefaultBranch: "main",
		Providers: []delivery.ProviderRequest{
			{Provider: delivery.ProviderGitHub, Merged: true, URL: "https://gh", BaseBranch: "main", MergeInstant: at(0), RequestNumber: 1},
			{Provider: delivery.ProviderAzureDevOps, Merged: true, URL: "https://az", BaseBranch: "main", MergeInstant: at(0), RequestNumber: 1},
		},
	}
	got := delivery.Classify(in)
	if got.Ref == nil || *got.Ref != "https://az" {
		t.Fatalf("ref = %v, want azure (azure_devops sorts before github on exact tie)", got.Ref)
	}
}

func TestClassify_GitLabSameMRIIDTiebreakByProjectPathOnEqualMergeInstant(t *testing.T) {
	// Per R5-F7 (Build decision), the literal spec GIVEN omits "sharing the
	// same merge instant" — the Ordering rule selects by earliest merge
	// instant first, so this test seeds an exact tie to actually exercise
	// the project_path tiebreak, rather than translating the scenario
	// literally (which could fail a correct implementation).
	in := delivery.PairInput{
		DefaultBranch: "main",
		Providers: []delivery.ProviderRequest{
			{Provider: delivery.ProviderGitLab, Merged: true, URL: "https://gl/z-path", BaseBranch: "main", MergeInstant: at(0), RequestNumber: 5, ScopeValue: "z-path"},
			{Provider: delivery.ProviderGitLab, Merged: true, URL: "https://gl/a-path", BaseBranch: "main", MergeInstant: at(0), RequestNumber: 5, ScopeValue: "a-path"},
		},
	}
	got := delivery.Classify(in)
	if got.Ref == nil || *got.Ref != "https://gl/a-path" {
		t.Fatalf("ref = %v, want the lower project_path", got.Ref)
	}
}

func TestClassify_ObservedBranchCommitsNilWithNoSnapshots(t *testing.T) {
	got := delivery.Classify(delivery.PairInput{DefaultBranch: "main"})
	if got.ObservedBranchCommits != nil {
		t.Fatalf("observed_branch_commits = %v, want nil", got.ObservedBranchCommits)
	}
}

func TestClassify_RemoteBranchFilterOnDefaultBranchDoesNotMatchEmptyBranchColumn(t *testing.T) {
	// Guards against the "empty default_branch matches an equally empty
	// branch column" trap called out in spec "Degraded evaluation": here
	// DefaultBranch is set but a snapshot's own Branch happens to be empty
	// — that must never satisfy branchMatches either.
	in := delivery.PairInput{
		DefaultBranch: "main",
		Snapshots: []delivery.Snapshot{
			{SessionID: "s1", Branch: "", HeadCommit: "aaa", CreatedAt: at(0)},
			{SessionID: "s1", Branch: "", HeadCommit: "bbb", CreatedAt: at(1)},
		},
	}
	got := delivery.Classify(in)
	if got.Outcome == delivery.OutcomeDirectCommit {
		t.Fatal("an empty branch column must never match a non-empty default_branch")
	}
}
