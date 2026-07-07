package service

import (
	"context"
	"time"

	"go.uber.org/zap"
)

const (
	quickChatIdleRetention      = 7 * 24 * time.Hour
	quickChatExpirationInterval = 24 * time.Hour
)

// StartQuickChatExpirationLoop starts a background goroutine that removes
// abandoned quick chats after the retention window.
func (s *Service) StartQuickChatExpirationLoop(ctx context.Context) {
	ticker := time.NewTicker(quickChatExpirationInterval)
	go func() {
		defer ticker.Stop()
		s.runQuickChatExpiration(ctx, time.Now())
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				s.runQuickChatExpiration(ctx, now)
			}
		}
	}()
	s.logger.Info("quick-chat expiration loop started",
		zap.Duration("interval", quickChatExpirationInterval),
		zap.Duration("retention", quickChatIdleRetention))
}

func (s *Service) runQuickChatExpiration(ctx context.Context, now time.Time) {
	cutoff := now.UTC().Add(-quickChatIdleRetention)
	tasks, err := s.tasks.ListExpiredQuickChatTasks(ctx, cutoff)
	if err != nil {
		s.logger.Warn("quick-chat expiration: failed to list candidates", zap.Error(err))
		return
	}
	if len(tasks) == 0 {
		return
	}

	s.logger.Info("quick-chat expiration: found candidates", zap.Int("count", len(tasks)))
	candidateIDs := s.expiredQuickChatCandidateIDs(ctx, cutoff)
	for _, task := range tasks {
		if !candidateIDs[task.ID] {
			continue
		}
		if err := s.DeleteTask(ctx, task.ID); err != nil {
			s.logger.Warn("quick-chat expiration: failed to delete task",
				zap.String("task_id", task.ID),
				zap.Error(err))
			continue
		}
		s.logger.Info("quick-chat expiration: deleted task",
			zap.String("task_id", task.ID))
	}
}

func (s *Service) expiredQuickChatCandidateIDs(ctx context.Context, cutoff time.Time) map[string]bool {
	tasks, err := s.tasks.ListExpiredQuickChatTasks(ctx, cutoff)
	if err != nil {
		s.logger.Warn("quick-chat expiration: failed to recheck candidates", zap.Error(err))
		return map[string]bool{}
	}
	candidates := make(map[string]bool, len(tasks))
	for _, task := range tasks {
		if task != nil {
			candidates[task.ID] = true
		}
	}
	return candidates
}
