package costs

import "github.com/kandev/kandev/internal/office/models"

// policyValidationIssues is every reason classifyStoredPolicy found to skip
// a stored policy (AC-OFFICE-BUDGET-002.5/.11/.14). All fields are detected
// in the same pass, never short-circuited at the first, so a policy
// violating more than one criterion can be named for all of them in a
// single activity entry (AC-OFFICE-BUDGET-002.13).
type policyValidationIssues struct {
	UnrecognizedPeriod bool
	NonPositiveLimit   bool
	UnrecognizedScope  bool
	UnrecognizedAction bool
	EmptyScopeID       bool
}

// Any reports whether classifyStoredPolicy found at least one issue.
func (i policyValidationIssues) Any() bool {
	return i.UnrecognizedPeriod || i.NonPositiveLimit || i.UnrecognizedScope ||
		i.UnrecognizedAction || i.EmptyScopeID
}

// classifyStoredPolicy implements AC-OFFICE-BUDGET-002.5/.11/.14: a stored
// policy the build cannot evaluate as written — an unrecognized period,
// scope type, or action, a non-positive limit, or an agent/project scope
// with no scope_id — is skipped by the caller rather than evaluated as a
// lifetime window or as matching every run whose corresponding identifier
// is also empty. Unlike validateBudgetPolicyWrite (AC-002.6/.7, which only
// guards new writes), this covers every stored row regardless of how it
// got there — a row written before validation shipped, or by any path that
// bypasses the write API.
func classifyStoredPolicy(policy *models.BudgetPolicy) policyValidationIssues {
	issues := policyValidationIssues{
		UnrecognizedPeriod: !policy.Period.Valid(),
		NonPositiveLimit:   policy.LimitSubcents <= 0,
		UnrecognizedScope:  !policy.ScopeType.Valid(),
		UnrecognizedAction: !policy.ActionOnExceed.Valid(),
	}
	if (policy.ScopeType == models.BudgetScopeAgent || policy.ScopeType == models.BudgetScopeProject) &&
		policy.ScopeID == "" {
		issues.EmptyScopeID = true
	}
	return issues
}
