package backendapp

import (
	"context"

	"github.com/kandev/kandev/internal/office/dashboard"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

type officePendingApproverReader interface {
	PendingApproversForCompletion(ctx context.Context, taskID, stepID string) ([]dashboard.PendingApproverDTO, error)
}

// officeApprovalGuardAdapter lets the task service consult Office approvers
// before a human move completes an Office task.
type officeApprovalGuardAdapter struct {
	reader officePendingApproverReader
}

func (a officeApprovalGuardAdapter) PendingApproversForCompletion(
	ctx context.Context, taskID, stepID string,
) ([]taskservice.PendingApprover, error) {
	pending, err := a.reader.PendingApproversForCompletion(ctx, taskID, stepID)
	if err != nil {
		return nil, err
	}
	out := make([]taskservice.PendingApprover, 0, len(pending))
	for _, p := range pending {
		out = append(out, taskservice.PendingApprover{AgentProfileID: p.AgentProfileID, Name: p.Name})
	}
	return out, nil
}
