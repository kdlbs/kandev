package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// mockBatchRepo tracks CreateMessages calls for LogBatcher tests.
type mockBatchRepo struct {
	mockMessageRepo
	batchMu     sync.Mutex
	batchCalls  int
	batchedMsgs []*models.Message
}

func newMockBatchRepo() *mockBatchRepo {
	return &mockBatchRepo{
		mockMessageRepo: mockMessageRepo{
			messages: make(map[string]*models.Message),
			appends:  make(map[string]string),
		},
	}
}

func (m *mockBatchRepo) CreateMessages(_ context.Context, msgs []*models.Message) error {
	m.batchMu.Lock()
	defer m.batchMu.Unlock()
	m.batchCalls++
	m.batchedMsgs = append(m.batchedMsgs, msgs...)
	return nil
}

func (m *mockBatchRepo) getBatchCalls() int {
	m.batchMu.Lock()
	defer m.batchMu.Unlock()
	return m.batchCalls
}

func (m *mockBatchRepo) getBatchedCount() int {
	m.batchMu.Lock()
	defer m.batchMu.Unlock()
	return len(m.batchedMsgs)
}

func TestLogBatcher_Add(t *testing.T) {
	repo := newMockBatchRepo()
	batcher := NewLogBatcher(repo, testLogger())

	// Add a message without starting the batcher — should accumulate
	batcher.Add(&models.Message{ID: "log-1", Content: "test log"})

	batcher.mu.Lock()
	pendingCount := len(batcher.pending)
	batcher.mu.Unlock()

	if pendingCount != 1 {
		t.Errorf("expected 1 pending message, got %d", pendingCount)
	}

	// No DB writes should have happened
	if got := repo.getBatchCalls(); got != 0 {
		t.Errorf("expected 0 batch calls, got %d", got)
	}
}

func TestLogBatcher_PeriodicFlush(t *testing.T) {
	repo := newMockBatchRepo()
	batcher := NewLogBatcher(repo, testLogger())
	batcher.interval = 50 * time.Millisecond
	batcher.Start()
	defer batcher.Stop()

	batcher.Add(&models.Message{ID: "log-1", Content: "first"})
	batcher.Add(&models.Message{ID: "log-2", Content: "second"})

	// Wait for flush
	time.Sleep(100 * time.Millisecond)

	if got := repo.getBatchedCount(); got != 2 {
		t.Errorf("expected 2 batched messages, got %d", got)
	}
	if got := repo.getBatchCalls(); got != 1 {
		t.Errorf("expected 1 batch call, got %d", got)
	}
}

func TestLogBatcher_MaxBatchFlush(t *testing.T) {
	repo := newMockBatchRepo()
	batcher := NewLogBatcher(repo, testLogger())
	batcher.interval = 1 * time.Hour // Ensure periodic flush doesn't fire
	batcher.maxBatch = 5
	batcher.Start()
	defer batcher.Stop()

	// Add exactly maxBatch messages
	for i := 0; i < 5; i++ {
		batcher.Add(&models.Message{ID: "log-" + string(rune('a'+i)), Content: "msg"})
	}

	// Give a moment for the flush goroutine triggered by Add
	time.Sleep(20 * time.Millisecond)

	if got := repo.getBatchedCount(); got != 5 {
		t.Errorf("expected 5 batched messages after max batch, got %d", got)
	}
}

func TestLogBatcher_Stop(t *testing.T) {
	repo := newMockBatchRepo()
	batcher := NewLogBatcher(repo, testLogger())
	batcher.interval = 1 * time.Hour // Long interval
	batcher.Start()

	batcher.Add(&models.Message{ID: "log-1", Content: "pending"})
	batcher.Add(&models.Message{ID: "log-2", Content: "pending2"})

	// Stop should flush remaining
	batcher.Stop()

	if got := repo.getBatchedCount(); got != 2 {
		t.Errorf("expected 2 batched messages after Stop, got %d", got)
	}
}

func TestLogBatcher_ConcurrentAdds(t *testing.T) {
	repo := newMockBatchRepo()
	batcher := NewLogBatcher(repo, testLogger())
	batcher.interval = 50 * time.Millisecond
	batcher.Start()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			batcher.Add(&models.Message{ID: "log-concurrent", Content: "x"})
		}(i)
	}
	wg.Wait()

	// Stop to force final flush
	batcher.Stop()

	if got := repo.getBatchedCount(); got != 50 {
		t.Errorf("expected 50 batched messages, got %d", got)
	}
}

func TestLogBatcher_EmptyFlush(t *testing.T) {
	repo := newMockBatchRepo()
	batcher := NewLogBatcher(repo, testLogger())
	batcher.interval = 50 * time.Millisecond
	batcher.Start()

	// Let a few ticks pass with no messages
	time.Sleep(120 * time.Millisecond)

	batcher.Stop()

	// No batch calls should have been made
	if got := repo.getBatchCalls(); got != 0 {
		t.Errorf("expected 0 batch calls for empty batcher, got %d", got)
	}
}
