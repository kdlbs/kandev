package orchestrator

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/task/models"
)

// dynamicRelaunchHasResumeSource reports whether a manual relaunch can
// prompt the session through a cold resume. Without an executors_running row
// (an orphaned starting route swept after a restart) ensureSessionRunning has
// nothing to resume, so the relaunch must start a fresh execution instead.
// A lookup error other than not-found keeps the resume path, whose own
// failure stays visible.
func (s *Service) dynamicRelaunchHasResumeSource(ctx context.Context, sessionID string) bool {
	_, err := s.repo.GetExecutorRunningBySessionID(ctx, sessionID)
	return !errors.Is(err, models.ErrExecutorRunningNotFound)
}
