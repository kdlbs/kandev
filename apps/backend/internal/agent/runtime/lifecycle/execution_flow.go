package lifecycle

import (
	"context"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

// Only durable boundaries are logged; content/reasoning chunks and status polls
// remain outside the flow envelope. No payload is decoded or copied.
func (sm *StreamManager) logDeliveryFlow(ctx context.Context, event *models.AgentDeliveryEvent, outcome string) {
	if event == nil {
		return
	}
	switch event.EventType {
	case streams.EventTypeToolCall, streams.EventTypeComplete, streams.EventTypeError, streams.EventTypeAgentPlan, streams.EventTypePlan:
		sm.logger.Flow(ctx, logger.ExecutionFlow{Stage: "delivery", Outcome: outcome, Reason: event.EventType, TaskID: event.TaskID, SessionID: event.SessionID, ExecutionID: event.ExecutionID, SubmissionID: event.SubmissionID, StreamID: event.StreamID, Sequence: event.Sequence, HarnessGeneration: uint64(event.HarnessGeneration)})
	}
}

func (m *Manager) beginLaunchFlow(ctx context.Context, req *LaunchRequest, execution **AgentExecution, result *error) func() {
	flow := logger.ExecutionFlow{Stage: "executor_assignment", ExecutorID: getMetadataString(req.Metadata, "executor_id"), TaskID: req.TaskID, SessionID: req.SessionID, TurnID: req.TurnID, AttemptID: ResumeAttemptIDFromContext(ctx), ProfileID: executionProfileID(req), SubmissionID: req.InitialDeliverySubmissionID, Model: req.ModelOverride, HarnessGeneration: req.DeliveryHarnessGeneration}
	finish := m.logger.BeginFlow(ctx, flow)
	return func() {
		if *result != nil {
			finish("failed", "executor_assignment_failed")
			return
		}
		finish("prepared", "")
		if *execution != nil {
			flow.ExecutionID = (*execution).ID
			flow.Outcome = "bound"
			m.logger.Flow(ctx, flow)
		}
	}
}
