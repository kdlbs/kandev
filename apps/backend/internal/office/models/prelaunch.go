package models

import (
	"fmt"
	"time"
)

// PreLaunchDecision is the pre-launch admission evaluator's outcome for one
// candidate run, per REQ-OFFICE-BUDGET-006. Declared here (rather than in
// internal/office/costs, where it is computed) so internal/office/service
// can declare a BudgetEvaluator interface method returning it without
// importing internal/office/costs — the same cross-package sharing this
// file's other types (SpendWindow, BudgetPolicy) already use.
type PreLaunchDecision string

const (
	PreLaunchDecisionLaunch               PreLaunchDecision = "launch"
	PreLaunchDecisionBlockedByLimit       PreLaunchDecision = "blocked_by_limit"
	PreLaunchDecisionBlockedByDegradation PreLaunchDecision = "blocked_by_degradation"
)

// PreLaunchPolicyResult is one applicable policy's full pre-launch
// evaluation, kept for observability.
type PreLaunchPolicyResult struct {
	PolicyID           string
	ScopeType          BudgetScopeType
	Period             BudgetPeriod
	ActionOnExceed     BudgetActionOnExceed
	CreatedAt          time.Time
	Skipped            bool
	SkipIssues         PolicyValidationIssues
	PricedSubcents     int64
	LimitSubcents      int64
	Degraded           bool
	LimitExceeded      bool
	DegradationBlocked bool
	// IsDefault is true when this result is the built-in default ceiling,
	// never a stored office_budget_policies row. PolicyID is "" in that case
	// (AC-OFFICE-BUDGET-003.7's stable identifier distinct from any policy row).
	IsDefault bool
}

// PreLaunchResult is the pre-launch evaluator's full output for one run.
type PreLaunchResult struct {
	Decision       PreLaunchDecision
	DecidingPolicy *PreLaunchPolicyResult
	Policies       []PreLaunchPolicyResult
	// WorkspaceDailyBlockingSuperseded is true when a workspace-scoped,
	// non-skipped, blocking-capable (pause_agent/block_new_tasks) daily
	// policy exists among the surviving set, independent of whether it
	// fired: AC-OFFICE-BUDGET-003.4 supersedes the built-in default by
	// existing, not by blocking.
	WorkspaceDailyBlockingSuperseded bool
}

// PolicyValidationIssues enumerates every reason classifyStoredPolicy found
// a stored policy row cannot be evaluated as written (AC-OFFICE-BUDGET-002.5,
// -002.11, -002.14). Declared here for the same cross-package reason as
// PreLaunchPolicyResult above.
type PolicyValidationIssues struct {
	UnrecognizedPeriod bool
	NonPositiveLimit   bool
	UnrecognizedScope  bool
	UnrecognizedAction bool
	EmptyScopeID       bool
}

// Any reports whether classifyStoredPolicy found at least one issue.
func (i PolicyValidationIssues) Any() bool {
	return i.UnrecognizedPeriod || i.NonPositiveLimit || i.UnrecognizedScope ||
		i.UnrecognizedAction || i.EmptyScopeID
}

// UnevaluatedPolicyError names the applicable policy a pre-launch evaluation
// attempted, but failed, to evaluate — as opposed to a plain evaluator
// invocation fault (e.g. listing policies itself failing), which carries no
// policy identity. AC-OFFICE-BUDGET-006.1/.4/.5 require the two be
// distinguishable and require the former to name the policy; a caller
// extracts PolicyID via errors.As.
type UnevaluatedPolicyError struct {
	PolicyID string
	Err      error
}

func (e *UnevaluatedPolicyError) Error() string {
	return fmt.Sprintf("unevaluated policy %s: %v", e.PolicyID, e.Err)
}

func (e *UnevaluatedPolicyError) Unwrap() error { return e.Err }
