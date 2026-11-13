package orchestrator

import "context"

// Automatic history recovery keeps the failed conversation primary unless
// another session already owns active execution. The failed session's own
// runtime can still be present while its bootstrap cleanup completes.
func (s *Service) preservePrimaryForAutomaticHistory(ctx context.Context, taskID, sessionID string) bool {
	if !s.config.AutoContinueFromHistory {
		return false
	}
	failed, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || failed == nil || failed.TaskID != taskID || nativeBootstrapRecoveryReason(failed) == "" {
		return false
	}
	otherActive, err := s.automaticTaskHasOtherExecution(ctx, taskID, sessionID)
	// An unreadable inventory does not prove another session owns execution.
	return err != nil || !otherActive
}
