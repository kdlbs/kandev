package orchestrator

import (
	"context"
	"fmt"
	"strings"

	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// findFinalWorkflowStep follows the workflow from stepID to its terminal
// position. A nil result means that the workflow has no valid terminal step.
func findFinalWorkflowStep(
	ctx context.Context,
	getter WorkflowStepGetter,
	stepID string,
) (*wfmodels.WorkflowStep, error) {
	if getter == nil || strings.TrimSpace(stepID) == "" {
		return nil, nil
	}
	step, err := getter.GetStep(ctx, stepID)
	if err != nil || step == nil {
		return nil, err
	}
	for range 1000 {
		next, nextErr := getter.GetNextStepByPosition(ctx, step.WorkflowID, step.Position)
		if nextErr != nil {
			return nil, nextErr
		}
		if next == nil {
			if wfmodels.IsTerminalStep(step, nil) {
				return step, nil
			}
			return nil, nil
		}
		step = next
	}
	return nil, fmt.Errorf("workflow final step traversal exceeded limit")
}
