package costs

import (
	"context"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/office/models"

	"go.uber.org/zap"
)

// Budget scope type constants.
const (
	scopeAgent     = "agent"
	scopeProject   = "project"
	scopeWorkspace = "workspace"
)

// Budget period and action-on-exceed constants. See spec
// docs/specs/office-costs/spec.md for semantics.
const (
	budgetPeriodMonthly = "monthly"

	budgetActionPauseAgent    = "pause_agent"
	budgetActionBlockNewTasks = "block_new_tasks"
)

// BudgetCheckResult describes the outcome of a budget check.
// SpentSubcents / LimitSubcents are hundredths of a cent.
type BudgetCheckResult struct {
	PolicyID       string
	ActionOnExceed string
	SpentSubcents  int64
	LimitSubcents  int64
	AlertFired     bool
	LimitExceed    bool
	AgentPaused    bool
}

// CheckBudget evaluates all budget policies for the given agent and project.
// Returns results for each applicable policy, performing side-effects (alert, pause).
func (s *CostService) CheckBudget(
	ctx context.Context,
	workspaceID, agentInstanceID, projectID string,
) ([]BudgetCheckResult, error) {
	policies, err := s.repo.ListBudgetPolicies(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	applicable := filterApplicablePolicies(policies, agentInstanceID, projectID)
	if len(applicable) == 0 {
		return nil, nil
	}

	var results []BudgetCheckResult
	for _, policy := range applicable {
		result, err := s.evaluatePolicy(ctx, workspaceID, policy)
		if err != nil {
			s.logger.Error("budget check failed",
				zap.String("policy_id", policy.ID),
				zap.Error(err))
			continue
		}
		results = append(results, result)
	}
	return results, nil
}

func filterApplicablePolicies(
	policies []*BudgetPolicy,
	agentInstanceID, projectID string,
) []*BudgetPolicy {
	var applicable []*BudgetPolicy
	for _, p := range policies {
		switch p.ScopeType {
		case scopeAgent:
			if p.ScopeID == agentInstanceID {
				applicable = append(applicable, p)
			}
		case scopeProject:
			if p.ScopeID == projectID {
				applicable = append(applicable, p)
			}
		case scopeWorkspace:
			applicable = append(applicable, p)
		}
	}
	return applicable
}

func (s *CostService) evaluatePolicy(
	ctx context.Context,
	workspaceID string,
	policy *BudgetPolicy,
) (BudgetCheckResult, error) {
	spent, err := s.getSpendForPolicy(ctx, workspaceID, policy)
	if err != nil {
		return BudgetCheckResult{}, err
	}

	limit := policy.LimitSubcents
	result := BudgetCheckResult{
		PolicyID:       policy.ID,
		ActionOnExceed: string(policy.ActionOnExceed),
		SpentSubcents:  spent,
		LimitSubcents:  limit,
	}

	threshold := limit * int64(policy.AlertThresholdPct) / 100
	if spent >= threshold && spent < limit {
		result.AlertFired = true
		s.logBudgetAlert(ctx, workspaceID, policy, spent)
	}

	if spent >= limit {
		result.LimitExceed = true
		if policy.ActionOnExceed == budgetActionPauseAgent && policy.ScopeType == scopeAgent {
			result.AgentPaused = s.pauseAgentForBudget(ctx, policy.ScopeID)
		}
		s.logBudgetExceeded(ctx, workspaceID, policy, spent)
	}

	return result, nil
}

// periodCutoff returns the time.Time at which the policy's spend window
// starts. A zero time means "no filter" (lifetime / total).
func periodCutoff(period string, now time.Time) time.Time {
	if period == budgetPeriodMonthly {
		n := now.UTC()
		return time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
	return time.Time{}
}

func (s *CostService) getSpendForPolicy(
	ctx context.Context,
	workspaceID string,
	policy *BudgetPolicy,
) (int64, error) {
	since := periodCutoff(string(policy.Period), time.Now())
	switch policy.ScopeType {
	case scopeAgent:
		return s.repo.GetCostForAgentSince(ctx, policy.ScopeID, since)
	case scopeProject:
		return s.repo.GetCostForProjectSince(ctx, policy.ScopeID, since)
	case scopeWorkspace:
		return s.repo.SumCostsSince(ctx, workspaceID, since)
	default:
		return 0, nil
	}
}

func (s *CostService) logBudgetAlert(
	ctx context.Context,
	workspaceID string,
	policy *BudgetPolicy,
	spentSubcents int64,
) {
	s.activity.LogActivity(ctx, workspaceID, "system", "budget_checker",
		"budget.alert", string(policy.ScopeType), policy.ScopeID,
		fmt.Sprintf(`{"spent_subcents":%d,"limit_subcents":%d,"period":%q,"policy_id":%q}`,
			spentSubcents, policy.LimitSubcents, policy.Period, policy.ID))
}

func (s *CostService) logBudgetExceeded(
	ctx context.Context,
	workspaceID string,
	policy *BudgetPolicy,
	spentSubcents int64,
) {
	s.activity.LogActivity(ctx, workspaceID, "system", "budget_checker",
		"budget.exceeded", string(policy.ScopeType), policy.ScopeID,
		fmt.Sprintf(`{"spent_subcents":%d,"limit_subcents":%d,"period":%q,"action":%q,"policy_id":%q}`,
			spentSubcents, policy.LimitSubcents, policy.Period, policy.ActionOnExceed, policy.ID))
}

// EvaluateBudget runs the post-event budget check: any applicable
// policy that is over its limit fires an alert; pause_agent policies
// flip the agent to paused. Per-policy results are discarded — the
// office service's subscriber only cares about side effects.
func (s *CostService) EvaluateBudget(
	ctx context.Context, workspaceID, agentInstanceID, projectID string,
) error {
	_, err := s.CheckBudget(ctx, workspaceID, agentInstanceID, projectID)
	return err
}

// CheckPreExecutionBudget checks all applicable budget policies before
// launching an agent session. Returns (allowed, reason, error). Only
// pause_agent and block_new_tasks return allowed=false (notify_only
// alerts but does not block). See docs/specs/office-costs/spec.md.
// Implements shared.BudgetChecker.
func (s *CostService) CheckPreExecutionBudget(
	ctx context.Context, agentInstanceID, projectID, workspaceID string,
) (bool, string, error) {
	results, err := s.CheckBudget(ctx, workspaceID, agentInstanceID, projectID)
	if err != nil {
		return false, "", fmt.Errorf("budget check: %w", err)
	}
	for _, r := range results {
		if !r.LimitExceed {
			continue
		}
		if r.ActionOnExceed == budgetActionPauseAgent || r.ActionOnExceed == budgetActionBlockNewTasks {
			reason := fmt.Sprintf(
				"budget exceeded: spent %d of %d subcents (policy %s, action %s)",
				r.SpentSubcents, r.LimitSubcents, r.PolicyID, r.ActionOnExceed)
			return false, reason, nil
		}
	}
	return true, "", nil
}

func (s *CostService) pauseAgentForBudget(ctx context.Context, agentID string) bool {
	agent, err := s.agents.GetAgentInstance(ctx, agentID)
	if err != nil {
		s.logger.Error("failed to get agent for budget pause",
			zap.String("agent_id", agentID), zap.Error(err))
		return false
	}
	if agent.Status == models.AgentStatusPaused {
		return false
	}
	if updErr := s.agentW.UpdateAgentStatusFields(
		ctx, agent.ID, string(models.AgentStatusPaused), "budget_exceeded",
	); updErr != nil {
		s.logger.Error("failed to persist agent pause for budget",
			zap.String("agent_id", agentID), zap.Error(updErr))
		return false
	}
	s.logger.Info("agent paused due to budget exceeded",
		zap.String("agent_id", agentID))
	return true
}

// CreateBudgetPolicy creates a new budget policy.
func (s *CostService) CreateBudgetPolicy(ctx context.Context, policy *BudgetPolicy) error {
	return s.repo.CreateBudgetPolicy(ctx, policy)
}

// ListBudgetPolicies returns all budget policies for a workspace.
func (s *CostService) ListBudgetPolicies(ctx context.Context, wsID string) ([]*BudgetPolicy, error) {
	return s.repo.ListBudgetPolicies(ctx, wsID)
}

// GetBudgetPolicy returns a budget policy by ID.
func (s *CostService) GetBudgetPolicy(ctx context.Context, id string) (*BudgetPolicy, error) {
	return s.repo.GetBudgetPolicy(ctx, id)
}

// UpdateBudgetPolicy updates a budget policy.
func (s *CostService) UpdateBudgetPolicy(ctx context.Context, policy *BudgetPolicy) error {
	return s.repo.UpdateBudgetPolicy(ctx, policy)
}

// DeleteBudgetPolicy deletes a budget policy.
func (s *CostService) DeleteBudgetPolicy(ctx context.Context, id string) error {
	return s.repo.DeleteBudgetPolicy(ctx, id)
}
