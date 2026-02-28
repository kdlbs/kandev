package service

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

const (
	defaultLogFlushInterval = 500 * time.Millisecond
	defaultMaxLogBatch      = 100
)

// LogBatcher accumulates log messages and batch-inserts them into the
// database in a single transaction instead of individual INSERTs.
//
// The WS event for each log message is published immediately by the caller
// so the frontend sees logs in real-time. Only the DB write is deferred.
type LogBatcher struct {
	mu       sync.Mutex
	pending  []*models.Message
	repo     repository.MessageRepository
	logger   *logger.Logger
	interval time.Duration
	maxBatch int
	done     chan struct{}
	wg       sync.WaitGroup
}

// NewLogBatcher creates a new log message batcher.
func NewLogBatcher(repo repository.MessageRepository, log *logger.Logger) *LogBatcher {
	return &LogBatcher{
		repo:     repo,
		logger:   log,
		interval: defaultLogFlushInterval,
		maxBatch: defaultMaxLogBatch,
	}
}

// Start begins the periodic flush goroutine.
func (b *LogBatcher) Start() {
	b.done = make(chan struct{})
	b.wg.Add(1)
	go b.flushLoop()
}

// Stop flushes all remaining messages and stops the flush goroutine.
func (b *LogBatcher) Stop() {
	close(b.done)
	b.wg.Wait()
	// Final flush after goroutine exits
	b.flush(context.Background())
}

// Add enqueues a log message for batch insertion.
// If the batch reaches maxBatch, an immediate flush is triggered.
func (b *LogBatcher) Add(message *models.Message) {
	b.mu.Lock()
	b.pending = append(b.pending, message)
	shouldFlush := len(b.pending) >= b.maxBatch
	b.mu.Unlock()

	if shouldFlush {
		b.flush(context.Background())
	}
}

func (b *LogBatcher) flushLoop() {
	defer b.wg.Done()
	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()

	for {
		select {
		case <-b.done:
			return
		case <-ticker.C:
			b.flush(context.Background())
		}
	}
}

func (b *LogBatcher) flush(ctx context.Context) {
	b.mu.Lock()
	if len(b.pending) == 0 {
		b.mu.Unlock()
		return
	}
	batch := b.pending
	b.pending = nil
	b.mu.Unlock()

	if err := b.repo.CreateMessages(ctx, batch); err != nil {
		b.logger.Error("failed to batch-insert log messages",
			zap.Int("count", len(batch)),
			zap.Error(err))
	}
}
