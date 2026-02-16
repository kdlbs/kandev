package service

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// StartAutoArchiveLoop starts a background goroutine that periodically archives tasks
// in workflow steps with auto_archive_after_hours > 0.
func (s *Service) StartAutoArchiveLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runAutoArchive(ctx)
			}
		}
	}()
	s.logger.Info("auto-archive loop started (every 5 minutes)")
}

func (s *Service) runAutoArchive(ctx context.Context) {
	tasks, err := s.repo.ListTasksForAutoArchive(ctx)
	if err != nil {
		s.logger.Error("auto-archive: failed to list candidates", zap.Error(err))
		return
	}
	if len(tasks) == 0 {
		return
	}

	s.logger.Info("auto-archive: found candidates", zap.Int("count", len(tasks)))
	for _, task := range tasks {
		if err := s.ArchiveTask(ctx, task.ID); err != nil {
			s.logger.Warn("auto-archive: failed to archive task",
				zap.String("task_id", task.ID),
				zap.Error(err))
		} else {
			s.logger.Info("auto-archive: archived task",
				zap.String("task_id", task.ID),
				zap.String("title", task.Title))
		}
	}
}
