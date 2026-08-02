package lifecycle

import (
	"sync"
	"time"
)

const defaultStreamCoalesceWindow = 100 * time.Millisecond

type coalescedStreamChunk struct {
	eventType string
	messageID string
	content   string
	isAppend  bool
}

// streamCoalescer combines adjacent append chunks for one execution. The
// first non-empty chunk of a record is emitted immediately; later chunks wait for the
// bounded window or an explicit lifecycle boundary. A single pending segment
// is intentional: combining across another message ID would change wire
// ordering.
type streamCoalescer struct {
	emitMu         sync.Mutex
	mu             sync.Mutex
	window         time.Duration
	pending        *coalescedStreamChunk
	timer          *time.Timer
	closed         bool
	publish        func(coalescedStreamChunk)
	lastEventType  string
	lastMessageID  string
	forceImmediate bool
	received       int
	coalesced      int
	flushed        int
}

type streamCoalescerStats struct {
	received  int
	coalesced int
	flushed   int
}

func newStreamCoalescer(window time.Duration, publish func(coalescedStreamChunk)) *streamCoalescer {
	if window <= 0 {
		window = defaultStreamCoalesceWindow
	}
	return &streamCoalescer{window: window, publish: publish}
}

func (c *streamCoalescer) add(chunk coalescedStreamChunk) {
	if chunk.content == "" {
		return
	}

	c.emitMu.Lock()
	defer c.emitMu.Unlock()

	var ready []coalescedStreamChunk
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.received++

	sameAsLast := c.lastEventType == chunk.eventType && c.lastMessageID == chunk.messageID
	immediate := !chunk.isAppend || c.forceImmediate || !sameAsLast
	c.forceImmediate = false
	switch {
	case immediate:
		ready = c.detachLocked(ready)
		ready = append(ready, chunk)
	case c.pending != nil && c.pending.eventType == chunk.eventType && c.pending.messageID == chunk.messageID:
		c.pending.content += chunk.content
		c.coalesced++
	default:
		ready = c.detachLocked(ready)
		pending := chunk
		c.pending = &pending
		c.coalesced++
		c.timer = time.AfterFunc(c.window, c.flush)
	}
	c.lastEventType = chunk.eventType
	c.lastMessageID = chunk.messageID
	c.mu.Unlock()

	c.publishReady(ready)
}

func (c *streamCoalescer) flush() {
	c.emitMu.Lock()
	defer c.emitMu.Unlock()
	c.mu.Lock()
	ready := c.detachLocked(nil)
	c.mu.Unlock()
	c.publishReady(ready)
}

func (c *streamCoalescer) flushBoundary() {
	c.emitMu.Lock()
	defer c.emitMu.Unlock()
	c.mu.Lock()
	c.forceImmediate = true
	ready := c.detachLocked(nil)
	c.mu.Unlock()
	c.publishReady(ready)
}

func (c *streamCoalescer) close() {
	c.emitMu.Lock()
	defer c.emitMu.Unlock()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	ready := c.detachLocked(nil)
	c.mu.Unlock()
	c.publishReady(ready)
}

func (c *streamCoalescer) detachLocked(ready []coalescedStreamChunk) []coalescedStreamChunk {
	// A timer callback that already fired can be waiting on emitMu while add
	// installs a later pending segment and timer. When that callback acquires
	// emitMu it may detach the later segment immediately, collapsing its window;
	// content and ordering remain correct because all detaches are serialized.
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	if c.pending != nil {
		ready = append(ready, *c.pending)
		c.pending = nil
		c.flushed++
	}
	return ready
}

func (c *streamCoalescer) stats() streamCoalescerStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return streamCoalescerStats{
		received:  c.received,
		coalesced: c.coalesced,
		flushed:   c.flushed,
	}
}

func (c *streamCoalescer) publishReady(chunks []coalescedStreamChunk) {
	if c.publish == nil {
		return
	}
	for _, chunk := range chunks {
		c.publish(chunk)
	}
}
