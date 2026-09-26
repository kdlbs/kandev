package requiredstores

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/system/maintenance"
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

type probeFailureDiagnostic struct {
	stage       string
	elapsed     time.Duration
	errorClass  string
	storeID     string
	writerStats sql.DBStats
	readerStats sql.DBStats
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
	_, err := h.check(ctx)
	return err
}

func (h *Health) check(ctx context.Context) (*probeFailureDiagnostic, error) {
	if h == nil || h.tracker == nil {
		return nil, errors.New("required-store health tracker is unavailable")
	}
	if h.pool == nil || h.pool.Writer() == nil || h.pool.Reader() == nil {
		err := errors.New("database pool is unavailable")
		diagnostic := h.failureDiagnostic("pool", 0, err, "")
		return diagnostic, h.recordUnavailable(err)
	}
	diagnostic, probeErr := h.pingObserved(ctx)
	results := make([]error, len(h.tracker.catalog))
	for index, descriptor := range h.tracker.catalog {
		results[index] = probeErr
		if probeErr == nil {
			tableDiagnostic, tableErr := h.probeTablesObserved(ctx, descriptor)
			results[index] = tableErr
			if diagnostic == nil {
				diagnostic = tableDiagnostic
			}
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
		return nil, nil
	}
	h.logTransition()
	return diagnostic, errors.Join(failures...)
}

// MarkUnavailable records a destructive database transition before the
// maintenance owner releases its admission lease. This keeps stateful
// requests fail-closed while the process waits for the required restart.
func (h *Health) MarkUnavailable() {
	if h == nil || h.tracker == nil {
		return
	}
	_ = h.recordUnavailable(errors.New("database maintenance requires restart"))
}

// checkRuntime runs a periodic probe when the database can be admitted. SQLite
// maintenance owns the same writer pool, so a busy maintenance lease defers
// the probe instead of turning bounded writer contention into an unhealthy
// state. Startup callers continue to use Check, which remains strict.
func (h *Health) checkRuntime(ctx context.Context) (deferred bool, err error) {
	deferred, _, err = h.checkRuntimeDetailed(ctx)
	return deferred, err
}

func (h *Health) checkRuntimeDetailed(
	ctx context.Context,
) (deferred bool, diagnostic *probeFailureDiagnostic, err error) {
	if h.isSQLite() {
		release, ok := maintenance.ForPool(h.pool).TryAcquire()
		if !ok {
			if h.log != nil {
				h.log.Debug("required persistence probe deferred during database maintenance")
			}
			return true, nil, nil
		}
		defer release()
	}
	diagnostic, err = h.check(ctx)
	return false, diagnostic, err
}

func (h *Health) isSQLite() bool {
	return h.pool != nil && h.pool.Writer() != nil && h.pool.Writer().DriverName() == dialect.SQLite3
}

func (h *Health) pingObserved(ctx context.Context) (*probeFailureDiagnostic, error) {
	started := time.Now()
	if err := h.pool.Writer().PingContext(ctx); err != nil {
		return h.failureDiagnostic("writer_ping", time.Since(started), err, ""), fmt.Errorf("writer ping failed: %w", err)
	}
	started = time.Now()
	if err := h.pool.Reader().PingContext(ctx); err != nil {
		return h.failureDiagnostic("reader_ping", time.Since(started), err, ""), fmt.Errorf("reader ping failed: %w", err)
	}
	return nil, nil
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

func (h *Health) probeTablesObserved(
	ctx context.Context,
	descriptor Descriptor,
) (*probeFailureDiagnostic, error) {
	started := time.Now()
	err := h.probeTables(ctx, descriptor)
	if err == nil {
		return nil, nil
	}
	return h.failureDiagnostic("table_probe", time.Since(started), err, descriptor.ID), err
}

func (h *Health) failureDiagnostic(stage string, elapsed time.Duration, err error, storeID string) *probeFailureDiagnostic {
	diagnostic := &probeFailureDiagnostic{
		stage:      stage,
		elapsed:    elapsed,
		errorClass: classifyProbeError(err),
		storeID:    storeID,
	}
	if h.pool == nil {
		return diagnostic
	}
	if writer := h.pool.Writer(); writer != nil {
		diagnostic.writerStats = writer.Stats()
	}
	if reader := h.pool.Reader(); reader != nil {
		diagnostic.readerStats = reader.Stats()
	}
	return diagnostic
}

func classifyProbeError(err error) string {
	message := ""
	if err != nil {
		message = strings.ToLower(err.Error())
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case strings.Contains(message, "required table") && strings.Contains(message, "missing"):
		return "required_table_missing"
	default:
		return "probe_error"
	}
}

func (d *probeFailureDiagnostic) logFields() []zap.Field {
	if d == nil {
		return []zap.Field{zap.String("stage", "unknown"), zap.String("error_class", "probe_error")}
	}
	fields := []zap.Field{
		zap.String("stage", d.stage),
		zap.Float64("elapsed_ms", float64(d.elapsed)/float64(time.Millisecond)),
		zap.String("error_class", d.errorClass),
		zap.Int("writer_open_connections", d.writerStats.OpenConnections),
		zap.Int("writer_in_use", d.writerStats.InUse),
		zap.Int64("writer_wait_count", d.writerStats.WaitCount),
		zap.Float64("writer_wait_duration_ms", float64(d.writerStats.WaitDuration)/float64(time.Millisecond)),
		zap.Int("reader_open_connections", d.readerStats.OpenConnections),
		zap.Int("reader_in_use", d.readerStats.InUse),
		zap.Int64("reader_wait_count", d.readerStats.WaitCount),
		zap.Float64("reader_wait_duration_ms", float64(d.readerStats.WaitDuration)/float64(time.Millisecond)),
	}
	if d.storeID != "" {
		fields = append(fields, zap.String("store_id", d.storeID))
	}
	return fields
}

func (h *Health) logProbeFailure(diagnostic *probeFailureDiagnostic) {
	if h == nil || h.log == nil {
		return
	}
	h.log.Warn("required persistence probe failed", diagnostic.logFields()...)
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
			if ctx.Err() != nil {
				return
			}
			checkCtx, cancel := context.WithTimeout(ctx, probeTimeout)
			_, diagnostic, err := h.checkRuntimeDetailed(checkCtx)
			if err != nil {
				h.logProbeFailure(diagnostic)
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
