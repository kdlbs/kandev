// Package conversationsearch owns the cancellable conversation indexing worker.
package conversationsearch

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// Store keeps indexing independent of the database dialect and application lifecycle.
type Store interface {
	ConversationSearchReady(context.Context) bool
	BackfillConversationSearch(context.Context, int) (bool, error)
	ProcessConversationSearchBatch(context.Context, bool) (int, error)
}

// Start returns an idempotent stop function that joins all database work.
func Start(ctx context.Context, store Store, log *logger.Logger) func() {
	return start(ctx, store, time.Second, log)
}

func start(ctx context.Context, store Store, interval time.Duration, log *logger.Logger) func() {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if !store.ConversationSearchReady(ctx) {
			return
		}
		run(ctx, store, interval, log)

	}()
	return func() { cancel(); <-done }
}

func run(ctx context.Context, store Store, interval time.Duration, log *logger.Logger) {
	reporter := indexErrorReporter{log: log}
	pause := min(interval, 25*time.Millisecond)
	ticker := time.NewTicker(pause)
	defer ticker.Stop()
	var nextLive, nextHistory time.Time
	livePasses := 0
	liveBusy := false
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if !now.Before(nextLive) {
				count, err := store.ProcessConversationSearchBatch(ctx, false)
				reporter.report(ctx, err, "live_batch_failed", now)
				liveBusy = count > 0 || err != nil
				livePasses++
				nextLive = time.Now().Add(interval)
			}
			if now.Before(nextHistory) || (liveBusy && livePasses < 3) {
				continue
			}
			livePasses = 0
			nextHistory = time.Now().Add(interval)
			if _, err := store.BackfillConversationSearch(ctx, 32); err != nil {
				reporter.report(ctx, err, "backfill_enqueue_failed", now)
				continue
			}
			count, err := store.ProcessConversationSearchBatch(ctx, true)
			reporter.report(ctx, err, "backfill_batch_failed", now)
			// Empty queues back off; historical batches yield without delaying live work.
			if count > 0 && err == nil {
				nextHistory = time.Now().Add(pause)
			}
		}
	}
}

type indexErrorReporter struct {
	log     *logger.Logger
	nextLog time.Time
}

func (r *indexErrorReporter) report(ctx context.Context, err error, reason string, now time.Time) {
	if err == nil || ctx.Err() != nil || r.log == nil || now.Before(r.nextLog) {
		return
	}
	r.nextLog = now.Add(time.Minute)
	r.log.Warn("conversation indexing failed, retry scheduled", zap.String("reason", reason))
}
