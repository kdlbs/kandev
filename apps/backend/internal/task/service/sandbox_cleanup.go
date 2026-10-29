package service

import (
	"context"
	"fmt"
	"github.com/kandev/kandev/internal/task/models"
)

// SetSessionSandboxCleanup binds owned child-state removal for durable deletion.
func (s *Service) SetSessionSandboxCleanup(cleanup func(context.Context, string, string) error) {
	s.sessionSandboxCleanup = cleanup
}

func (s *Service) cleanupTaskSandboxSessions(ctx context.Context, job *models.TaskResourceCleanupJob, sessionIDs []string) error {
	if s.sessionSandboxCleanup == nil || !taskResourceCleanupDeletesTask(job.Trigger) {
		return nil
	}
	for _, sessionID := range sessionIDs {
		if err := s.sessionSandboxCleanup(ctx, job.TaskID, sessionID); err != nil {
			return fmt.Errorf("delete sandbox session state: %w", err)
		}
	}
	return nil
}
