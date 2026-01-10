package process

import (
	"sync"
	"time"
)

// OutputLine represents a line of output from the agent
type OutputLine struct {
	Timestamp time.Time `json:"timestamp"`
	Stream    string    `json:"stream"` // "stdout" or "stderr"
	Content   string    `json:"content"`
}

// Subscriber is a channel that receives output lines
type Subscriber chan OutputLine

// OutputBuffer is a ring buffer for agent output that supports streaming
type OutputBuffer struct {
	lines   []OutputLine
	size    int
	head    int
	count   int
	mu      sync.RWMutex
	
	// Subscribers for real-time streaming
	subscribers map[Subscriber]struct{}
	subMu       sync.RWMutex
}

// NewOutputBuffer creates a new output buffer with the given capacity
func NewOutputBuffer(size int) *OutputBuffer {
	return &OutputBuffer{
		lines:       make([]OutputLine, size),
		size:        size,
		subscribers: make(map[Subscriber]struct{}),
	}
}

// Add adds a new line to the buffer and notifies subscribers
func (b *OutputBuffer) Add(line OutputLine) {
	b.mu.Lock()
	
	// Add to ring buffer
	idx := (b.head + b.count) % b.size
	if b.count < b.size {
		b.count++
	} else {
		b.head = (b.head + 1) % b.size
	}
	b.lines[idx] = line
	
	b.mu.Unlock()

	// Notify subscribers (non-blocking)
	b.subMu.RLock()
	for sub := range b.subscribers {
		select {
		case sub <- line:
		default:
			// Subscriber is slow, skip
		}
	}
	b.subMu.RUnlock()
}

// GetAll returns all lines in the buffer (oldest first)
func (b *OutputBuffer) GetAll() []OutputLine {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make([]OutputLine, b.count)
	for i := 0; i < b.count; i++ {
		idx := (b.head + i) % b.size
		result[i] = b.lines[idx]
	}
	return result
}

// GetLast returns the last n lines from the buffer
func (b *OutputBuffer) GetLast(n int) []OutputLine {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if n > b.count {
		n = b.count
	}

	result := make([]OutputLine, n)
	start := b.count - n
	for i := 0; i < n; i++ {
		idx := (b.head + start + i) % b.size
		result[i] = b.lines[idx]
	}
	return result
}

// Subscribe creates a new subscriber that receives output lines in real-time
func (b *OutputBuffer) Subscribe() Subscriber {
	sub := make(Subscriber, 100)
	
	b.subMu.Lock()
	b.subscribers[sub] = struct{}{}
	b.subMu.Unlock()
	
	return sub
}

// Unsubscribe removes a subscriber
func (b *OutputBuffer) Unsubscribe(sub Subscriber) {
	b.subMu.Lock()
	delete(b.subscribers, sub)
	b.subMu.Unlock()
	close(sub)
}

// Count returns the number of lines in the buffer
func (b *OutputBuffer) Count() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.count
}

// Clear clears the buffer
func (b *OutputBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.head = 0
	b.count = 0
}

