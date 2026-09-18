package orchestrator

import "context"

// StopTaskSessionForCoordinator reuses native cancellation/queue guards and
// checks session.control before acting on the exact task/session pair.
func (s *Service) StopTaskSessionForCoordinator(ctx context.Context, taskID, sessionID string) (bool, error) {
	if err := s.authorizeSessionControl(ctx, sessionID); err != nil {
		return false, err
	}
	return s.stopTaskSessionForCoordinator(ctx, taskID, sessionID)
}
