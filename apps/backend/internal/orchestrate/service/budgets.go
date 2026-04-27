package service

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/orchestrate/models"

	"go.uber.org/zap"
)

// Budget scope type constants.
const (
	scopeAgent     = "agent"
	scopeProject   = "project"
	scopeWorkspace = "workspace"
)

// BudgetCheckResult describes the outcome of a budget check.
type BudgetCheckResult struct {
	PolicyID    string
	SpentCents  int
	LimitCents  int
	AlertFired  bool
	LimitExceed bool
	AgentPaused bool
}

// CheckBudget evaluates all budget policies for the given agent and project.
// Returns results for each applicable policy, performing side-effects (alert, pause).
func (s *Service) CheckBudget(
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
	policies []*models.BudgetPolicy,
	agentInstanceID, projectID string,
) []*models.BudgetPolicy {
	var applicable []*models.BudgetPolicy
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

func (s *Service) evaluatePolicy(
	ctx context.Context,
	workspaceID string,
	policy *models.BudgetPolicy,
) (BudgetCheckResult, error) {
	spent, err := s.getSpendForPolicy(ctx, workspaceID, policy)
	if err != nil {
		return BudgetCheckResult{}, err
	}

	result := BudgetCheckResult{
		PolicyID:   policy.ID,
		SpentCents: spent,
		LimitCents: policy.LimitCents,
	}

	threshold := policy.LimitCents * policy.AlertThresholdPct / 100
	if spent >= threshold && spent < policy.LimitCents {
		result.AlertFired = true
		s.logBudgetAlert(ctx, workspaceID, policy, spent)
	}

	if spent >= policy.LimitCents {
		result.LimitExceed = true
		if policy.ActionOnExceed == "pause_agent" && policy.ScopeType == scopeAgent {
			result.AgentPaused = s.pauseAgentForBudget(ctx, policy.ScopeID)
		}
		s.logBudgetExceeded(ctx, workspaceID, policy, spent)
	}

	return result, nil
}

func (s *Service) getSpendForPolicy(
	ctx context.Context,
	workspaceID string,
	policy *models.BudgetPolicy,
) (int, error) {
	var breakdowns []*models.CostBreakdown
	var err error

	switch policy.ScopeType {
	case scopeAgent:
		breakdowns, err = s.repo.GetCostsByAgent(ctx, workspaceID)
	case scopeProject:
		breakdowns, err = s.repo.GetCostsByProject(ctx, workspaceID)
	case scopeWorkspace:
		// Sum all costs for the workspace.
		events, listErr := s.repo.ListCostEvents(ctx, workspaceID)
		if listErr != nil {
			return 0, listErr
		}
		total := 0
		for _, e := range events {
			total += e.CostCents
		}
		return total, nil
	}
	if err != nil {
		return 0, err
	}

	for _, b := range breakdowns {
		if b.GroupKey == policy.ScopeID {
			return b.TotalCents, nil
		}
	}
	return 0, nil
}

func (s *Service) logBudgetAlert(
	ctx context.Context,
	workspaceID string,
	policy *models.BudgetPolicy,
	spentCents int,
) {
	s.LogActivity(ctx, workspaceID, "system", "budget_checker",
		"budget.alert", policy.ScopeType, policy.ScopeID,
		fmt.Sprintf(`{"spent_cents":%d,"limit_cents":%d,"policy_id":"%s"}`,
			spentCents, policy.LimitCents, policy.ID))
}

func (s *Service) logBudgetExceeded(
	ctx context.Context,
	workspaceID string,
	policy *models.BudgetPolicy,
	spentCents int,
) {
	s.LogActivity(ctx, workspaceID, "system", "budget_checker",
		"budget.exceeded", policy.ScopeType, policy.ScopeID,
		fmt.Sprintf(`{"spent_cents":%d,"limit_cents":%d,"action":"%s","policy_id":"%s"}`,
			spentCents, policy.LimitCents, policy.ActionOnExceed, policy.ID))
}

// CheckPreExecutionBudget checks all applicable budget policies before launching
// an agent session. Returns (allowed, reason, error). If allowed is false, the
// caller should skip execution and log the reason.
func (s *Service) CheckPreExecutionBudget(
	ctx context.Context, agentInstanceID, projectID, workspaceID string,
) (bool, string, error) {
	results, err := s.CheckBudget(ctx, workspaceID, agentInstanceID, projectID)
	if err != nil {
		return false, "", fmt.Errorf("budget check: %w", err)
	}
	for _, r := range results {
		if r.LimitExceed && r.AgentPaused {
			reason := fmt.Sprintf(
				"budget exceeded: spent %d cents of %d limit (policy %s)",
				r.SpentCents, r.LimitCents, r.PolicyID)
			return false, reason, nil
		}
	}
	return true, "", nil
}

func (s *Service) pauseAgentForBudget(_ context.Context, agentID string) bool {
	agent, err := s.getAgentFromCacheMutable(agentID)
	if err != nil {
		s.logger.Error("failed to get agent for budget pause",
			zap.String("agent_id", agentID), zap.Error(err))
		return false
	}
	if agent.Status == models.AgentStatusPaused {
		return false // already paused
	}
	agent.Status = models.AgentStatusPaused
	agent.PauseReason = "budget_exceeded"
	s.logger.Info("agent paused due to budget exceeded",
		zap.String("agent_id", agentID))
	return true
}
