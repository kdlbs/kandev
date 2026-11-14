package service

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"go.uber.org/zap"
)

type UsageProjectionBatchStore interface {
	RunUsageHistoryProjectionBatch(context.Context, int) (bool, error)
}

type usageSelectionBackfiller interface {
	BackfillUsageSelection(context.Context, int) (bool, error)
}

type UsageHistoryWorker struct {
	store       UsageProjectionBatchStore
	log         *logger.Logger
	mu          sync.Mutex
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	batches     atomic.Uint64
	failures    atomic.Uint64
	nanoseconds atomic.Uint64
}

func NewUsageHistoryWorker(store UsageProjectionBatchStore, log *logger.Logger) *UsageHistoryWorker {
	return &UsageHistoryWorker{store: store, log: log}
}

func (w *UsageHistoryWorker) Start(parent context.Context) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	w.cancel = cancel
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.run(ctx)
	}()
}

func (w *UsageHistoryWorker) Stop() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		w.cancel()
		w.wg.Wait()
		w.cancel = nil
	}
	return nil
}

// Statistics uses bounded counters without task, actor, or source labels.
func (w *UsageHistoryWorker) Statistics() (batches, failures uint64, duration time.Duration) {
	return w.batches.Load(), w.failures.Load(), time.Duration(w.nanoseconds.Load())
}

func (w *UsageHistoryWorker) run(ctx context.Context) {
	migrated, indexed := false, false
	for ctx.Err() == nil {
		started := time.Now()
		operation, cancel := context.WithTimeout(ctx, 10*time.Second)
		done, err := w.step(operation, &migrated, &indexed)
		cancel()
		w.batches.Add(1)
		w.nanoseconds.Add(uint64(time.Since(started)))
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			w.failures.Add(1)
			if w.log != nil {
				w.log.Warn("usage history projection retained pending work", zap.Error(err), zap.Duration("duration", time.Since(started)))
			}
		}
		if done || err != nil {
			if done && w.log != nil {
				batches, failures, duration := w.Statistics()
				w.log.Debug("usage history projection idle", zap.Uint64("batches", batches), zap.Uint64("failures", failures), zap.Duration("duration", duration))
			}
			timer := time.NewTimer(time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}
}

func (w *UsageHistoryWorker) step(ctx context.Context, migrated, indexed *bool) (bool, error) {
	if !*migrated {
		if migrator, ok := w.store.(usagehistory.Migrator); ok {
			done, err := migrator.BackfillUsageHistory(ctx, 200)
			*migrated = done && err == nil
			return false, err
		}
		*migrated = true
	}
	if !*indexed {
		if indexer, ok := w.store.(usageSelectionBackfiller); ok {
			done, err := indexer.BackfillUsageSelection(ctx, 200)
			*indexed = done && err == nil
			return false, err
		}
		*indexed = true
	}
	return w.store.RunUsageHistoryProjectionBatch(ctx, 200)
}
