package service

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/archivecascade"
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
	deadline := archivecascade.ArchiveDeadline(ctx)
	runCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	tasks, err := s.tasks.ListTasksForAutoArchive(runCtx)
	if err != nil {
		s.logger.Error("auto-archive: failed to list candidates", zap.Error(err))
		return
	}
	if len(tasks) == 0 {
		return
	}

	s.logger.Info("auto-archive: found candidates", zap.Int("count", len(tasks)))
	if s.autoArchiveCoordinator == nil {
		s.logger.Error("auto-archive: lifecycle coordinator is not configured")
		return
	}
	for _, task := range tasks {
		out, err := s.autoArchiveCoordinator.ArchiveAutoTask(runCtx, task)
		switch {
		case err != nil:
			s.logger.Warn("auto-archive: failed to archive task",
				zap.String("task_id", task.ID),
				zap.Error(err))
		case out != nil && len(out.ArchivedTaskIDs) == 0 && len(out.SkippedTaskIDs) > 0:
			s.logger.Info("auto-archive: candidate changed before archive",
				zap.String("task_id", task.ID))
		default:
			s.logger.Info("auto-archive: archived task",
				zap.String("task_id", task.ID),
				zap.String("title", task.Title))
		}
	}
}
