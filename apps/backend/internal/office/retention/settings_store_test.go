package retention

import (
	"context"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
)

func newTestSettingsStore(t *testing.T) (*SettingsStore, *systemsettings.Store) {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	raw, err := systemsettings.NewStore(db.NewPool(conn, conn))
	if err != nil {
		t.Fatalf("new settings store: %v", err)
	}
	return NewSettingsStore(raw), raw
}

func TestSettingsStore_MissingReturnsDefaults(t *testing.T) {
	store, _ := newTestSettingsStore(t)
	got, err := store.GetSettings(context.Background())
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if got != DefaultSettings() {
		t.Fatalf("GetSettings() = %+v, want defaults", got)
	}
}

func TestSettingsStore_SaveThenGetRoundTrips(t *testing.T) {
	store, _ := newTestSettingsStore(t)
	ctx := context.Background()
	in := DefaultSettings()
	in.SweepIntervalHours = 12
	in.RoutineRuns.WindowDays = 90

	saved, err := store.SaveSettings(ctx, in)
	if err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	if saved != in {
		t.Fatalf("SaveSettings returned %+v, want %+v", saved, in)
	}

	got, err := store.GetSettings(ctx)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if got != in {
		t.Fatalf("GetSettings() = %+v, want %+v", got, in)
	}
}

func TestSettingsStore_SaveRejectsOutOfRangeAndLeavesStoredUnchanged(t *testing.T) {
	store, _ := newTestSettingsStore(t)
	ctx := context.Background()
	in := DefaultSettings()
	in.SweepIntervalHours = 12
	if _, err := store.SaveSettings(ctx, in); err != nil {
		t.Fatalf("seed SaveSettings: %v", err)
	}

	bad := DefaultSettings()
	bad.BatchLimit = 1
	if _, err := store.SaveSettings(ctx, bad); !errors.Is(err, ErrValidation) {
		t.Fatalf("SaveSettings(bad): err = %v, want ErrValidation", err)
	}

	got, err := store.GetSettings(ctx)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if got.SweepIntervalHours != 12 {
		t.Fatalf("stored settings changed after rejected write: %+v", got)
	}
}

// TestSettingsStore_UnparseableFallsBackToDefaultsForReporting proves
// AC-OFFICE-RUN-HISTORY-RETENTION-004.4: reading for reporting/startup
// tolerates an unreadable document and yields the documented defaults
// (wrapped in ErrInvalidPersistedSettings so a caller can raise a health
// issue), never failing outright.
func TestSettingsStore_UnparseableFallsBackToDefaultsForReporting(t *testing.T) {
	store, raw := newTestSettingsStore(t)
	ctx := context.Background()
	if err := raw.Save(ctx, settingsKey, []byte("not json")); err != nil {
		t.Fatalf("seed unparseable settings: %v", err)
	}

	got, err := store.GetSettings(ctx)
	if !errors.Is(err, ErrInvalidPersistedSettings) {
		t.Fatalf("GetSettings: err = %v, want ErrInvalidPersistedSettings", err)
	}
	if got != DefaultSettings() {
		t.Fatalf("GetSettings() = %+v, want defaults on unparseable document", got)
	}
}

// TestSettingsStore_ForSweepFailsClosedOnUnparseable proves the other half
// of AC-OFFICE-RUN-HISTORY-RETENTION-004.5: reading settings *to delete by*
// must not silently fall back to the (possibly shorter) default window. The
// caller sees ErrInvalidPersistedSettings and a zero Settings value, and is
// expected to skip the sweep rather than run it under defaults.
func TestSettingsStore_ForSweepFailsClosedOnUnparseable(t *testing.T) {
	store, raw := newTestSettingsStore(t)
	ctx := context.Background()
	if err := raw.Save(ctx, settingsKey, []byte("not json")); err != nil {
		t.Fatalf("seed unparseable settings: %v", err)
	}

	_, err := store.GetSettingsForSweep(ctx)
	if !errors.Is(err, ErrInvalidPersistedSettings) {
		t.Fatalf("GetSettingsForSweep: err = %v, want ErrInvalidPersistedSettings", err)
	}
}

func TestSettingsStore_ForSweepMissingUsesDefaults(t *testing.T) {
	store, _ := newTestSettingsStore(t)
	got, err := store.GetSettingsForSweep(context.Background())
	if err != nil {
		t.Fatalf("GetSettingsForSweep: %v", err)
	}
	if got != DefaultSettings() {
		t.Fatalf("GetSettingsForSweep() = %+v, want defaults when nothing stored", got)
	}
}

func TestSettingsStore_ForSweepReadsWriterPool(t *testing.T) {
	store, _ := newTestSettingsStore(t)
	ctx := context.Background()
	in := DefaultSettings()
	in.RoutineRuns.WindowDays = 3650
	if _, err := store.SaveSettings(ctx, in); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	got, err := store.GetSettingsForSweep(ctx)
	if err != nil {
		t.Fatalf("GetSettingsForSweep: %v", err)
	}
	if got.RoutineRuns.WindowDays != 3650 {
		t.Fatalf("GetSettingsForSweep() = %+v, want the just-saved window", got)
	}
}
