package sleepinhibition

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
)

func TestStoreDefaultsToDisabledWhenAbsent(t *testing.T) {
	store := newSleepInhibitionStore(t)

	settings, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load missing settings: %v", err)
	}
	if settings != (Settings{}) {
		t.Fatalf("missing settings = %#v, want disabled", settings)
	}
}

func TestStoreRoundTripsSettings(t *testing.T) {
	store := newSleepInhibitionStore(t)
	want := Settings{Enabled: true}

	if err := store.Save(context.Background(), want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != want {
		t.Fatalf("round trip = %#v, want %#v", got, want)
	}
}

func TestStoreMalformedValueReturnsTypedError(t *testing.T) {
	store := newSleepInhibitionStore(t)
	if err := store.raw.Save(context.Background(), SettingsKey, []byte(`{"enabled":"yes"}`)); err != nil {
		t.Fatalf("seed malformed setting: %v", err)
	}

	_, err := store.Load(context.Background())
	if err == nil || !IsInvalidPersisted(err) {
		t.Fatalf("malformed load error = %v, want invalid persisted error", err)
	}
}

func newSleepInhibitionStore(t *testing.T) *Store {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	raw, err := systemsettings.NewStore(db.NewPool(conn, conn))
	if err != nil {
		t.Fatalf("new system settings store: %v", err)
	}
	return NewStore(raw)
}
