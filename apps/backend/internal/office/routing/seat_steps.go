package routing

import "context"

// SeatStep is one workflow step's ensure_participant_seat action for a role,
// with whether it keeps the seat's runs off the author's provider
// (REQ-OFFICE-REVIEW-AUTHOR-EXCL-006).
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
