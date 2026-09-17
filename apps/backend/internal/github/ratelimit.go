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
	Resource          Resource  `json:"resource"`
	Remaining         int       `json:"remaining"`
	RemainingObserved bool      `json:"-"`
	ParsedFromHeaders bool      `json:"-"`
	Limit             int       `json:"limit"`
	ResetAt           time.Time `json:"reset_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// SecondaryRateLimitState is Kandev's observed and locally enforced view of a
// GitHub secondary throttle. GitHub exposes no authoritative status endpoint.
type SecondaryRateLimitState struct {
	Resource    Resource    `json:"resource"`
	Active      bool        `json:"active"`
	RetryAt     time.Time   `json:"retry_at"`
	ObservedAt  time.Time   `json:"observed_at"`
	RetrySource RetrySource `json:"retry_source"`
	Reason      string      `json:"reason,omitempty"`
}

// Exhausted returns true when the bucket has no quota left and ResetAt is in
// the future. Limit may be unknown (0) when the snapshot was synthesized from
// an out-of-band signal (e.g. `gh` stderr).
func (s RateSnapshot) Exhausted() bool {
	return s.Remaining <= 0 && s.ResetAt.After(time.Now()) &&
		(s.RemainingObserved || !s.ParsedFromHeaders)
}

// BackgroundReserve returns the quota retained for interactive requests.
func (s RateSnapshot) BackgroundReserve() int {
	if s.Limit <= 0 {
		return 0
	}
	return (s.Limit + 9) / 10
}

// interactiveReserveReached reports whether only the ten-percent interactive
// reserve remains in a known primary bucket.
func interactiveReserveReached(snap RateSnapshot) bool {
	return snap.Remaining <= snap.BackgroundReserve() && snap.BackgroundReserve() > 0
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
	secondary   map[Resource]SecondaryRateLimitState
	changed     chan struct{}
	refreshMu   sync.Mutex
	refreshDone chan struct{}
	bus         bus.EventBus
	log         *logger.Logger
}

// NewRateTracker constructs a tracker. The event bus may be nil (tests).
func NewRateTracker(eventBus bus.EventBus, log *logger.Logger) *RateTracker {
	return &RateTracker{
		snapshots:   make(map[Resource]RateSnapshot),
		exhausted:   make(map[Resource]bool),
		lastEmitted: make(map[Resource]time.Time),
		secondary:   make(map[Resource]SecondaryRateLimitState),
		changed:     make(chan struct{}),
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
	r.signalChangedLocked()

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

func (r *RateTracker) Changed() <-chan struct{} {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.changed
}

func (r *RateTracker) signalChangedLocked() {
	close(r.changed)
	r.changed = make(chan struct{})
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

func (r *RateTracker) beginRateLimitRefresh(force bool) (chan struct{}, bool) {
	r.refreshMu.Lock()
	defer r.refreshMu.Unlock()
	if r.refreshDone != nil {
		return r.refreshDone, false
	}
	if !force && hasCompleteRateLimitSnapshot(r) {
		return nil, false
	}
	done := make(chan struct{})
	r.refreshDone = done
	return done, true
}

func (r *RateTracker) finishRateLimitRefresh(done chan struct{}) {
	r.refreshMu.Lock()
	defer r.refreshMu.Unlock()
	if r.refreshDone != done {
		return
	}
	r.refreshDone = nil
	close(done)
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
	retryAt := time.Time{}
	if r.exhausted[resource] {
		retryAt = r.snapshots[resource].ResetAt
	}
	if secondary := r.secondary[resource]; secondary.RetryAt.After(retryAt) {
		retryAt = secondary.RetryAt
	}
	d := time.Until(retryAt)
	if d < 0 {
		return 0
	}
	return d
}

// BackgroundWaitDuration adds the interactive quota reserve to provider
// retry windows. Exactly ten percent of a known primary budget is retained.
func (r *RateTracker) BackgroundWaitDuration(resource Resource) time.Duration {
	r.mu.RLock()
	snap, known := r.snapshots[resource]
	r.mu.RUnlock()
	if known && interactiveReserveReached(snap) {
		if wait := time.Until(snap.ResetAt); wait > 0 {
			return wait
		}
	}
	return r.WaitDuration(resource)
}

// ObserveSecondary records a provider refusal independently from primary
// resource snapshots.
func (r *RateTracker) ObserveSecondary(resource Resource, retryAt time.Time, source RetrySource, reason string) {
	if resource == "" || retryAt.IsZero() {
		return
	}
	now := time.Now().UTC()
	r.mu.Lock()
	previous := r.secondary[resource]
	r.secondary[resource] = SecondaryRateLimitState{
		Resource: resource, Active: retryAt.After(now), RetryAt: retryAt,
		ObservedAt: now, RetrySource: source, Reason: reason,
	}
	r.signalChangedLocked()
	r.mu.Unlock()
	if !previous.RetryAt.After(now) && r.log != nil {
		r.log.Info("github secondary rate limit observed",
			zap.String("resource", string(resource)),
			zap.Time("retry_at", retryAt),
			zap.String("retry_source", string(source)))
	}
}

// Secondary returns the current observed secondary state for a resource.
func (r *RateTracker) Secondary(resource Resource) SecondaryRateLimitState {
	r.mu.RLock()
	state := r.secondary[resource]
	r.mu.RUnlock()
	state.Active = state.RetryAt.After(time.Now())
	return state
}

// ObserveSuccess clears a secondary estimate early after GitHub accepts a
// real request. A primary snapshot remains unchanged.
func (r *RateTracker) ObserveSuccess(resource Resource) {
	now := time.Now().UTC()
	r.mu.Lock()
	previous, observed := r.secondary[resource]
	if !observed {
		r.mu.Unlock()
		return
	}
	delete(r.secondary, resource)
	r.signalChangedLocked()
	r.mu.Unlock()
	early := previous.RetryAt.After(now)
	incGitHubSecondaryRecovery(resource, previous.RetrySource, early)
	if r.log != nil {
		r.log.Info("github secondary rate limit cleared by accepted response",
			zap.String("resource", string(resource)),
			zap.String("retry_source", string(previous.RetrySource)),
			zap.Bool("early", early))
	}
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
	return parseRateHeadersAt(resp, defaultResource, time.Now().UTC())
}

func parseRateHeadersAt(resp *http.Response, defaultResource Resource, now time.Time) (RateSnapshot, bool) {
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
		Resource:          resource,
		Limit:             limit,
		Remaining:         remaining,
		RemainingObserved: remainingStr != "",
		ParsedFromHeaders: true,
		ResetAt:           time.Unix(reset, 0).UTC(),
		UpdatedAt:         now,
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
		Resource:          resource,
		Limit:             prev.Limit,
		Remaining:         0,
		RemainingObserved: true,
		ResetAt:           resetAt,
		UpdatedAt:         time.Now().UTC(),
	})
}
