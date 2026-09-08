// Package delivery implements the task delivery ledger: classifying how,
// and whether, each (task, repository) pair delivered
// (docs/specs/task-delivery-ledger/spec.md).
package delivery

// Outcome is task_delivery_ledger.delivery_outcome. The zero value is used
// only as a "no outcome computed" sentinel in Go; the stored column is
// NULL for a never-evaluated row, never the empty string.
type Outcome string

const (
	OutcomeUnknown            Outcome = "unknown"
	OutcomeNoDeliveryObserved Outcome = "no_delivery_observed"
	OutcomeDirectCommit       Outcome = "direct_commit"
	OutcomePRMerge            Outcome = "pr_merge"
)

// Basis is task_delivery_ledger.delivery_basis. See spec "Basis
// vocabulary".
type Basis string

const (
	BasisProviderPRMerged           Basis = "provider_pr_merged"
	BasisDefaultBranchCommit        Basis = "default_branch_commit"
	BasisReachedDefaultUnattributed Basis = "reached_default_unattributed"
	BasisBranchCommitsUnmerged      Basis = "branch_commits_unmerged"
	BasisNoCommitsObserved          Basis = "no_commits_observed"
	BasisProviderPRUnmerged         Basis = "provider_pr_unmerged"
	BasisNoObservations             Basis = "no_observations"
	BasisDefaultBranchUnknown       Basis = "default_branch_unknown"
)

// ReachedBasis is task_delivery_ledger.reached_default_basis. See spec
// "Basis vocabulary". ReachedBasisPushWebhookDefault is defined for a
// later card; nothing in this package writes it.
type ReachedBasis string

const (
	ReachedBasisProviderPRMerged    ReachedBasis = "provider_pr_merged"
	ReachedBasisDefaultBranchCommit ReachedBasis = "default_branch_commit"
	ReachedBasisAncestorOfDefault   ReachedBasis = "ancestor_of_default"
	ReachedBasisPushWebhookDefault  ReachedBasis = "push_webhook_default"
)

// Provider names, used for the ordering tiebreak (spec "Ordering",
// provider name ascending in the order azure_devops, github, gitlab).
const (
	ProviderAzureDevOps = "azure_devops"
	ProviderGitHub      = "github"
	ProviderGitLab      = "gitlab"
)

var providerOrder = map[string]int{
	ProviderAzureDevOps: 0,
	ProviderGitHub:      1,
	ProviderGitLab:      2,
}

// evidenceRankTable keys a (Outcome, Basis) pair to its rank (2-8). Rank 0
// (never evaluated) is a DB default, never computed here. Rank 1
// (degraded) applies to every (outcome, BasisDefaultBranchUnknown) pair
// and is handled by Rank directly rather than through this table, because
// it is deliberately non-injective (spec "Evidence rank").
var evidenceRankTable = map[Outcome]map[Basis]int{
	OutcomeUnknown: {
		BasisNoObservations:             2,
		BasisProviderPRUnmerged:         3,
		BasisBranchCommitsUnmerged:      5,
		BasisReachedDefaultUnattributed: 6,
	},
	OutcomeNoDeliveryObserved: {
		BasisNoCommitsObserved: 4,
	},
	OutcomeDirectCommit: {
		BasisDefaultBranchCommit: 7,
	},
	OutcomePRMerge: {
		BasisProviderPRMerged: 8,
	},
}

// Rank returns the evidence_rank for (outcome, basis), per spec "Evidence
// rank". Degraded evaluations (basis == BasisDefaultBranchUnknown) always
// rank 1, regardless of which rule matched.
func Rank(outcome Outcome, basis Basis) int {
	if basis == BasisDefaultBranchUnknown {
		return 1
	}
	if m, ok := evidenceRankTable[outcome]; ok {
		if r, ok := m[basis]; ok {
			return r
		}
	}
	return 0
}
