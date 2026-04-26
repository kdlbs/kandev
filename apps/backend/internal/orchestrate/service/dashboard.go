package service

import (
	"context"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

// GetDashboardData returns aggregated dashboard data for a workspace
// including agent counts by status, task counts, cost summary, pending
// approvals, and recent activity.
func (s *Service) GetDashboardData(ctx context.Context, wsID string) (*models.DashboardData, error) {
	agents, err := s.repo.ListAgentInstances(ctx, wsID)
	if err != nil {
		return nil, err
	}
	agentCounts := countAgentsByStatus(agents)

	pendingApprovals, err := s.repo.CountPendingApprovals(ctx, wsID)
	if err != nil {
		return nil, err
	}

	monthSpend, err := s.GetCostSummary(ctx, wsID)
	if err != nil {
		return nil, err
	}

	activity, err := s.repo.ListActivityEntries(ctx, wsID, 10)
	if err != nil {
		return nil, err
	}

	return &models.DashboardData{
		AgentCount:       len(agents),
		RunningCount:     agentCounts.running,
		PausedCount:      agentCounts.paused,
		ErrorCount:       agentCounts.errors,
		MonthSpendCents:  monthSpend,
		PendingApprovals: pendingApprovals,
		RecentActivity:   activity,
	}, nil
}

type agentStatusCounts struct {
	running int
	paused  int
	errors  int
}

func countAgentsByStatus(agents []*models.AgentInstance) agentStatusCounts {
	var c agentStatusCounts
	for _, a := range agents {
		switch a.Status {
		case models.AgentStatusWorking:
			c.running++
		case models.AgentStatusPaused:
			c.paused++
		case models.AgentStatusStopped:
			// Check pause reason for error state.
			if a.PauseReason != "" {
				c.errors++
			}
		}
	}
	return c
}
