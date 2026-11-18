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

// ReleaseParkedRuns re-queues the workspace's runs parked by routing so they
// resolve again under the current settings, for example after a step stops
// excluding the author's provider. A run that is still blocked parks again.
func (p *Provider) ReleaseParkedRuns(ctx context.Context, workspaceID string) error {
	if err := p.repo.ClearAllParkedRoutingForWorkspace(ctx, workspaceID); err != nil {
		return fmt.Errorf("routing: release parked runs: %w", err)
	}
	return nil
}
