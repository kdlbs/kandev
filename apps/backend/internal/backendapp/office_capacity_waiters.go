package backendapp

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/orchestrator"
	runmodels "github.com/kandev/kandev/internal/runs/models"
)

func (l *officeRunSessionLauncher) officeCapacityWaiters(ctx context.Context) ([]orchestrator.OfficeCapacityWaiter, error) {
	if l.repo == nil || l.resolver == nil {
		return nil, errors.New("office queue or logical grade reader unavailable")
	}
	queued, err := l.repo.ListTasklessCapacityCandidates(ctx)
	if err != nil {
		return nil, err
	}
	claimed, err := l.repo.ListUnstartedClaimedTasklessRuns(ctx)
	if err != nil {
		return nil, err
	}
	// Already-claimed owners retain their original grant through registration.
	// Remaining rows keep Office's existing class/age/requested_at order.
	rows := claimed
	rows = append(rows, queued...)
	result := make([]orchestrator.OfficeCapacityWaiter, 0, len(rows))
	for _, row := range rows {
		eligible, err := l.officeQueueEligible(ctx, row)
		if err != nil {
			return nil, err
		}
		if !eligible {
			continue
		}
		grade, gradeErr := l.resolver.LogicalExecutionGrade(ctx, row.AgentProfileID)
		result = append(result, orchestrator.OfficeCapacityWaiter{RunID: row.ID, WorkspaceID: row.WorkspaceID, ProfileID: row.AgentProfileID, QueuedAt: row.RequestedAt, Grade: grade, GradeKnown: gradeErr == nil, Claimed: row.Status == runmodels.RunStatusClaimed})
	}
	return result, nil
}

// Use the actual agent workspace (including legacy queue rows whose stored
// workspace is empty), and the same persisted pause predicate as Office
// dispatch. Read failures stay unknown; no pause/activity row is changed.
func (l *officeRunSessionLauncher) officeQueueEligible(ctx context.Context, row runmodels.Run) (bool, error) {
	agent, err := l.repo.GetAgentInstance(ctx, row.AgentProfileID)
	if err != nil {
		return false, err
	}
	pause, err := l.repo.GetActiveWorkspacePause(ctx, agent.WorkspaceID)
	return pause == nil, err
}
