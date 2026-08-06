package lsp

import (
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const DefaultMaxServers = 8

type TaskLanguageKey struct {
	TaskID   string
	Language string
}

type QueueEntry struct {
	Key        TaskLanguageKey
	Generation uint64
	AcceptedAt time.Time
}

// Capacity counts real task/language server slots. Browser attachments and
// editor mounts do not consume capacity and cannot release a slot.
type Capacity struct {
	mu       sync.Mutex
	limit    int
	active   map[TaskLanguageKey]uint64
	queued   map[TaskLanguageKey]QueueEntry
	epoch    string
	revision uint64
}

func NewCapacity(limit int) *Capacity {
	if limit <= 0 {
		limit = DefaultMaxServers
	}
	return &Capacity{
		limit:  limit,
		active: make(map[TaskLanguageKey]uint64),
		queued: make(map[TaskLanguageKey]QueueEntry),
		epoch:  time.Now().UTC().Format("20060102T150405.000000000Z"),
	}
}

func NewCapacityFromEnv() *Capacity {
	value, present := os.LookupEnv("KANDEV_LSP_MAX_SERVERS")
	if !present {
		value = os.Getenv("KANDEV_LSP_MAX_CONNECTIONS")
	}
	limit, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || limit <= 0 {
		limit = DefaultMaxServers
	}
	return NewCapacity(limit)
}

// Admit reserves one server slot, or places the accepted command in the
// deterministic queue. Re-admitting an active task/language only advances its
// generation and never consumes another slot.
func (c *Capacity) Admit(key TaskLanguageKey, generation uint64, acceptedAt time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if activeGeneration, ok := c.active[key]; ok {
		if generation >= activeGeneration {
			c.active[key] = generation
		}
		return true
	}
	if queued, ok := c.queued[key]; ok {
		if generation > queued.Generation {
			queued.Generation = generation
			c.queued[key] = queued
		}
		return false
	}
	if len(c.active) < c.limit {
		c.active[key] = generation
		c.revision++
		return true
	}
	c.queued[key] = QueueEntry{Key: key, Generation: generation, AcceptedAt: acceptedAt}
	c.revision++
	return false
}

// Adopt accounts for a server proven live during recovery. Existing processes
// cannot be queued retroactively, so adoption may temporarily exceed the
// configured admission limit while still preventing new starts.
func (c *Capacity) Adopt(key TaskLanguageKey, generation uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	activeBefore, queuedBefore := len(c.active), len(c.queued)
	delete(c.queued, key)
	if current, ok := c.active[key]; !ok || generation >= current {
		c.active[key] = generation
	}
	if len(c.active) != activeBefore || len(c.queued) != queuedBefore {
		c.revision++
	}
}

// Release frees only the matching real server generation, then atomically
// promotes the oldest accepted queued task/language.
func (c *Capacity) Release(key TaskLanguageKey, generation uint64) *QueueEntry {
	c.mu.Lock()
	defer c.mu.Unlock()

	activeGeneration, ok := c.active[key]
	if !ok || activeGeneration != generation {
		return nil
	}
	delete(c.active, key)
	c.revision++
	if len(c.queued) == 0 {
		return nil
	}
	entries := make([]QueueEntry, 0, len(c.queued))
	for _, entry := range c.queued {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if !entries[i].AcceptedAt.Equal(entries[j].AcceptedAt) {
			return entries[i].AcceptedAt.Before(entries[j].AcceptedAt)
		}
		if entries[i].Key.TaskID != entries[j].Key.TaskID {
			return entries[i].Key.TaskID < entries[j].Key.TaskID
		}
		return entries[i].Key.Language < entries[j].Key.Language
	})
	next := entries[0]
	delete(c.queued, next.Key)
	c.active[next.Key] = next.Generation
	return &next
}

func (c *Capacity) CancelQueued(key TaskLanguageKey) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.queued[key]; !ok {
		return false
	}
	delete(c.queued, key)
	c.revision++
	return true
}

func (c *Capacity) Snapshot() CapacitySnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return CapacitySnapshot{
		Active: len(c.active), Queued: len(c.queued), Limit: c.limit,
		Epoch: c.epoch, Revision: c.revision,
	}
}

func (c *Capacity) Active() int {
	return c.Snapshot().Active
}

func (c *Capacity) Queued() int {
	return c.Snapshot().Queued
}

func (c *Capacity) Limit() int { return c.limit }
