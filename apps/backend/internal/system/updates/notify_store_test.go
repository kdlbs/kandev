package updates

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
)

func newTestNotifyStore(t *testing.T) *NotifyStore {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	settingsStore, err := systemsettings.NewStore(db.NewPool(conn, conn))
	if err != nil {
		t.Fatalf("new settings store: %v", err)
	}
	return NewNotifyStore(settingsStore)
}

func TestNotifyStore_GetSettings_DefaultsWhenUnset(t *testing.T) {
	store := newTestNotifyStore(t)
	got, err := store.GetSettings(context.Background())
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if got != DefaultNotifySettings() {
		t.Errorf("got %+v, want default %+v", got, DefaultNotifySettings())
	}
}

func TestNotifyStore_SaveAndGet_Roundtrip(t *testing.T) {
	store := newTestNotifyStore(t)
	ctx := context.Background()

	saved, err := store.SaveSettings(ctx, NotifySettings{Enabled: false, Channel: NotifyChannelDesktop})
	if err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	if saved.Enabled || saved.Channel != NotifyChannelDesktop {
		t.Errorf("saved = %+v", saved)
	}

	got, err := store.GetSettings(ctx)
	if err != nil {
		t.Fatalf("GetSettings after save: %v", err)
	}
	if got != saved {
		t.Errorf("got %+v after save, want %+v", got, saved)
	}
}

func TestNotifyStore_SaveSettings_RejectsInvalidChannel(t *testing.T) {
	store := newTestNotifyStore(t)
	_, err := store.SaveSettings(context.Background(), NotifySettings{Enabled: true, Channel: "smoke-signal"})
	if err == nil {
		t.Fatal("expected error for invalid channel")
	}
}
