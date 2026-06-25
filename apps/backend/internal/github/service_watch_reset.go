package github

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/watchreset"
)

// --- Issue watch reset ---

// githubIssueWatchResetter is the watchreset.Resetter adapter for a single
// GitHub issue watch. Closes over the store and watch ID so the shared
// watchreset.Run helper stays integration-agnostic.
type githubIssueWatchResetter struct {
	store   *Store
	watchID string
}

func (r *githubIssueWatchResetter) ListTaskIDs(ctx context.Context) ([]string, error) {
	return r.store.ListIssueWatchTaskIDsByWatch(ctx, r.watchID)
}

func (r *githubIssueWatchResetter) Clear(ctx context.Context) error {
	return r.store.ResetIssueWatchState(ctx, r.watchID)
}

// PreviewResetIssueWatch returns how many tasks ResetIssueWatch would
// cascade-delete. Used by the frontend to populate the confirmation dialog.
func (s *Service) PreviewResetIssueWatch(ctx context.Context, watchID string) (int, error) {
	if s.store == nil {
		return 0, errStoreUnavailable
	}
	return watchreset.Preview(ctx, &githubIssueWatchResetter{store: s.store, watchID: watchID})
}

// ResetIssueWatch is destructive: cascade-deletes every task previously
// created by the issue watch (including archived), wipes the per-watch
// dedup rows, and nulls last_polled_at so the next poll re-imports every
// currently-matching issue. Returns the count of tasks deleted.
func (s *Service) ResetIssueWatch(ctx context.Context, watchID string) (int, error) {
	if s.store == nil {
		return 0, errStoreUnavailable
	}
	s.mu.Lock()
	td := s.cascadeTaskDeleter
	s.mu.Unlock()
	if td == nil {
		return 0, errors.New("github: cascade task deleter not wired; reset unavailable")
	}
	res, err := watchreset.Run(ctx,
		&githubIssueWatchResetter{store: s.store, watchID: watchID},
		td, s.logger)
	return res.TasksDeleted, err
}

// --- Review watch reset ---

// githubReviewWatchResetter is the watchreset.Resetter adapter for a single
// GitHub review watch.
type githubReviewWatchResetter struct {
	store   *Store
	watchID string
}

func (r *githubReviewWatchResetter) ListTaskIDs(ctx context.Context) ([]string, error) {
	return r.store.ListReviewPRTaskIDsByWatch(ctx, r.watchID)
}

func (r *githubReviewWatchResetter) Clear(ctx context.Context) error {
	return r.store.ResetReviewWatchState(ctx, r.watchID)
}

// PreviewResetReviewWatch returns how many tasks ResetReviewWatch would
// cascade-delete. Used by the frontend to populate the confirmation dialog.
func (s *Service) PreviewResetReviewWatch(ctx context.Context, watchID string) (int, error) {
	if s.store == nil {
		return 0, errStoreUnavailable
	}
	return watchreset.Preview(ctx, &githubReviewWatchResetter{store: s.store, watchID: watchID})
}

// ResetReviewWatch is destructive: cascade-deletes every task previously
// created by the review watch (including archived), wipes the per-watch
// dedup rows, and then schedules the watch to re-run so currently-matching
// PRs are published for task creation. Returns the count of tasks deleted.
func (s *Service) ResetReviewWatch(ctx context.Context, watchID string) (int, error) {
	if s.store == nil {
		return 0, errStoreUnavailable
	}
	s.mu.Lock()
	td := s.cascadeTaskDeleter
	s.mu.Unlock()
	if td == nil {
		return 0, errors.New("github: cascade task deleter not wired; reset unavailable")
	}
	res, err := watchreset.Run(ctx,
		&githubReviewWatchResetter{store: s.store, watchID: watchID},
		td, s.logger)
	if err != nil {
		return res.TasksDeleted, err
	}
	runCtx := context.Background()
	watch, err := s.GetReviewWatch(runCtx, watchID)
	if err != nil {
		return res.TasksDeleted, fmt.Errorf("load review watch after reset: %w", err)
	}
	if watch == nil || !watch.Enabled {
		return res.TasksDeleted, nil
	}
	go s.reimportReviewWatchAfterReset(runCtx, watch)
	return res.TasksDeleted, nil
}

func (s *Service) reimportReviewWatchAfterReset(ctx context.Context, watch *ReviewWatch) {
	if _, err := s.TriggerReviewWatch(ctx, watch); err != nil {
		s.logger.Warn("reset review watch: re-import failed",
			zap.String("watch_id", watch.ID),
			zap.Error(err))
	}
}
