package plugins

import (
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
)

func newTestSettingsStore(t *testing.T) *settingsStore {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	s, err := newSettingsStore(db.NewPool(conn, conn))
	if err != nil {
		t.Fatalf("new settings store: %v", err)
	}
	return s
}

func TestSettingsStoreDefaultsToAutoUpdateOff(t *testing.T) {
	s := newTestSettingsStore(t)
	got, err := s.Get()
	if err != nil {
		t.Fatalf("Get(): %v", err)
	}
	if got.AutoUpdateDefault {
		t.Fatal("fresh settings store should default AutoUpdateDefault to false (opt-in)")
	}
}

func TestSettingsStoreSetAndGetRoundTrips(t *testing.T) {
	s := newTestSettingsStore(t)
	if err := s.SetAutoUpdateDefault(true); err != nil {
		t.Fatalf("SetAutoUpdateDefault(true): %v", err)
	}
	got, err := s.Get()
	if err != nil {
		t.Fatalf("Get(): %v", err)
	}
	if !got.AutoUpdateDefault {
		t.Fatal("AutoUpdateDefault = false after SetAutoUpdateDefault(true)")
	}

	// Toggling back off overwrites the single row rather than inserting a
	// second one (the id=1 CHECK + ON CONFLICT upsert).
	if err := s.SetAutoUpdateDefault(false); err != nil {
		t.Fatalf("SetAutoUpdateDefault(false): %v", err)
	}
	got, err = s.Get()
	if err != nil {
		t.Fatalf("Get(): %v", err)
	}
	if got.AutoUpdateDefault {
		t.Fatal("AutoUpdateDefault = true after SetAutoUpdateDefault(false)")
	}
}

func TestSettingsStoreSchemaInitIsReplayable(t *testing.T) {
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	pool := db.NewPool(conn, conn)

	first, err := newSettingsStore(pool)
	if err != nil {
		t.Fatalf("first newSettingsStore(): %v", err)
	}
	if err := first.SetAutoUpdateDefault(true); err != nil {
		t.Fatalf("SetAutoUpdateDefault(): %v", err)
	}
	// A second construction on the same DB (e.g. a restart) must not error on
	// the CREATE TABLE and must see the previously persisted value.
	second, err := newSettingsStore(pool)
	if err != nil {
		t.Fatalf("second newSettingsStore(): %v", err)
	}
	got, err := second.Get()
	if err != nil {
		t.Fatalf("Get(): %v", err)
	}
	if !got.AutoUpdateDefault {
		t.Fatal("persisted AutoUpdateDefault did not survive a second store construction")
	}
}
