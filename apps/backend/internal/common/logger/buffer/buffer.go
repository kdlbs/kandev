// Package buffer provides an in-memory ring buffer of structured log entries.
// Used to capture recent log lines for inclusion in user-submitted reports
// (e.g. Improve Kandev) without unbounded memory growth.
package buffer

import (
	"sync"
	"time"

	"go.uber.org/zap/zapcore"
)

// DefaultCapacity is the maximum number of entries kept in the default buffer.
const DefaultCapacity = 2000

// Entry is a structured log record snapshotted from a zap core.
type Entry struct {
	Timestamp time.Time      `json:"timestamp"`
	Level     string         `json:"level"`
	Logger    string         `json:"logger,omitempty"`
	Caller    string         `json:"caller,omitempty"`
	Message   string         `json:"message"`
	Fields    map[string]any `json:"fields,omitempty"`
	Stack     string         `json:"stack,omitempty"`
}

// RingBuffer is a thread-safe FIFO buffer with a fixed capacity.
type RingBuffer struct {
	mu       sync.Mutex
	entries  []Entry
	capacity int
}

// New returns a RingBuffer with the given capacity. A non-positive capacity
// is treated as DefaultCapacity.
func New(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	return &RingBuffer{capacity: capacity, entries: make([]Entry, 0, capacity)}
}

// Push appends e to the buffer, evicting the oldest entry when full.
func (b *RingBuffer) Push(e Entry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.entries) >= b.capacity {
		copy(b.entries, b.entries[1:])
		b.entries = b.entries[:len(b.entries)-1]
	}
	b.entries = append(b.entries, e)
}

// Snapshot returns a copy of the current entries in chronological order.
// Each entry is deep-copied so callers cannot mutate buffered Fields maps.
func (b *RingBuffer) Snapshot() []Entry {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]Entry, len(b.entries))
	for i, e := range b.entries {
		out[i] = e
		if e.Fields != nil {
			fields := make(map[string]any, len(e.Fields))
			for k, v := range e.Fields {
				fields[k] = v
			}
			out[i].Fields = fields
		}
	}
	return out
}

// Len returns the number of entries currently held.
func (b *RingBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.entries)
}

// Clear removes all entries.
func (b *RingBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entries = b.entries[:0]
}

var (
	defaultOnce sync.Once
	defaultBuf  *RingBuffer
)

// Default returns the process-wide ring buffer used by the structured logger.
func Default() *RingBuffer {
	defaultOnce.Do(func() {
		defaultBuf = New(DefaultCapacity)
	})
	return defaultBuf
}

// Core is a zapcore.Core that pushes every emitted entry into a RingBuffer.
// It performs no encoding or output; it is intended to be teed alongside the
// real output core via zapcore.NewTee.
type Core struct {
	zapcore.LevelEnabler
	buffer *RingBuffer
	fields []zapcore.Field
}

// NewCore returns a zapcore.Core that writes to buf at the given level.
func NewCore(buf *RingBuffer, level zapcore.LevelEnabler) zapcore.Core {
	return &Core{LevelEnabler: level, buffer: buf}
}

// With returns a child Core with additional fields attached.
func (c *Core) With(fields []zapcore.Field) zapcore.Core {
	clone := *c
	clone.fields = append(append([]zapcore.Field{}, c.fields...), fields...)
	return &clone
}

// Check adds the core to the checked entry when the level is enabled.
func (c *Core) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(ent.Level) {
		return ce.AddCore(ent, c)
	}
	return ce
}

// Write captures the entry into the ring buffer.
func (c *Core) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	enc := zapcore.NewMapObjectEncoder()
	for _, f := range c.fields {
		f.AddTo(enc)
	}
	for _, f := range fields {
		f.AddTo(enc)
	}
	c.buffer.Push(Entry{
		Timestamp: ent.Time,
		Level:     ent.Level.String(),
		Logger:    ent.LoggerName,
		Caller:    ent.Caller.TrimmedPath(),
		Message:   ent.Message,
		Fields:    enc.Fields,
		Stack:     ent.Stack,
	})
	return nil
}

// Sync is a no-op; the ring buffer holds entries in memory only.
func (c *Core) Sync() error { return nil }
