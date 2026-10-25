package backendapp

import (
	"context"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func (b bootStateBuilder) taskWorkflowCoverage(ctx context.Context, workspaceID string) *taskmodels.TaskWorkflowCoverage {
	coverage, err := b.p.taskSvc.TaskWorkflowCoverage(ctx, workspaceID)
	if err != nil {
		b.logBootError("read workflow task coverage", err)
		return nil
	}
	return coverage
}
