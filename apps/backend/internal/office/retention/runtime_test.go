package retention

import (
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
)

func newTestRuntime(t *testing.T) *Runtime {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := officesqlite.NewWithDB(conn, conn, nil); err != nil {
		t.Fatalf("init office schema: %v", err)
	}
	pool := db.NewPool(conn, conn)
	settingsStore, err := systemsettings.NewStore(pool)
	if err != nil {
		t.Fatalf("init settings schema: %v", err)
	}
	return NewRuntime(pool, settingsStore, nil)
}

func TestNewRuntime_WiresEveryComponentNonNil(t *testing.T) {
	runtime := newTestRuntime(t)
	if runtime.Store == nil || runtime.SettingsStore == nil || runtime.PreviewMarker == nil ||
		runtime.Sweeper == nil || runtime.Scheduler == nil || runtime.Checker == nil || runtime.Handler == nil {
		t.Fatalf("Runtime has a nil component: %+v", runtime)
	}
}

func TestRuntime_StartStopIsClean(t *testing.T) {
	runtime := newTestRuntime(t)
	ctx := t.Context()

	if err := runtime.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer runtime.Stop()

	if err := runtime.Start(ctx); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	runtime.Stop()
	runtime.Stop() // idempotent
}

func TestRuntime_HandlerOnSettingsChangedReachesScheduler(t *testing.T) {
	runtime := newTestRuntime(t)
	ctx := t.Context()
	if err := runtime.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer runtime.Stop()

	settings := DefaultSettings()
	settings.SweepIntervalHours = 2
	saved, err := runtime.SettingsStore.SaveSettings(ctx, settings)
	if err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	// Handler's OnSettingsChanged is wired to Scheduler.ApplySettings; call
	// it exactly as putRetention would and confirm the running scheduler
	// picked up the change (AC-004.5, without a restart).
	runtime.Handler.config.OnSettingsChanged(saved)
	if got := runtime.Scheduler.latestSettings(); got.SweepIntervalHours != 2 {
		t.Fatalf("Scheduler.latestSettings().SweepIntervalHours = %d, want 2", got.SweepIntervalHours)
	}
}
