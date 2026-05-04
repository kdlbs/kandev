package github

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// Resource identifies a GitHub API rate-limit bucket.
//
// GitHub maintains separate counters for REST core, GraphQL, and search; the
// `gh` CLI hits GraphQL even for `pr view`, so exhausting one bucket does not
// imply the others are exhausted.
type Resource string

const (
	ResourceCore    Resource = "core"
	ResourceGraphQL Resource = "graphql"
	ResourceSearch  Resource = "search"
)

// rateUpdateDebounce throttles non-transition update events so settings-page
// counters update at human speed rather than once per request.
const rateUpdateDebounce = 5 * time.Second

// RateSnapshot captures the rate-limit state for one bucket at a point in time.
type RateSnapshot struct {
	Resource  Resource  `json:"resource"`
	Remaining int       `json:"remaining"`
	Limit     int       `json:"limit"`
	ResetAt   time.Time `json:"reset_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Exhausted returns true when the bucket has no quota left and ResetAt is in
// the future. Limit may be unknown (0) when the snapshot was synthesized from
// an out-of-band signal (e.g. `gh` stderr).
func (s RateSnapshot) Exhausted() bool {
	return s.Remaining <= 0 && s.ResetAt.After(time.Now())
}

// RateLimitUpdatedEvent is published when a snapshot changes, either because
// new headers arrived or because exhaustion state flipped.
//
// Carries all known buckets so subscribers can surface the full picture
// without follow-up queries.
type RateLimitUpdatedEvent struct {
	Snapshots []RateSnapshot `json:"snapshots"`
	// Trigger names the bucket whose update produced this event; useful for
	// downstream filtering/debug.
	Trigger Resource `json:"trigger"`
	// ExhaustionTransition is non-empty on the tick where the trigger bucket
	// flipped between exhausted and recovered ("exhausted" or "recovered").
	ExhaustionTransition string `json:"exhaustion_transition,omitempty"`
}

// RateTracker accumulates rate-limit snapshots from the GitHub clients and
// publishes events on change.
type RateTracker struct {
	mu          sync.RWMutex
	snapshots   map[Resource]RateSnapshot
	exhausted   map[Resource]bool
	lastEmitted map[Resource]time.Time
	bus         bus.EventBus
	log         *logger.Logger
}

// NewRateTracker constructs a tracker. The event bus may be nil (tests).
func NewRateTracker(eventBus bus.EventBus, log *logger.Logger) *RateTracker {
	return &RateTracker{
		snapshots:   make(map[Resource]RateSnapshot),
		exhausted:   make(map[Resource]bool),
		lastEmitted: make(map[Resource]time.Time),
		bus:         eventBus,
		log:         log,
	}
}

// Record stores a snapshot. Always emits an event on exhaustion transition;
// otherwise debounces to at most one update per resource per
// rateUpdateDebounce window.
func (r *RateTracker) Record(snap RateSnapshot) {
	if snap.Resource == "" {
		return
	}
	if snap.UpdatedAt.IsZero() {
		snap.UpdatedAt = time.Now().UTC()
	}

	r.mu.Lock()
	prev := r.exhausted[snap.Resource]
	now := snap.Exhausted()
	r.snapshots[snap.Resource] = snap
	r.exhausted[snap.Resource] = now

	transition := ""
	switch {
	case !prev && now:
		transition = "exhausted"
	case prev && !now:
		transition = "recovered"
	}

	shouldEmit := transition != ""
	if !shouldEmit {
		last := r.lastEmitted[snap.Resource]
		if snap.UpdatedAt.Sub(last) >= rateUpdateDebounce {
			shouldEmit = true
		}
	}
	if shouldEmit {
		r.lastEmitted[snap.Resource] = snap.UpdatedAt
	}
	allSnap := r.allLocked()
	r.mu.Unlock()

	if !shouldEmit {
		return
	}
	r.publish(allSnap, snap.Resource, transition)
}

// Snapshot returns a copy of the current snapshot for resource.
func (r *RateTracker) Snapshot(resource Resource) (RateSnapshot, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	snap, ok := r.snapshots[resource]
	return snap, ok
}

// All returns a copy of every known snapshot.
func (r *RateTracker) All() map[Resource]RateSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.allLocked()
}

func (r *RateTracker) allLocked() map[Resource]RateSnapshot {
	out := make(map[Resource]RateSnapshot, len(r.snapshots))
	for k, v := range r.snapshots {
		out[k] = v
	}
	return out
}

// IsExhausted reports whether the resource bucket is currently exhausted.
func (r *RateTracker) IsExhausted(resource Resource) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.exhausted[resource]
}

// WaitDuration returns how long callers should wait before retrying the
// resource, or 0 if the bucket is not exhausted.
func (r *RateTracker) WaitDuration(resource Resource) time.Duration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.exhausted[resource] {
		return 0
	}
	d := time.Until(r.snapshots[resource].ResetAt)
	if d < 0 {
		return 0
	}
	return d
}

func (r *RateTracker) publish(all map[Resource]RateSnapshot, trigger Resource, transition string) {
	if r.bus == nil {
		return
	}
	snaps := make([]RateSnapshot, 0, len(all))
	for _, s := range all {
		snaps = append(snaps, s)
	}
	evt := &RateLimitUpdatedEvent{
		Snapshots:            snaps,
		Trigger:              trigger,
		ExhaustionTransition: transition,
	}
	event := bus.NewEvent(events.GitHubRateLimitUpdated, "github", evt)
	if err := r.bus.Publish(context.Background(), events.GitHubRateLimitUpdated, event); err != nil && r.log != nil {
		r.log.Debug("publish rate-limit event failed", zap.Error(err))
	}
}

// parseRateHeaders extracts a snapshot from a GitHub HTTP response. Returns
// false when the response carries no rate-limit headers.
func parseRateHeaders(resp *http.Response, defaultResource Resource) (RateSnapshot, bool) {
	if resp == nil {
		return RateSnapshot{}, false
	}
	limitStr := resp.Header.Get("X-RateLimit-Limit")
	remainingStr := resp.Header.Get("X-RateLimit-Remaining")
	resetStr := resp.Header.Get("X-RateLimit-Reset")
	if limitStr == "" && remainingStr == "" && resetStr == "" {
		return RateSnapshot{}, false
	}
	limit, _ := strconv.Atoi(limitStr)
	remaining, _ := strconv.Atoi(remainingStr)
	reset, _ := strconv.ParseInt(resetStr, 10, 64)

	resource := defaultResource
	if r := resp.Header.Get("X-RateLimit-Resource"); r != "" {
		resource = Resource(strings.ToLower(r))
	}
	return RateSnapshot{
		Resource:  resource,
		Limit:     limit,
		Remaining: remaining,
		ResetAt:   time.Unix(reset, 0).UTC(),
		UpdatedAt: time.Now().UTC(),
	}, true
}

// markRateExhausted records an exhaustion snapshot for callers that detect
// rate-limit conditions out-of-band (e.g. parsing `gh` stderr or 4xx bodies
// without headers). Uses a conservative one-hour reset when no better value
// is known.
func (r *RateTracker) markRateExhausted(resource Resource, resetAt time.Time) {
	if resource == "" {
		return
	}
	if resetAt.IsZero() {
		resetAt = time.Now().Add(time.Hour).UTC()
	}
	prev, _ := r.Snapshot(resource)
	r.Record(RateSnapshot{
		Resource:  resource,
		Limit:     prev.Limit,
		Remaining: 0,
		ResetAt:   resetAt,
		UpdatedAt: time.Now().UTC(),
	})
}
