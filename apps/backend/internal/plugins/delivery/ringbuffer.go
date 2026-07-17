package delivery

import (
	"sync"
	"time"

	"github.com/kandev/kandev/pkg/pluginsdk"
)

// Delivery is one queued/buffered event delivery job for a single plugin.
type Delivery struct {
	PluginID string
	Event    *pluginsdk.Event
}

// bufferedDelivery pairs a Delivery with the time it was buffered, for TTL
// eviction.
type bufferedDelivery struct {
	delivery Delivery
	addedAt  time.Time
}

// ringBuffer holds events for one plugin while it is in the error state, so
// Deliverer.Flush can replay them in order after a health-poller recovery
// transition. Capacity and TTL are the numbers in
// docs/specs/plugins/spec.md ("Event webhook delivery"): 100 events / 5
// minutes. The oldest entry is dropped on overflow; expired entries are
// purged lazily on the next Add, Drain, or Len call.
type ringBuffer struct {
	mu    sync.Mutex
	cap   int
	ttl   time.Duration
	items []bufferedDelivery
	nowFn func() time.Time
}

// newRingBuffer returns a ring buffer bounded to capacity entries, each
// evicted after ttl. nowFn defaults to time.Now when nil; tests inject a
// fake clock for deterministic TTL assertions.
func newRingBuffer(capacity int, ttl time.Duration, nowFn func() time.Time) *ringBuffer {
	if nowFn == nil {
		nowFn = time.Now
	}
	return &ringBuffer{cap: capacity, ttl: ttl, nowFn: nowFn}
}

// Add appends d to the buffer, purging expired entries first and then
// evicting the oldest remaining entry if still at capacity. Returns the
// event id of the entry dropped due to overflow, or "" if none was
// dropped.
func (r *ringBuffer) Add(d Delivery) (droppedID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.purgeExpiredLocked()

	if r.cap > 0 && len(r.items) >= r.cap && len(r.items) > 0 {
		droppedID = r.items[0].delivery.Event.EventID
		r.items = r.items[1:]
	}
	r.items = append(r.items, bufferedDelivery{delivery: d, addedAt: r.nowFn()})
	return droppedID
}

// Drain returns every non-expired buffered delivery in insertion (oldest
// first) order and empties the buffer.
func (r *ringBuffer) Drain() []Delivery {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.purgeExpiredLocked()
	out := make([]Delivery, len(r.items))
	for i, it := range r.items {
		out[i] = it.delivery
	}
	r.items = nil
	return out
}

// Len reports the number of currently buffered (non-expired) deliveries.
func (r *ringBuffer) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.purgeExpiredLocked()
	return len(r.items)
}

// purgeExpiredLocked drops entries older than ttl from the front of items
// (insertion order == age order, so this is a prefix trim). Caller must
// hold mu.
func (r *ringBuffer) purgeExpiredLocked() {
	if r.ttl <= 0 {
		return
	}
	cutoff := r.nowFn().Add(-r.ttl)
	idx := 0
	for idx < len(r.items) && r.items[idx].addedAt.Before(cutoff) {
		idx++
	}
	if idx > 0 {
		r.items = r.items[idx:]
	}
}
