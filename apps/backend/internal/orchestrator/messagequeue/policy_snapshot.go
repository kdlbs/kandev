package messagequeue

import "context"

// AutoRunSnapshot reads policy without taking a queue admission lock. It grants
// nothing; the existing exact-session admission still validates before dispatch.
func (s *Service) AutoRunSnapshot(ctx context.Context, sessionID string) (bool, error) {
	return s.repo.GetAutoRun(ctx, sessionID)
}
