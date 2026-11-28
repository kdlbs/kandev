package backendapp

import (
	"context"
	"errors"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/system/sessioncapacity"
)

func (b *sharedInferenceConsumer) logInferenceFlow(ctx context.Context, binding sessioncapacity.InferenceBinding, stage, outcome, reason string, duration int64) {
	logger.EmitExecutionFlow(b.flowLogger, ctx, logger.ExecutionFlow{Stage: stage, Outcome: outcome, Reason: reason, DurationMS: duration, TaskID: binding.TaskID, SessionID: binding.SessionID, ExecutionID: binding.InstanceID, ParentExecutionID: binding.ParentExecutionID, AttemptID: binding.AttemptID, OwnerID: binding.OwnerID, ReservationID: binding.Key, ProfileID: binding.ExecutionProfileID, Model: binding.Model, Grade: binding.Grade})
}

func (b *sharedInferenceConsumer) finishInferenceFlow(ctx context.Context, binding sessioncapacity.InferenceBinding, started time.Time, response *utility.PromptResponse, err error) {
	outcome, reason := "completed", ""
	if err != nil || response == nil || !response.Success {
		outcome, reason = "failed", "inference_request_failed"
	}
	var capacityErr *agentruntime.InferenceCapacityError
	if errors.As(err, &capacityErr) {
		reason = capacityErr.Reason
	}
	b.logInferenceFlow(ctx, binding, "inference_request", outcome, reason, time.Since(started).Milliseconds())
}

func (b *sharedInferenceConsumer) logInferenceResult(ctx context.Context, binding sessioncapacity.InferenceBinding, started time.Time, response *utility.PromptResponse, err error) {
	outcome := "returned"
	if err != nil || response == nil || !response.Success {
		outcome = "failed"
	}
	b.logInferenceFlow(ctx, binding, "inference", outcome, "", time.Since(started).Milliseconds())
}
