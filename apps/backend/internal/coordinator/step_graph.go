package coordinator

import (
	"context"

	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

// LoadStepGraph reads a workflow's steps through reader and maps them to
// []StepNode for EligibleStep (docs/specs/coordinator/system-design/
// proposals.md#no-agent-starts). Returns an empty, non-nil slice for a
// workflow with no steps.
func LoadStepGraph(ctx context.Context, reader WorkflowStepReader, workflowID string) ([]StepNode, error) {
	steps, err := reader.ListStepsByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	nodes := make([]StepNode, len(steps))
	for i, step := range steps {
		nodes[i] = StepNode{
			ID:               step.ID,
			IsStart:          step.IsStartStep,
			AllowManualMove:  step.AllowManualMove,
			AutoStartOnEnter: step.HasOnEnterAction(workflowmodels.OnEnterAutoStartAgent),
			CompletesOnEnter: step.CompleteTaskOnEnter,
			PullFromStepID:   step.PullFromStepID,
		}
	}
	return nodes, nil
}

// StartStepID returns the id of nodes' start step (IsStart == true), or ""
// when none is present (an empty or malformed graph).
func StartStepID(nodes []StepNode) string {
	for _, n := range nodes {
		if n.IsStart {
			return n.ID
		}
	}
	return ""
}
