package metrics

import (
	"context"
	"errors"
	"sync"
	"time"
)

const diskCapacityIdentityTimeout = 250 * time.Millisecond

// DefaultDiskCapacitySampleTTL is the advisory sampling interval shared by
// every target on one agentctl filesystem.
const DefaultDiskCapacitySampleTTL = 30 * time.Second

// ErrDiskCapacitySamplerUnavailable reports an uninitialized sampler.
var ErrDiskCapacitySamplerUnavailable = errors.New("disk capacity sampler is unavailable")

// DiskCapacitySample is one raw filesystem measurement plus the opaque
// source-scoped filesystem identity used for sample sharing.
type DiskCapacitySample struct {
	Capacity     DiskCapacity
	FilesystemID string
	ObservedAt   time.Time
}

type diskCapacitySampleEntry struct {
	done   chan struct{}
	sample DiskCapacitySample
	err    error
}

// DiskCapacitySampler coalesces concurrent advisory reads and reuses a fresh
// raw sample per proven filesystem. It never feeds journal write admission.
type DiskCapacitySampler struct {
	mu       sync.Mutex
	entries  map[string]*diskCapacitySampleEntry
	ttl      time.Duration
	now      func() time.Time
	read     func(context.Context, string) (DiskCapacity, error)
	identity func(string) (string, error)
}

// NewDiskCapacitySampler builds a sampler for one agentctl process.
func NewDiskCapacitySampler() *DiskCapacitySampler {
	return &DiskCapacitySampler{
		entries:  make(map[string]*diskCapacitySampleEntry),
		ttl:      DefaultDiskCapacitySampleTTL,
		now:      time.Now,
		read:     DiskUsage,
		identity: FilesystemIdentity,
	}
}

// SetTTL overrides the sample lifetime. Tests use a short value.
func (s *DiskCapacitySampler) SetTTL(ttl time.Duration) {
	if s == nil || ttl <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ttl = ttl
}

// Sample returns one capacity measurement, reusing a fresh sample for the same
// verified filesystem and coalescing concurrent callers onto one read.
func (s *DiskCapacitySampler) Sample(ctx context.Context, path string) (DiskCapacitySample, error) {
	if s == nil {
		return DiskCapacitySample{}, ErrDiskCapacitySamplerUnavailable
	}
	key, filesystemID := s.sampleKey(ctx, path)
	entry, inFlight := s.acquire(key)
	if !inFlight {
		capacity, err := s.readSample(ctx, path)
		s.complete(key, entry, DiskCapacitySample{
			Capacity:     capacity,
			FilesystemID: filesystemID,
			ObservedAt:   s.now().UTC(),
		}, err)
	}
	return waitDiskCapacitySample(ctx, entry)
}

func (s *DiskCapacitySampler) acquire(key string) (*diskCapacitySampleEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.entries[key]
	if entry != nil && !isCompleted(entry) {
		return entry, true
	}
	if entry != nil && s.now().Sub(entry.sample.ObservedAt) < s.ttl {
		return entry, true
	}
	entry = &diskCapacitySampleEntry{done: make(chan struct{})}
	s.entries[key] = entry
	return entry, false
}

func isCompleted(entry *diskCapacitySampleEntry) bool {
	if entry == nil || entry.done == nil {
		return false
	}
	select {
	case <-entry.done:
		return true
	default:
		return false
	}
}

func (s *DiskCapacitySampler) complete(key string, entry *diskCapacitySampleEntry, sample DiskCapacitySample, err error) {
	s.mu.Lock()
	entry.sample = sample
	entry.err = err
	if err != nil {
		delete(s.entries, key)
	}
	done := entry.done
	s.mu.Unlock()
	close(done)
}

func (s *DiskCapacitySampler) readSample(ctx context.Context, path string) (DiskCapacity, error) {
	readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return s.read(readCtx, path)
}

func (s *DiskCapacitySampler) sampleKey(ctx context.Context, path string) (string, string) {
	if identity := boundedFilesystemIdentity(ctx, s.identity, path); identity != "" {
		return "filesystem:" + identity, identity
	}
	return "path:" + path, ""
}

// identityCalls bounds the potentially blocking stat behind filesystem
// identity. A stalled mount can leave one goroutine, but callers fall back to
// a path key instead of waiting for it.
var identityCalls = make(chan struct{}, 1)

func boundedFilesystemIdentity(
	ctx context.Context,
	identity func(string) (string, error),
	path string,
) string {
	if identity == nil {
		return ""
	}
	timer := time.NewTimer(diskCapacityIdentityTimeout)
	defer timer.Stop()
	select {
	case identityCalls <- struct{}{}:
	case <-ctx.Done():
		return ""
	case <-timer.C:
		return ""
	}
	result := make(chan string, 1)
	go func() {
		defer func() { <-identityCalls }()
		value, err := identity(path)
		if err != nil {
			value = ""
		}
		result <- value
	}()
	select {
	case value := <-result:
		return value
	case <-ctx.Done():
		return ""
	case <-timer.C:
		return ""
	}
}

func waitDiskCapacitySample(ctx context.Context, entry *diskCapacitySampleEntry) (DiskCapacitySample, error) {
	select {
	case <-entry.done:
		if entry.err != nil {
			return DiskCapacitySample{}, entry.err
		}
		return entry.sample, nil
	case <-ctx.Done():
		return DiskCapacitySample{}, ctx.Err()
	}
}
