package lifecycle

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// Native restore can emit setup events before its candidate is committed in SQL.
// Only an explicit interrupted-conversation successor may wait for that commit;
// receipt and projection still require the committed generation and live owner.
func awaitSilentRestoreGeneration(
	ctx context.Context, execution *AgentExecution, reader harnessGenerationReader, sessionID, incarnationID string,
) (*models.HarnessSessionGeneration, error) {
	current, err := reader.GetCurrentHarnessSessionGeneration(ctx, sessionID, incarnationID)
	if err != nil || !silentRestoreGenerationPending(execution, current) {
		return current, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for {
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
		current, err = reader.GetCurrentHarnessSessionGeneration(ctx, sessionID, incarnationID)
		if err != nil || !silentRestoreGenerationPending(execution, current) {
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
