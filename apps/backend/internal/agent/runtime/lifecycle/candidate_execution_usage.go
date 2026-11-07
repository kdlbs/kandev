package lifecycle

import (
	"context"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/usage"
)

// PeekCandidateExecutionUsage supports an already materialized original actor.
// A cold/different candidate or edited profile has no proved execution source
// here and remains unknown; it never borrows another namespace or starts one.
func (m *Manager) PeekCandidateExecutionUsage(ctx context.Context, sessionID string, profile *models.AgentProfile) *usage.ExecutionUsageReport {
	if !m.providerUsageLimits || profile == nil || sessionID == "" {
		return nil
	}
	if m.sessionAccessCheck != nil && m.sessionAccessCheck(ctx, sessionID) != nil {
		return nil
	}
	execution, exists := m.GetExecutionBySessionID(sessionID)
	if !exists {
		return nil
	}
	matches := false
	if m.executionStore.WithRLock(execution.ID, func(current *AgentExecution) {
		matches = current == execution && matchesCandidateUsageProfile(current, profile, sessionID)
	}) != nil || !matches {
		return nil
	}
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	if client == nil {
		return nil
	}
	report, err := client.PeekExecutionUsage(ctx)
	if err != nil || report == nil || report.Model != profile.Model {
		return nil
	}
	return report
}

func matchesCandidateUsageProfile(execution *AgentExecution, profile *models.AgentProfile, sessionID string) bool {
	return execution.SessionID == sessionID && execution.AgentProfileID == profile.ID && !execution.StartedAt.IsZero() && !profile.UpdatedAt.After(execution.StartedAt)
}
