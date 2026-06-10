package orchestrator

import (
	"context"
	"expvar"
	"strings"

	"go.uber.org/zap"
)

// WatcherTaskCounter exposes the count of open watcher-created tasks for a
// single (integration, watchID) pair. Implemented by the task repository.
// Kept as a narrow interface so the orchestrator's throttle gate can be
// tested without spinning up SQLite.
type WatcherTaskCounter interface {
	CountOpenWatcherCreatedTasks(ctx context.Context, integration, watchID string) (int, error)
}

// SetWatcherTaskCounter wires the open-task counter used by the watcher
// throttle gate. Safe to call once during boot. When unset, the gate falls
// open (no throttling) — the orchestrator must remain usable in tests and
// during early init before the task repository is plumbed through.
func (s *Service) SetWatcherTaskCounter(c WatcherTaskCounter) {
	s.watcherTaskCount = c
}

// Watcher throttle metrics, exposed via stdlib's /debug/vars handler. Counter
// labels match the office routing metrics format ("k=v;k=v") so a downstream
// Prometheus translator can read both with the same parser. Saturation is
// recorded as a counter of state-transition events (cap_reached /
// cap_cleared); a live "currently saturated" gauge would drift across
// restarts so we stick with deltas.
var (
	watcherDispatchAttemptedTotal = expvar.NewMap("watcher_dispatch_attempted_total")
	watcherDispatchDeferredTotal  = expvar.NewMap("watcher_dispatch_deferred_total")
	watcherCapTransitionsTotal    = expvar.NewMap("watcher_cap_transitions_total")
)

// watcherMetricLabel builds a "k1=v1;k2=v2" label string for an expvar map
// key. Kept package-local so it doesn't clash with the office package's
// metricLabel helper.
func watcherMetricLabel(pairs ...string) string {
	if len(pairs)%2 != 0 {
		return ""
	}
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		parts = append(parts, pairs[i]+"="+pairs[i+1])
	}
	return strings.Join(parts, ";")
}

// watcherSlotKey is the (integration, watchID) compound key used to scope
// the pending counter and the saturation state map. Linear and Jira watches
// can share id strings; the integration prefix keeps them separate.
func watcherSlotKey(integration, watchID string) string {
	return integration + "|" + watchID
}

// acquireWatcherSlot is the synchronous slot acquisition that gates the
// `go coordinator.Dispatch(...)` spawn. It is the front-pressure valve for
// the Linear, Jira, and Sentry watchers: with a per-watch cap, an unexpectedly
// broad filter cannot fan out into dozens of concurrent tasks on a single poll
// tick.
//
// Returns (release, ok). When ok is true the caller MUST invoke release()
// exactly once (typically via `defer release()` in the dispatch goroutine)
// to drop the in-process pending count. When ok is false the caller MUST
// drop the event — no goroutine, no dedup row — so the next poll tick can
// retry.
//
// Bypass paths (all return release=no-op, ok=true):
//
//   - cap is nil (uncapped watch)
//   - cap is <= 0 (defensive: API rejects these, but a stale row should not
//     freeze the gate)
//   - watchID is empty (event has no watch — treat as unthrottled)
//   - watcherTaskCount is unset (early boot / tests)
//   - the count query errors (fail-open: a transient DB blip must not
//     silently stall the watcher; one-event overshoot is acceptable)
//
// Throttling path: when count(db) + pending(in-process) >= cap, the gate
// defers. Pending is incremented atomically with the read under watcherMu
// so a burst of events arriving in the same poll tick cannot collectively
// read the same stale DB count and all pass.
func (s *Service) acquireWatcherSlot(ctx context.Context, integration, watchID string, maxInflight *int) (func(), bool) {
	watcherDispatchAttemptedTotal.Add(watcherMetricLabel("integration", integration), 1)
	noop := func() {}

	if maxInflight == nil || *maxInflight <= 0 || watchID == "" {
		return noop, true
	}
	if s.watcherTaskCount == nil {
		return noop, true
	}

	count, err := s.watcherTaskCount.CountOpenWatcherCreatedTasks(ctx, integration, watchID)
	if err != nil {
		s.logger.Warn("watcher throttle: count failed, failing open",
			zap.String("integration", integration),
			zap.String("watch_id", watchID),
			zap.Error(err))
		return noop, true
	}

	key := watcherSlotKey(integration, watchID)
	s.watcherMu.Lock()
	defer s.watcherMu.Unlock()
	if s.pendingByWatch == nil {
		s.pendingByWatch = make(map[string]int)
	}
	if s.watcherSaturated == nil {
		s.watcherSaturated = make(map[string]bool)
	}
	pending := s.pendingByWatch[key]
	if count+pending >= *maxInflight {
		s.recordSaturatedLocked(integration, watchID, key, count, pending, *maxInflight)
		return noop, false
	}
	s.recordClearedLocked(integration, watchID, key)
	s.pendingByWatch[key] = pending + 1
	return s.releaseFunc(key), true
}

func (s *Service) releaseFunc(key string) func() {
	return func() {
		s.watcherMu.Lock()
		defer s.watcherMu.Unlock()
		if s.pendingByWatch == nil {
			return
		}
		if v := s.pendingByWatch[key]; v <= 1 {
			delete(s.pendingByWatch, key)
		} else {
			s.pendingByWatch[key] = v - 1
		}
	}
}

// recordSaturatedLocked logs a one-shot Warn the first time the gate sees
// (count + pending) >= cap for this watch, and bumps the deferred counter.
// Subsequent deferrals during the same saturation window are Debug-only.
// Must be called with s.watcherMu held.
func (s *Service) recordSaturatedLocked(integration, watchID, key string, count, pending, maxInflight int) {
	watcherDispatchDeferredTotal.Add(watcherMetricLabel("integration", integration), 1)
	if s.watcherSaturated[key] {
		s.logger.Debug("watcher throttle: deferring event (cap held)",
			zap.String("integration", integration),
			zap.String("watch_id", watchID),
			zap.Int("count", count),
			zap.Int("pending", pending),
			zap.Int("cap", maxInflight))
		return
	}
	s.watcherSaturated[key] = true
	watcherCapTransitionsTotal.Add(
		watcherMetricLabel("integration", integration, "transition", "reached"), 1)
	s.logger.Warn("watcher throttle: cap reached, deferring further events until tasks drain",
		zap.String("integration", integration),
		zap.String("watch_id", watchID),
		zap.Int("count", count),
		zap.Int("pending", pending),
		zap.Int("cap", maxInflight))
}

// recordClearedLocked emits a single Warn when the gate transitions back
// from saturated → not-saturated for this watch. Cheap when not saturated:
// just a map lookup. Must be called with s.watcherMu held.
func (s *Service) recordClearedLocked(integration, watchID, key string) {
	if !s.watcherSaturated[key] {
		return
	}
	delete(s.watcherSaturated, key)
	watcherCapTransitionsTotal.Add(
		watcherMetricLabel("integration", integration, "transition", "cleared"), 1)
	s.logger.Warn("watcher throttle: cap cleared, resuming dispatch",
		zap.String("integration", integration),
		zap.String("watch_id", watchID))
}
