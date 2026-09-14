package requiredstores

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
)

const (
	defaultProbeInterval = 15 * time.Second
	probeTimeout         = 2 * time.Second
)

// Health probes the shared database and every table declared by the catalog.
// It owns the periodic probe lifecycle but delegates state storage to Tracker.
type Health struct {
	tracker *Tracker
	pool    *db.Pool
	log     *logger.Logger

	mu       sync.Mutex
	interval time.Duration
	cancel   context.CancelFunc
	done     chan struct{}
}

// NewHealth creates a runtime health probe for a completed store tracker.
func NewHealth(tracker *Tracker, pool *db.Pool, log *logger.Logger) *Health {
	return &Health{tracker: tracker, pool: pool, log: log, interval: defaultProbeInterval}
}

// SetInterval changes the periodic interval. It is intended for tests and
// controlled embedders; callers must set it before Start.
func (h *Health) SetInterval(interval time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if interval > 0 {
		h.interval = interval
	}
}

// Check runs one synchronous probe. It returns an aggregate error when one or
// more stores are unavailable, while recording an independent result for each
// catalog entry.
func (h *Health) Check(ctx context.Context) error {
	if h == nil || h.tracker == nil {
		return errors.New("required-store health tracker is unavailable")
	}
	if h.pool == nil || h.pool.Writer() == nil || h.pool.Reader() == nil {
		return h.recordUnavailable(errors.New("database pool is unavailable"))
	}
	probeErr := h.ping(ctx)
	results := make([]error, len(h.tracker.catalog))
	for index, descriptor := range h.tracker.catalog {
		results[index] = probeErr
		if probeErr == nil {
			results[index] = h.probeTables(ctx, descriptor)
		}
	}
	var failures []error
	for index, result := range results {
		if err := h.tracker.RecordProbe(h.tracker.catalog[index].ID, result); err != nil {
			failures = append(failures, err)
			continue
		}
		if result != nil {
			failures = append(failures, fmt.Errorf("%s: %w", h.tracker.catalog[index].ID, result))
		}
	}
	if len(failures) == 0 {
		h.logTransition()
		return nil
	}
	h.logTransition()
	return errors.Join(failures...)
}

func (h *Health) ping(ctx context.Context) error {
	if err := h.pool.Writer().PingContext(ctx); err != nil {
		return fmt.Errorf("writer ping failed: %w", err)
	}
	if err := h.pool.Reader().PingContext(ctx); err != nil {
		return fmt.Errorf("reader ping failed: %w", err)
	}
	return nil
}

func (h *Health) probeTables(ctx context.Context, descriptor Descriptor) error {
	for _, table := range descriptor.RequiredTables {
		exists, err := db.TableExistsContext(ctx, h.pool.Writer(), table)
		if err != nil {
			return fmt.Errorf("table probe failed: %w", err)
		}
		if !exists {
			return fmt.Errorf("required table %q is missing", table)
		}
	}
	return nil
}

func (h *Health) recordUnavailable(err error) error {
	for _, descriptor := range h.tracker.catalog {
		if recordErr := h.tracker.RecordProbe(descriptor.ID, err); recordErr != nil {
			return recordErr
		}
	}
	h.logTransition()
	return err
}

// Start begins the periodic probe loop. The returned cleanup is idempotent.
func (h *Health) Start(ctx context.Context) func() error {
	if h == nil {
		return func() error { return nil }
	}
	h.mu.Lock()
	if h.cancel != nil {
		h.mu.Unlock()
		return h.Stop
	}
	loopCtx, cancel := context.WithCancel(ctx)
	h.cancel = cancel
	h.done = make(chan struct{})
	interval := h.interval
	done := h.done
	h.mu.Unlock()
	go h.run(loopCtx, interval, done)
	return h.Stop
}

func (h *Health) run(ctx context.Context, interval time.Duration, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			checkCtx, cancel := context.WithTimeout(ctx, probeTimeout)
			if err := h.Check(checkCtx); err != nil && h.log != nil {
				h.log.Warn("required persistence probe failed", zap.Error(err))
			}
			cancel()
		}
	}
}

// Stop terminates the periodic probe loop.
func (h *Health) Stop() error {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	cancel := h.cancel
	done := h.done
	h.cancel = nil
	h.done = nil
	h.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	<-done
	return nil
}

// Healthy reports the current aggregate runtime state.
func (h *Health) Healthy() bool {
	return h != nil && h.tracker != nil && h.tracker.Healthy()
}

// State returns the current aggregate state.
func (h *Health) State() State {
	if h == nil || h.tracker == nil {
		return StateUnhealthy
	}
	return h.tracker.AggregateState()
}

// UnhealthyStoreIDs returns the stable list used by readiness and caller
// errors.
func (h *Health) UnhealthyStoreIDs() []string {
	if h == nil || h.tracker == nil {
		return nil
	}
	return h.tracker.UnhealthyStoreIDs()
}

// UnavailableStoreIDs returns all stores that are not currently healthy.
func (h *Health) UnavailableStoreIDs() []string {
	if h == nil || h.tracker == nil {
		return nil
	}
	return h.tracker.UnavailableStoreIDs()
}

func (h *Health) logTransition() {
	if h.log == nil {
		return
	}
	state := h.State()
	h.log.Info("required persistence state updated",
		zap.String("state", string(state)),
		zap.Strings("store_ids", h.UnhealthyStoreIDs()),
		zap.String("error_class", publicErrorClass(state)))
}

// PublicError returns a stable, non-sensitive error for diagnostics.
func PublicError(status Status) string {
	if status.Error == "" {
		return ""
	}
	if strings.Contains(status.Error, "required table") && strings.Contains(status.Error, "missing") {
		return status.Error
	}
	return "database probe failed"
}

func publicErrorClass(state State) string {
	if state == StateUnhealthy {
		return "database_probe"
	}
	return "none"
}
