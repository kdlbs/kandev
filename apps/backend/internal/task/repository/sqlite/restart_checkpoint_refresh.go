package sqlite

import "github.com/kandev/kandev/internal/task/models"

// Notice updates may refresh an unclaimed attempt. Allocating a candidate
// pins the source so concurrent delivery notices cannot change its authority.
func validPreparedRestoreNoticeRefresh(expected, next models.SilentRestoreCheckpoint) bool {
	if expected.Stage != models.SilentRestoreStagePrepared || next.Stage != models.SilentRestoreStagePrepared ||
		expected.CandidateExecutionID != "" || next.CandidateExecutionID != "" ||
		next.SourceRecovery.Revision < expected.SourceRecovery.Revision ||
		next.SourceBlock.UpdatedAt.Before(expected.SourceBlock.UpdatedAt) ||
		!pendingRestoreNoticePhase(expected.SourceRecovery.Phase) || !pendingRestoreNoticePhase(next.SourceRecovery.Phase) {
		return false
	}
	previous := expected
	if next.SourceRecovery.Revision > expected.SourceRecovery.Revision {
		previous.SourceRecovery.Phase = next.SourceRecovery.Phase
		previous.SourceRecovery.Revision = next.SourceRecovery.Revision
		previous.SourceRecovery.Message = next.SourceRecovery.Message
		previous.SourceRecovery.UpdatedAt = next.SourceRecovery.UpdatedAt
	}
	previous.SourceBlock.UpdatedAt = next.SourceBlock.UpdatedAt
	return sameSilentRestoreCheckpointSource(previous, next)
}

func pendingRestoreNoticePhase(phase string) bool {
	return phase == models.AgentDeliveryRecoveryUncertain || phase == models.AgentDeliveryRecoveryReconnecting
}
