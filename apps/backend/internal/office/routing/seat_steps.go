package routing

import (
	"context"
	"fmt"
)

// SeatStep is one workflow step's ensure_participant_seat action for a role,
// with whether it keeps the seat's runs off the author's provider.
type SeatStep struct {
	WorkflowID            string
	WorkflowName          string
	StepID                string
	StepName              string
	Role                  string
	ExcludeAuthorProvider bool
}

// SeatSteps lists the workspace's seat steps, hidden workflows included.
func (p *Provider) SeatSteps(ctx context.Context, workspaceID string) ([]SeatStep, error) {
	return p.repo.ListSeatSteps(ctx, workspaceID)
}

// ReleaseAuthorExclusionParkedRuns re-queues runs at stepIDs that parked after
// skipping the author's provider, so they resolve again once those steps stop
// excluding it. Runs held for any other reason, such as a session-ceiling
// deferral whose replay owns the launch, stay parked.
func (p *Provider) ReleaseAuthorExclusionParkedRuns(ctx context.Context, workspaceID string, stepIDs []string) error {
	if len(stepIDs) == 0 {
		return nil
	}
	if _, err := p.repo.ReleaseAuthorExclusionParkedRuns(ctx, workspaceID, stepIDs); err != nil {
		return fmt.Errorf("routing: release author-exclusion parked runs: %w", err)
	}
	return nil
}
