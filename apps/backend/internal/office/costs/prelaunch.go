package costs

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
)

// windowStart computes the UTC start of period's spend window ending at at,
// per REQ-OFFICE-BUDGET-002 (docs/specs/office/requirements/budget-measurement-integrity.md).
//
// ok is false for any period value not explicitly handled below, including
// one this build has never seen — callers treat that as "skip this policy"
// (AC-OFFICE-BUDGET-002.5), never as an unbounded lifetime window.
func windowStart(period models.BudgetPeriod, at time.Time) (start time.Time, ok bool) {
	u := at.UTC()
	switch period {
	case models.BudgetPeriodDaily:
		return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC), true
	case models.BudgetPeriodMonthly:
		return time.Date(u.Year(), u.Month(), 1, 0, 0, 0, 0, time.UTC), true
	case models.BudgetPeriodYearly:
		return time.Date(u.Year(), time.January, 1, 0, 0, 0, 0, time.UTC), true
	case models.BudgetPeriodTotal:
		return time.Time{}, true
	default:
		return time.Time{}, false
	}
}

// PreLaunchDecision is EvaluatePreLaunch's admission outcome for one
// candidate run, per REQ-OFFICE-BUDGET-006. Aliased from models so
// internal/office/service can declare a BudgetEvaluator interface method
// returning it without importing this package.
type PreLaunchDecision = models.PreLaunchDecision

const (
	PreLaunchDecisionLaunch               = models.PreLaunchDecisionLaunch
	PreLaunchDecisionBlockedByLimit       = models.PreLaunchDecisionBlockedByLimit
	PreLaunchDecisionBlockedByDegradation = models.PreLaunchDecisionBlockedByDegradation
)

// PreLaunchPolicyResult is one applicable policy's full evaluation, kept for
// observability. Activity-entry writing and dedup (AC-OFFICE-BUDGET-002.13)
// live one layer up, in the caller that owns side effects — EvaluatePreLaunch
// itself stays a pure read (AC-OFFICE-BUDGET-006.2). Aliased from models,
// see PreLaunchDecision above.
type PreLaunchPolicyResult = models.PreLaunchPolicyResult

// PreLaunchResult is EvaluatePreLaunch's full output. Aliased from models,
// see PreLaunchDecision above.
type PreLaunchResult = models.PreLaunchResult

// preLaunchApplicablePolicies implements AC-OFFICE-BUDGET-001.15:
// workspace-scoped always applies; agent-scoped only when scope_id matches
// the run's agent; project-scoped only when hasProject and scope_id matches
// the run's project. A policy whose scope_type or scope_id is malformed
// enough that no case below could ever match it, regardless of this run's
// agent or project, is included anyway (scopeUnclassifiable) rather than
// silently excluded: evaluateOnePolicy's classifyStoredPolicy call is what
// actually produces AC-OFFICE-BUDGET-002.14's skip entry, and it only runs
// on policies this function returns.
func preLaunchApplicablePolicies(
	policies []*models.BudgetPolicy, agentInstanceID, projectID string, hasProject bool,
) []*models.BudgetPolicy {
	var out []*models.BudgetPolicy
	for _, p := range policies {
		if scopeUnclassifiable(p) {
			out = append(out, p)
			continue
		}
		switch p.ScopeType {
		case models.BudgetScopeWorkspace:
			out = append(out, p)
		case models.BudgetScopeAgent:
			if p.ScopeID == agentInstanceID {
				out = append(out, p)
			}
		case models.BudgetScopeProject:
			if hasProject && p.ScopeID == projectID {
				out = append(out, p)
			}
		}
	}
	return out
}

// scopeUnclassifiable reports whether p's scope_type or scope_id is
// malformed enough that preLaunchApplicablePolicies' switch above has no
// case that could ever match it, independent of this run's agent or
// project: an unrecognized scope_type, or an agent/project scope with no
// scope_id (which could never equal a real, non-empty agent/project
// identifier either way). These are exactly the two classifyStoredPolicy
// issues that are also applicability-blocking, so without this check they
// would be silently excluded here and classifyStoredPolicy would never run
// on them at all -- AC-OFFICE-BUDGET-002.14 requires them to be skipped
// with an operator-visible entry, not dropped with no trace.
func scopeUnclassifiable(p *models.BudgetPolicy) bool {
	if !p.ScopeType.Valid() {
		return true
	}
	if (p.ScopeType == models.BudgetScopeAgent || p.ScopeType == models.BudgetScopeProject) && p.ScopeID == "" {
		return true
	}
	return false
}

// spendWindowForPolicy resolves the scope-selected spend window for policy's
// own period, ending at the given instant (AC-OFFICE-BUDGET-002.15). windowStart failing is
// unreachable once classifyStoredPolicy has already passed the policy (both
// switch on the same models.BudgetPeriod.Valid() set); erroring instead of
// panicking keeps that invariant enforced by AC-OFFICE-BUDGET-006.1 rather
// than by construction alone.
func (s *CostService) spendWindowForPolicy(
	ctx context.Context, workspaceID string, p *models.BudgetPolicy, at time.Time,
) (models.SpendWindow, error) {
	start, ok := windowStart(p.Period, at)
	if !ok {
		return models.SpendWindow{}, fmt.Errorf("unrecognized period %q for policy %s", p.Period, p.ID)
	}
	hasStart := p.Period != models.BudgetPeriodTotal
	switch p.ScopeType {
	case models.BudgetScopeAgent:
		return s.repo.SpendWindowForAgent(ctx, p.ScopeID, start, hasStart, at)
	case models.BudgetScopeProject:
		return s.repo.SpendWindowForProject(ctx, p.ScopeID, start, hasStart, at)
	default:
		return s.repo.SpendWindowForWorkspace(ctx, workspaceID, start, hasStart, at)
	}
}

// evaluateOnePolicy computes p's PreLaunchPolicyResult: skipped (with
// SkipIssues set) if classifyStoredPolicy finds anything wrong, otherwise
// its spend window, limit test, and degradation test. survives is false for
// a skipped policy -- callers exclude those from ordering/selection and
// default-supersession (AC-OFFICE-BUDGET-002.5/.11/.14).
func (s *CostService) evaluateOnePolicy(
	ctx context.Context, workspaceID string, p *models.BudgetPolicy, at time.Time, provenance shared.RunProvenance,
) (result PreLaunchPolicyResult, survives bool, err error) {
	result = PreLaunchPolicyResult{
		PolicyID:       p.ID,
		ScopeType:      p.ScopeType,
		Period:         p.Period,
		ActionOnExceed: p.ActionOnExceed,
		CreatedAt:      p.CreatedAt,
		LimitSubcents:  p.LimitSubcents,
	}

	if issues := classifyStoredPolicy(p); issues.Any() {
		result.Skipped = true
		result.SkipIssues = issues
		return result, false, nil
	}

	window, werr := s.spendWindowForPolicy(ctx, workspaceID, p, at)
	if werr != nil {
		return PreLaunchPolicyResult{}, false, &models.UnevaluatedPolicyError{PolicyID: p.ID, Err: werr}
	}
	result.PricedSubcents = window.PricedSubcents
	result.Degraded = window.Degraded
	result.LimitExceeded = window.PricedSubcents >= p.LimitSubcents &&
		(p.ActionOnExceed == models.BudgetActionPauseAgent || p.ActionOnExceed == models.BudgetActionBlockNewTasks)
	result.DegradationBlocked = degradationBlocks(window.Degraded, window.PricedSubcents, p.LimitSubcents, provenance)

	return result, true, nil
}

// workspaceDailyBlockingSuperseded implements AC-OFFICE-BUDGET-003.4: true
// when a workspace-scoped, non-skipped, blocking-capable daily policy
// exists among survivors, regardless of whether it fired.
func workspaceDailyBlockingSuperseded(survivors []*PreLaunchPolicyResult) bool {
	for _, r := range survivors {
		if r.ScopeType == models.BudgetScopeWorkspace && r.Period == models.BudgetPeriodDaily &&
			(r.ActionOnExceed == models.BudgetActionPauseAgent || r.ActionOnExceed == models.BudgetActionBlockNewTasks) {
			return true
		}
	}
	return false
}

// selectPreLaunchDecision picks the first (in survivors' already-sorted
// created_at ASC, id ASC order) policy whose limit or degradation test
// fired. Every survivor's tests were computed up front, so this is a
// selection over an already-fully-evaluated slice, never a
// short-circuiting loop (AC-OFFICE-BUDGET-001.9/AC-OFFICE-BUDGET-006.1).
// Within one policy, the limit test is checked before the degradation test:
// LimitExceeded implies DegradationBlocked can also be true for the same
// policy, and that combination still reports BlockedByLimit.
func selectPreLaunchDecision(survivors []*PreLaunchPolicyResult) (PreLaunchDecision, *PreLaunchPolicyResult) {
	for _, r := range survivors {
		switch {
		case r.LimitExceeded:
			return PreLaunchDecisionBlockedByLimit, r
		case r.DegradationBlocked:
			return PreLaunchDecisionBlockedByDegradation, r
		}
	}
	return PreLaunchDecisionLaunch, nil
}

// EvaluatePreLaunch implements REQ-OFFICE-BUDGET-006's two-phase policy
// evaluation: lists and filters applicable policies, classifies/skips
// malformed ones, computes spend and degradation for every surviving policy
// up front (never short-circuiting), then selects the first (by created_at
// ASC, id ASC) whose limit or degradation test fires. It does not call the
// existing CheckBudget/evaluatePolicy post-event path (AC-OFFICE-BUDGET-006.2)
// and has no side effects of its own.
func (s *CostService) EvaluatePreLaunch(
	ctx context.Context,
	workspaceID, agentInstanceID, projectID string,
	hasProject bool,
	provenance shared.RunProvenance,
	at time.Time,
) (PreLaunchResult, error) {
	policies, err := s.repo.ListBudgetPolicies(ctx, workspaceID)
	if err != nil {
		return PreLaunchResult{}, err
	}
	applicable := preLaunchApplicablePolicies(policies, agentInstanceID, projectID, hasProject)

	results := make([]PreLaunchPolicyResult, 0, len(applicable))
	survivors := make([]*PreLaunchPolicyResult, 0, len(applicable))
	for _, p := range applicable {
		result, survives, evalErr := s.evaluateOnePolicy(ctx, workspaceID, p, at, provenance)
		if evalErr != nil {
			return PreLaunchResult{}, evalErr
		}
		results = append(results, result)
		if survives {
			survivors = append(survivors, &results[len(results)-1])
		}
	}

	sort.Slice(survivors, func(i, j int) bool {
		if !survivors[i].CreatedAt.Equal(survivors[j].CreatedAt) {
			return survivors[i].CreatedAt.Before(survivors[j].CreatedAt)
		}
		return survivors[i].PolicyID < survivors[j].PolicyID
	})

	decision, decidingPolicy := selectPreLaunchDecision(survivors)
	return PreLaunchResult{
		Decision:                         decision,
		DecidingPolicy:                   decidingPolicy,
		Policies:                         results,
		WorkspaceDailyBlockingSuperseded: workspaceDailyBlockingSuperseded(survivors),
	}, nil
}
