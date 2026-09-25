package backendapp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/system/sessioncapacity"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
)

func TestResolveSessionCapacityStartupRestoresSavedSetting(t *testing.T) {
	pool := newMessageQueueSettingsTestPool(t)
	settingsStore, err := systemsettings.NewStore(pool)
	if err != nil {
		t.Fatalf("new system settings store: %v", err)
	}
	if err := sessioncapacity.NewStore(settingsStore).Save(context.Background(), sessioncapacity.Settings{
		Enabled: true, MaxSessions: 7,
	}); err != nil {
		t.Fatalf("save session capacity: %v", err)
	}

	resolution, err := resolveSessionCapacityWithStore(settingsStore, sessioncapacity.Environment{}, testLogger(t))
	if err != nil {
		t.Fatalf("resolve session capacity: %v", err)
	}
	if !resolution.Effective.Enabled || resolution.Effective.MaxSessions != 7 ||
		resolution.Effective.Source != sessioncapacity.SourceSetting {
		t.Fatalf("resolution = %+v, want saved enabled capacity", resolution.Effective)
	}
	if got := effectiveSessionCapacity(resolution); got != 7 {
		t.Fatalf("effective capacity = %d, want 7", got)
	}
}

func TestResolveSessionCapacityStartupDefaultsToDisabled(t *testing.T) {
	resolution, err := resolveSessionCapacityWithStore(nil, sessioncapacity.Environment{}, testLogger(t))
	if err != nil {
		t.Fatalf("resolve default session capacity: %v", err)
	}
	if resolution.Effective.Enabled || resolution.Effective.MaxSessions != 0 ||
		resolution.Effective.Source != sessioncapacity.SourceDefault {
		t.Fatalf("resolution = %+v, want disabled default", resolution.Effective)
	}
}

func TestResolveSessionCapacityStartupSurfacesInvalidPersistedSetting(t *testing.T) {
	pool := newMessageQueueSettingsTestPool(t)
	settingsStore, err := systemsettings.NewStore(pool)
	if err != nil {
		t.Fatalf("new system settings store: %v", err)
	}
	if err := settingsStore.Save(context.Background(), sessioncapacity.SettingsKey, []byte(`{"enabled":true,"max_sessions":0}`)); err != nil {
		t.Fatalf("save invalid session capacity: %v", err)
	}

	if _, err := resolveSessionCapacityWithStore(settingsStore, sessioncapacity.Environment{}, testLogger(t)); err == nil {
		t.Fatal("invalid persisted session capacity was silently ignored")
	}
}
