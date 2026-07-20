package gitlab

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/watchreset"
)

type reviewWatchResetter struct {
	store   *Store
	watchID string
}

func (r reviewWatchResetter) ListTaskIDs(ctx context.Context) ([]string, error) {
	return r.store.ListReviewMRTaskIDsByWatch(ctx, r.watchID)
}

func (r reviewWatchResetter) Clear(ctx context.Context) error {
	return r.store.ResetReviewWatchState(ctx, r.watchID)
}

type issueWatchResetter struct {
	store   *Store
	watchID string
}

func (r issueWatchResetter) ListTaskIDs(ctx context.Context) ([]string, error) {
	return r.store.ListIssueWatchTaskIDsByWatch(ctx, r.watchID)
}

func (r issueWatchResetter) Clear(ctx context.Context) error {
	return r.store.ResetIssueWatchState(ctx, r.watchID)
}

func (s *Service) PreviewResetReviewWatch(ctx context.Context, watchID string) (int, error) {
	store := s.requireStore()
	if store == nil {
		return 0, errStoreUnavailable
	}
	return watchreset.Preview(ctx, reviewWatchResetter{store: store, watchID: watchID})
}

func (s *Service) DisableReviewWatchWithError(ctx context.Context, watchID, cause string) error {
	store := s.requireStore()
	if store == nil {
		return errStoreUnavailable
	}
	return store.DisableReviewWatchWithError(ctx, watchID, cause)
}

func (s *Service) ResetReviewWatch(ctx context.Context, watchID string) (int, error) {
	store := s.requireStore()
	if store == nil {
		return 0, errStoreUnavailable
	}
	s.mu.RLock()
	deleter := s.cascadeTaskDeleter
	s.mu.RUnlock()
	if deleter == nil {
		return 0, errors.New("gitlab: cascade task deleter not wired; reset unavailable")
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	ctx = context.WithoutCancel(ctx)
	invalidation, err := store.BeginReviewWatchReset(ctx, watchID)
	if err != nil {
		return 0, err
	}
	deleted := s.deleteInvalidatedWatchTasks(ctx, watchID, invalidation.TaskIDs, deleter)
	err = store.FinishReviewWatchReset(ctx, watchID, invalidation.Generation)
	if err == nil {
		go s.reimportReviewWatchAfterReset(context.Background(), watchID)
	}
	return deleted, err
}

func (s *Service) reimportReviewWatchAfterReset(ctx context.Context, watchID string) {
	watch, err := s.GetReviewWatch(ctx, watchID)
	if err != nil || watch == nil || !watch.Enabled {
		if err != nil {
			s.logger.Warn("reset review watch: load for re-import", zap.String("watch_id", watchID), zap.Error(err))
		}
		return
	}
	if _, err := s.TriggerReviewWatch(ctx, watchID); err != nil {
		s.logger.Warn("reset review watch: re-import failed", zap.String("watch_id", watchID), zap.Error(err))
	}
}

func (s *Service) PreviewResetIssueWatch(ctx context.Context, watchID string) (int, error) {
	store := s.requireStore()
	if store == nil {
		return 0, errStoreUnavailable
	}
	return watchreset.Preview(ctx, issueWatchResetter{store: store, watchID: watchID})
}

func (s *Service) DisableIssueWatchWithError(ctx context.Context, watchID, cause string) error {
	store := s.requireStore()
	if store == nil {
		return errStoreUnavailable
	}
	return store.DisableIssueWatchWithError(ctx, watchID, cause)
}

func (s *Service) ResetIssueWatch(ctx context.Context, watchID string) (int, error) {
	store := s.requireStore()
	if store == nil {
		return 0, errStoreUnavailable
	}
	s.mu.RLock()
	deleter := s.cascadeTaskDeleter
	s.mu.RUnlock()
	if deleter == nil {
		return 0, errors.New("gitlab: cascade task deleter not wired; reset unavailable")
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	ctx = context.WithoutCancel(ctx)
	invalidation, err := store.BeginIssueWatchReset(ctx, watchID)
	if err != nil {
		return 0, err
	}
	deleted := s.deleteInvalidatedWatchTasks(ctx, watchID, invalidation.TaskIDs, deleter)
	err = store.FinishIssueWatchReset(ctx, watchID, invalidation.Generation)
	return deleted, err
}

func (s *Service) deleteInvalidatedWatchTasks(ctx context.Context, watchID string, ids []string, deleter watchreset.TaskDeleter) int {
	deleted := 0
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, err := deleter.DeleteTaskTree(ctx, id, true); err != nil {
			s.logger.Warn("watch reset: task tree cleanup failed",
				zap.String("watch_id", watchID), zap.String("task_id", id), zap.Error(err))
			continue
		}
		deleted++
	}
	return deleted
}

func (s *Service) deleteWatchTaskTrees(ctx context.Context, watchID string, ids []string, deleter watchreset.TaskDeleter) {
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, err := deleter.DeleteTaskTree(ctx, id, true); err != nil {
			s.logger.Warn("delete watch: task tree cleanup failed",
				zap.String("watch_id", watchID), zap.String("task_id", id), zap.Error(err))
		}
	}
}
