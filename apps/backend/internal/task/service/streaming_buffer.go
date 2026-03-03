package service

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

const (
	defaultFlushInterval = 500 * time.Millisecond
)

// bufEntry holds the cached state of a streaming message.
type bufEntry struct {
	message  *models.Message // full cached message (loaded on first access)
	pending  string          // accumulated content delta since last flush
	dirty    bool
	thinking bool // true if this entry tracks metadata.thinking, not content
}

// StreamingBuffer coalesces streaming message writes by caching messages in
// memory and flushing accumulated content to the database periodically.
//
// Instead of a GET + full-row UPDATE per text chunk (~40 round-trips per
// agent response), the buffer reduces this to 1 GET + ~5-8 SQL appends.
//
// The buffer is safe because:
//   - CREATEs still go through the normal immediate DB path
//   - The first APPEND loads the message from DB (guaranteed to exist)
//   - Subsequent APPENDs update the in-memory cache only
//   - A background goroutine flushes dirty entries every 500ms
//   - Callers receive the cached message (with full content) for WS events
type StreamingBuffer struct {
	mu       sync.Mutex
	entries  map[string]*bufEntry
	repo     repository.MessageRepository
	logger   *logger.Logger
	interval time.Duration
	done     chan struct{}
	wg       sync.WaitGroup
}

// NewStreamingBuffer creates a new streaming write buffer.
func NewStreamingBuffer(repo repository.MessageRepository, log *logger.Logger) *StreamingBuffer {
	return &StreamingBuffer{
		entries:  make(map[string]*bufEntry),
		repo:     repo,
		logger:   log,
		interval: defaultFlushInterval,
	}
}

// Start begins the periodic flush goroutine.
func (b *StreamingBuffer) Start() {
	b.done = make(chan struct{})
	b.wg.Add(1)
	go b.flushLoop()
}

// Stop flushes all remaining entries and stops the flush goroutine.
func (b *StreamingBuffer) Stop() {
	close(b.done)
	b.wg.Wait()
	// Final flush after goroutine exits
	b.flushAll(context.Background())
}

// Append accumulates a text chunk for the given message. On first call for a
// messageID it loads the message from the database; subsequent calls use the
// cached copy. Returns a snapshot of the message with full accumulated content,
// suitable for publishing WS events.
func (b *StreamingBuffer) Append(ctx context.Context, messageID, content string) (*models.Message, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	entry, ok := b.entries[messageID]
	if !ok {
		msg, err := b.repo.GetMessage(ctx, messageID)
		if err != nil {
			return nil, err
		}
		entry = &bufEntry{message: msg}
		b.entries[messageID] = entry
	}

	entry.message.Content += content
	entry.pending += content
	entry.dirty = true

	return entry.message, nil
}

// AppendThinking accumulates a thinking text chunk for the given message.
// Thinking content is stored in metadata.thinking rather than in the content
// field, so it requires a full metadata UPDATE on flush.
func (b *StreamingBuffer) AppendThinking(ctx context.Context, messageID, content string) (*models.Message, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	entry, ok := b.entries[messageID]
	if !ok {
		msg, err := b.repo.GetMessage(ctx, messageID)
		if err != nil {
			return nil, err
		}
		entry = &bufEntry{message: msg, thinking: true}
		b.entries[messageID] = entry
	}

	if entry.message.Metadata == nil {
		entry.message.Metadata = make(map[string]interface{})
	}

	existing := ""
	if v, ok := entry.message.Metadata["thinking"].(string); ok {
		existing = v
	}
	entry.message.Metadata["thinking"] = existing + content
	entry.pending += content
	entry.dirty = true

	return entry.message, nil
}

// Finalize force-flushes and removes a specific message from the buffer.
// Call this when a streaming message is complete (e.g., on turn boundary).
func (b *StreamingBuffer) Finalize(ctx context.Context, messageID string) {
	b.mu.Lock()
	entry, ok := b.entries[messageID]
	if !ok {
		b.mu.Unlock()
		return
	}
	delta := entry.pending
	isThinking := entry.thinking
	msg := entry.message
	entry.pending = ""
	entry.dirty = false
	delete(b.entries, messageID)
	b.mu.Unlock()

	if delta == "" {
		return
	}

	if isThinking {
		b.flushThinkingEntry(ctx, msg)
	} else {
		b.flushContentEntry(ctx, messageID, delta)
	}
}

// FinalizeAll force-flushes all entries for a given session.
func (b *StreamingBuffer) FinalizeAll(ctx context.Context, sessionID string) {
	b.mu.Lock()
	var toFlush []struct {
		id       string
		delta    string
		thinking bool
		msg      *models.Message
	}
	for id, entry := range b.entries {
		if entry.message.TaskSessionID != sessionID {
			continue
		}
		if entry.dirty && entry.pending != "" {
			toFlush = append(toFlush, struct {
				id       string
				delta    string
				thinking bool
				msg      *models.Message
			}{id, entry.pending, entry.thinking, entry.message})
		}
		entry.pending = ""
		entry.dirty = false
		delete(b.entries, id)
	}
	b.mu.Unlock()

	for _, item := range toFlush {
		if item.thinking {
			b.flushThinkingEntry(ctx, item.msg)
		} else {
			b.flushContentEntry(ctx, item.id, item.delta)
		}
	}
}

func (b *StreamingBuffer) flushLoop() {
	defer b.wg.Done()
	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()

	for {
		select {
		case <-b.done:
			return
		case <-ticker.C:
			b.flushAll(context.Background())
		}
	}
}

func (b *StreamingBuffer) flushAll(ctx context.Context) {
	b.mu.Lock()
	var contentFlush []struct {
		id    string
		delta string
	}
	var thinkingFlush []*models.Message

	for id, entry := range b.entries {
		if !entry.dirty || entry.pending == "" {
			continue
		}
		if entry.thinking {
			// Clone metadata for thread safety
			meta := cloneMetadata(entry.message.Metadata)
			msgCopy := *entry.message
			msgCopy.Metadata = meta
			thinkingFlush = append(thinkingFlush, &msgCopy)
		} else {
			contentFlush = append(contentFlush, struct {
				id    string
				delta string
			}{id, entry.pending})
		}
		entry.pending = ""
		entry.dirty = false
	}
	b.mu.Unlock()

	for _, item := range contentFlush {
		b.flushContentEntry(ctx, item.id, item.delta)
	}
	for _, msg := range thinkingFlush {
		b.flushThinkingEntry(ctx, msg)
	}
}

func (b *StreamingBuffer) flushContentEntry(ctx context.Context, messageID, delta string) {
	if err := b.repo.AppendContent(ctx, messageID, delta); err != nil {
		b.logger.Error("failed to flush streaming content",
			zap.String("message_id", messageID),
			zap.Int("delta_len", len(delta)),
			zap.Error(err))
	}
}

func (b *StreamingBuffer) flushThinkingEntry(ctx context.Context, msg *models.Message) {
	if err := b.repo.UpdateMessage(ctx, msg); err != nil {
		b.logger.Error("failed to flush thinking content",
			zap.String("message_id", msg.ID),
			zap.Error(err))
	}
}

func cloneMetadata(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	// Deep-enough clone: re-marshal + unmarshal to get independent copy.
	// This is called infrequently (only on flush ticks), so the cost is fine.
	data, err := json.Marshal(m)
	if err != nil {
		// Fallback: shallow copy
		cp := make(map[string]interface{}, len(m))
		for k, v := range m {
			cp[k] = v
		}
		return cp
	}
	cp := make(map[string]interface{})
	_ = json.Unmarshal(data, &cp)
	return cp
}
