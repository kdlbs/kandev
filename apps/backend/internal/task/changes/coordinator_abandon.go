package changes

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// AbandonTerminal settles only the original interval after retry exhaustion.
// Missing or superseded intervals have no work left for this terminal owner.
func (c *Coordinator) AbandonTerminal(ctx context.Context, terminal Terminal) error {
	changeSet, err := c.readTerminalTurnChangeSet(terminal)
	if errors.Is(err, repoerrors.ErrTurnChangeSetNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if changeSet.TerminalAt != nil || !sameTurnChangeTerminalOwner(changeSet, terminal.Admission) {
		return nil
	}
	if changeSet.TerminalCaptureStartedAt != nil && !terminalChangeClaimMatches(changeSet, models.TurnChangeSetTerminalClaim{
		ExecutionID: terminal.ExecutionID, StartupAttemptID: terminal.StartupAttemptID,
		PromptGeneration: int64(terminal.PromptGeneration), TaskEnvironmentID: terminal.TaskEnvironmentID,
	}) {
		return nil
	}
	// Keep accepted endpoint refs and retained file content. Their existing
	// retention owner can clean them after this unavailable result is durable.
	return c.finalizeUnavailable(ctx, changeSet, terminal, models.TurnChangeReasonCaptureFailed)
}
