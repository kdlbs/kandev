package retention

import (
	"context"

	"github.com/kandev/kandev/internal/db"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
)

// Runtime composes every retention component behind one Start/Stop pair,
// mirroring internal/system/storage.Runtime's shape: a scheduler goroutine,
// an HTTP handler, and a health checker, wired together once at boot over
// the shared pool and the shared key/value settings store.
type Runtime struct {
	Store         *Store
	SettingsStore *SettingsStore
	PreviewMarker *PreviewMarkerStore
	Sweeper       *Sweeper
	Scheduler     *Scheduler
	Checker       *Checker
	Handler       *Handler
}

// NewRuntime constructs every retention component. logError, when set, is
// forwarded to Handler for a failed settings read or write; it never
// affects the sweep, which fails closed on its own terms (see
// SettingsStore.GetSettingsForSweep).
func NewRuntime(pool *db.Pool, settingsStore *systemsettings.Store, logError func(string, error)) *Runtime {
	store := NewStore(pool)
	retentionSettings := NewSettingsStore(settingsStore)
	previewMarker := NewPreviewMarkerStore(settingsStore)
	sweeper := NewSweeper(pool, store, retentionSettings, previewMarker)
	scheduler := NewScheduler(retentionSettings, sweeper, SchedulerOptions{})
	checker := NewChecker(retentionSettings, sweeper, previewMarker)
	handler := NewHandler(HandlerConfig{
		SettingsStore:     retentionSettings,
		Sweeper:           sweeper,
		OnSettingsChanged: scheduler.ApplySettings,
		LogError:          logError,
	})
	return &Runtime{
		Store:         store,
		SettingsStore: retentionSettings,
		PreviewMarker: previewMarker,
		Sweeper:       sweeper,
		Scheduler:     scheduler,
		Checker:       checker,
		Handler:       handler,
	}
}

// Start begins the scheduler loop (census immediately, sweep gated by
// enablement — see scheduler.go). Idempotent: safe to call once at boot.
func (r *Runtime) Start(ctx context.Context) error {
	return r.Scheduler.Start(ctx)
}

// Stop cancels and joins the scheduler loop.
func (r *Runtime) Stop() {
	r.Scheduler.Stop()
}
