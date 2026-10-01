package coordinator

import (
	"context"
	"fmt"
	"github.com/kandev/kandev/internal/coordinator/replay"
	"time"

	"go.uber.org/zap"
)

const (
	activityRetentionInterval = 24 * time.Hour
	activityRetentionAge      = 400 * 24 * time.Hour
)

// activityRetentionBatch is the number of rows one delete transaction removes.
var activityRetentionBatch = 500

// DeleteActivityBatch deletes up to limit rows created before cutoff, oldest
// first, in its own transaction, and reports how many it removed.
func (s *Store) DeleteActivityBatch(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	res, err := s.db.ExecContext(ctx, s.db.Rebind(`DELETE FROM coordinator_activity WHERE id IN (
		SELECT id FROM coordinator_activity WHERE created_at < ? ORDER BY created_at, id LIMIT ?)`), cutoff.UTC(), limit)
	if err != nil {
		return 0, fmt.Errorf("delete coordinator activity batch: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("delete coordinator activity batch: %w", err)
	}
	return n, nil
}

// StartActivityRetention starts the daily retention ticker and runs one pass
// at once in its own goroutine, so readiness never waits on it. It does
// nothing while phase 2 is off, and a running ticker never re-reads the flag.
// Both goroutines end when ctx is cancelled.
func (s *Service) StartActivityRetention(ctx context.Context) {
	if !s.phase2 {
		return
	}
	ticker := time.NewTicker(activityRetentionInterval)
	s.startActivityRetention(ctx, ticker.C, ticker.Stop)
}

func (s *Service) startActivityRetention(ctx context.Context, tick <-chan time.Time, stopTicker func()) {
	s.retentionWG.Add(2)
	go func() {
		defer s.retentionWG.Done()
		s.runActivityRetention(ctx)
	}()
	go func() {
		defer s.retentionWG.Done()
		defer stopTicker()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick:
				s.runActivityRetention(ctx)
			}
		}
	}()
}

// WaitActivityRetentionStopped blocks until the retention goroutines have
// exited after their context was cancelled.
func (s *Service) WaitActivityRetentionStopped() { s.retentionWG.Wait() }

// outcomesRetentionBatch is the number of rows of each outcomes table one
// prune transaction removes.
const outcomesRetentionBatch = 200

// runActivityRetention deletes rows older than the retention age in batches.
// A run that starts while another is in progress returns at once; a batch
// error is logged and ends the run, and the next run retries.
func (s *Service) runActivityRetention(ctx context.Context) {
	if !s.retentionRunning.CompareAndSwap(false, true) {
		return
	}
	defer s.retentionRunning.Store(false)
	cutoff := s.store.now().UTC().Add(-activityRetentionAge)
	for ctx.Err() == nil {
		n, err := s.store.DeleteActivityBatch(ctx, cutoff, activityRetentionBatch)
		if err != nil {
			if ctx.Err() == nil {
				s.logger.Warn("activity retention batch failed", zap.Error(err))
			}
			return
		}
		if n < int64(activityRetentionBatch) {
			break
		}
	}
	s.pruneOutcomeRows(ctx)
	s.pruneReplayRows(ctx)
}

// pruneOutcomeRows deletes outcome, feedback and seen rows past their
// retention in batches; an error is logged and the next run retries.
func (s *Service) pruneOutcomeRows(ctx context.Context) {
	cutoff := s.store.now().UTC().Add(-outcomesRetention)
	for ctx.Err() == nil {
		n, err := s.store.PruneOutcomeRows(ctx, cutoff, outcomesRetentionBatch)
		if err != nil {
			if ctx.Err() == nil {
				s.logger.Warn("outcomes retention batch failed", zap.Error(err))
			}
			return
		}
		if n == 0 {
			return
		}
	}
}

// pruneReplayRows deletes replay result rows past their retention in batches;
// an error is logged and the next run retries.
func (s *Service) pruneReplayRows(ctx context.Context) {
	cutoff := s.store.now().UTC().Add(-replay.Retention)
	for ctx.Err() == nil {
		n, err := s.store.PruneReplayResults(ctx, cutoff, outcomesRetentionBatch)
		if err != nil {
			if ctx.Err() == nil {
				s.logger.Warn("replay retention batch failed", zap.Error(err))
			}
			return
		}
		if n == 0 {
			return
		}
	}
}
