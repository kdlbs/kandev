package lifecycle

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

type successorGenerationOwner struct {
	ctx context.Context
}

// The launch owner remains active through the SQL commit. The shorter process
// startup context ends when native initialization returns, before that commit.
func (e *AgentExecution) bindSuccessorGenerationOwner(ctx context.Context, req *LaunchRequest) {
	if req.RequiredNativeConversationID == "" && !req.startsSuccessorGeneration() {
		return
	}
	if req.CandidateExecutionID != "" && req.CandidateExecutionID != e.ID {
		return
	}
	previous := e.successorGenerationOwner.Load()
	if previous == nil || previous.ctx.Err() != nil {
		e.successorGenerationOwner.CompareAndSwap(previous, &successorGenerationOwner{ctx: ctx})
	}
}

// Native restore and fresh start can emit setup events before their successor
// generation is committed in SQL. Only an explicit interrupted-conversation
// successor or fresh-start replacement may wait for that commit; receipt and
// projection still require the committed generation and live owner.
func awaitSilentRestoreGeneration(
	ctx context.Context, execution *AgentExecution, reader harnessGenerationReader, sessionID, incarnationID string,
) (*models.HarnessSessionGeneration, error) {
	current, err := reader.GetCurrentHarnessSessionGeneration(ctx, sessionID, incarnationID)
	if err != nil || !successorGenerationPending(execution, current) {
		return current, err
	}
	streamCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if owner := execution.successorGenerationOwner.Load(); owner != nil {
		stop := context.AfterFunc(owner.ctx, cancel)
		defer stop()
	}
	for {
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			// The owner can finish immediately after committing. Read once more
			// under stream ownership before treating its cancellation as failure.
			readCtx, cancelRead := context.WithTimeout(streamCtx, 5*time.Second)
			current, err = reader.GetCurrentHarnessSessionGeneration(readCtx, sessionID, incarnationID)
			cancelRead()
			if err == nil && !successorGenerationPending(execution, current) {
				return current, nil
			}
			return nil, ctx.Err()
		case <-timer.C:
		}
		current, err = reader.GetCurrentHarnessSessionGeneration(ctx, sessionID, incarnationID)
		if ctx.Err() != nil {
			continue
		}
		if err != nil || !successorGenerationPending(execution, current) {
			return current, err
		}
	}

}

func silentRestoreGenerationPending(execution *AgentExecution, current *models.HarnessSessionGeneration) bool {
	return execution != nil && current != nil && execution.RequiredNativeConversationID != "" &&
		execution.InterruptedSubmissionID != "" && execution.InterruptedStreamID != "" &&
		execution.InterruptedHarnessGeneration > 0 && execution.InterruptedHarnessGeneration+1 == execution.DeliveryHarnessGeneration &&
		current.SessionID == execution.SessionID && current.IncarnationID == execution.DeliveryIncarnationID &&
		uint64(current.Generation) == execution.InterruptedHarnessGeneration && current.NativeSessionID == execution.RequiredNativeConversationID
}

func successorGenerationPending(execution *AgentExecution, current *models.HarnessSessionGeneration) bool {
	return silentRestoreGenerationPending(execution, current) || replacementGenerationPending(execution, current)
}

func replacementGenerationPending(execution *AgentExecution, current *models.HarnessSessionGeneration) bool {
	return execution != nil && current != nil && execution.ReplacesNativeConversation &&
		current.SessionID == execution.SessionID && current.IncarnationID == execution.DeliveryIncarnationID &&
		uint64(current.Generation)+1 == execution.DeliveryHarnessGeneration
}

// startsSuccessorGeneration reports whether the launch starts a new native
// conversation on the harness generation after the committed one.
func (r *LaunchRequest) startsSuccessorGeneration() bool {
	return r.ForceContextContinuation || r.ReplaceNativeConversation
}
