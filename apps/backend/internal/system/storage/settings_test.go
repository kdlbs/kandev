package storage

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
)

func TestSettingsStoreMissingUsesDisabledDefaults(t *testing.T) {
	store, _ := newTestStores(t)

	got, err := store.GetSettings(context.Background())
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	want := StorageMaintenanceSettings{
		Enabled:                  false,
		CheckIntervalHours:       24,
		IdleForMinutes:           10,
		OrphanGraceHours:         168,
		QuarantineRetentionHours: 168,
		Workspaces:               ResourceSettings{Enabled: true},
		KandevContainers:         ResourceSettings{Enabled: true},
		GoCache: GoCacheSettings{
			Enabled:     false,
			MaxBytes:    16106127360,
			AdoptedPath: "",
		},
		Docker: DockerSettings{
			DedicatedDaemonAcknowledged: false,
			BuildCacheEnabled:           false,
			BuildCacheKeepBytes:         10737418240,
			BuildCacheUnusedHours:       168,
			UnusedImagesEnabled:         false,
			UnusedImagesHours:           168,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("settings = %#v, want %#v", got, want)
	}
}

func TestSettingsStoreReadFillsMissingFieldsAndIgnoresUnknownFields(t *testing.T) {
	store, rawStore := newTestStores(t)
	if err := rawStore.Save(context.Background(), settingsKey, []byte(`{"enabled":true,"unknown":"ignored"}`)); err != nil {
		t.Fatalf("seed settings: %v", err)
	}

	got, err := store.GetSettings(context.Background())
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if !got.Enabled || got.CheckIntervalHours != 24 || !got.Workspaces.Enabled || !got.KandevContainers.Enabled {
		t.Fatalf("missing fields were not defaulted: %#v", got)
	}
}

func TestSettingsStoreInvalidSavePreservesPreviousValue(t *testing.T) {
	store, rawStore := newTestStores(t)
	ctx := context.Background()
	want := DefaultSettings()
	want.Enabled = true
	if _, err := store.SaveSettings(ctx, want); err != nil {
		t.Fatalf("save valid settings: %v", err)
	}

	invalid := want
	invalid.CheckIntervalHours = 169
	if _, err := store.SaveSettings(ctx, invalid); !errors.Is(err, ErrValidation) {
		t.Fatalf("invalid save error = %v, want ErrValidation", err)
	}

	raw, found, err := rawStore.Get(ctx, settingsKey)
	if err != nil || !found {
		t.Fatalf("read raw settings = (%q, %v, %v)", raw, found, err)
	}
	got, err := store.GetSettings(ctx)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("settings after invalid save = %#v, want %#v", got, want)
	}
}

func TestNormalizeSettingsValidatesRangesAndDockerAcknowledgement(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*StorageMaintenanceSettings)
	}{
		{name: "check interval too low", mutate: func(s *StorageMaintenanceSettings) { s.CheckIntervalHours = 0 }},
		{name: "idle too high", mutate: func(s *StorageMaintenanceSettings) { s.IdleForMinutes = 1441 }},
		{name: "orphan grace too low", mutate: func(s *StorageMaintenanceSettings) { s.OrphanGraceHours = 23 }},
		{name: "quarantine retention too high", mutate: func(s *StorageMaintenanceSettings) { s.QuarantineRetentionHours = 2161 }},
		{name: "go cache too small", mutate: func(s *StorageMaintenanceSettings) { s.GoCache.MaxBytes = 1073741823 }},
		{name: "build cache too small", mutate: func(s *StorageMaintenanceSettings) { s.Docker.BuildCacheKeepBytes = 1073741823 }},
		{name: "build cache age too low", mutate: func(s *StorageMaintenanceSettings) { s.Docker.BuildCacheUnusedHours = 23 }},
		{name: "image age too low", mutate: func(s *StorageMaintenanceSettings) { s.Docker.UnusedImagesHours = 23 }},
		{name: "build cache without dedicated daemon", mutate: func(s *StorageMaintenanceSettings) { s.Docker.BuildCacheEnabled = true }},
		{name: "images without dedicated daemon", mutate: func(s *StorageMaintenanceSettings) { s.Docker.UnusedImagesEnabled = true }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := DefaultSettings()
			tt.mutate(&settings)
			if _, err := NormalizeSettings(settings); !errors.Is(err, ErrValidation) {
				t.Fatalf("NormalizeSettings error = %v, want ErrValidation", err)
			}
		})
	}
}

func TestNormalizeSettingsAllowsAcknowledgedDockerCleanup(t *testing.T) {
	settings := DefaultSettings()
	settings.Docker.DedicatedDaemonAcknowledged = true
	settings.Docker.BuildCacheEnabled = true
	settings.Docker.UnusedImagesEnabled = true

	if _, err := NormalizeSettings(settings); err != nil {
		t.Fatalf("NormalizeSettings: %v", err)
	}
}

func TestSettingsStoreRejectsRelativeAdoptedPath(t *testing.T) {
	settings := DefaultSettings()
	settings.GoCache.AdoptedPath = "relative/go-build"
	if _, err := NormalizeSettings(settings); !errors.Is(err, ErrValidation) {
		t.Fatalf("NormalizeSettings error = %v, want ErrValidation", err)
	}
}

func TestSettingsStoreRequiresDedicatedAdoptionPath(t *testing.T) {
	store, _ := newTestStores(t)
	ctx := context.Background()
	settings := DefaultSettings()
	settings.GoCache.AdoptedPath = "/tmp/go-build"

	if _, err := store.SaveSettings(ctx, settings); !errors.Is(err, ErrAdoptionRequired) {
		t.Fatalf("SaveSettings error = %v, want ErrAdoptionRequired", err)
	}
	got, err := store.GetSettings(ctx)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if got.GoCache.AdoptedPath != "" {
		t.Fatalf("adopted path after rejected save = %q", got.GoCache.AdoptedPath)
	}

	got, err = store.AdoptGoCachePath(ctx, "/tmp/go-build")
	if err != nil {
		t.Fatalf("AdoptGoCachePath: %v", err)
	}
	if got.GoCache.AdoptedPath != "/tmp/go-build" {
		t.Fatalf("adopted path = %q", got.GoCache.AdoptedPath)
	}
}

func TestSettingsStoreRequiresDedicatedDockerConfirmation(t *testing.T) {
	store, _ := newTestStores(t)
	ctx := context.Background()
	settings := DefaultSettings()
	settings.Docker.DedicatedDaemonAcknowledged = true

	if _, err := store.SaveSettings(ctx, settings); !errors.Is(err, ErrDedicatedDockerConfirmation) {
		t.Fatalf("SaveSettings error = %v, want ErrDedicatedDockerConfirmation", err)
	}
	if _, err := store.SaveSettingsWithConfirmations(ctx, settings, SaveConfirmations{DedicatedDocker: true}); err != nil {
		t.Fatalf("SaveSettingsWithConfirmations: %v", err)
	}
}

func TestSettingsStoreInvalidPersistedValuesFailSafeToDisabledDefaults(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "corrupt JSON", raw: `{"enabled":true`},
		{name: "overflowing Docker age", raw: `{
			"enabled":true,
			"docker":{
				"dedicated_daemon_acknowledged":true,
				"build_cache_enabled":true,
				"build_cache_unused_hours":` + fmt.Sprint(math.MaxInt) + `
			}
		}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, rawStore := newTestStores(t)
			ctx := context.Background()
			if err := rawStore.Save(ctx, settingsKey, []byte(tt.raw)); err != nil {
				t.Fatalf("seed settings: %v", err)
			}

			got, err := store.GetSettings(ctx)
			if !errors.Is(err, ErrInvalidPersistedSettings) {
				t.Fatalf("GetSettings error = %v, want ErrInvalidPersistedSettings", err)
			}
			if !reflect.DeepEqual(got, DefaultSettings()) {
				t.Fatalf("settings = %#v, want exact disabled defaults %#v", got, DefaultSettings())
			}
		})
	}
}

func TestNormalizeSettingsRejectsOverflowingDockerAges(t *testing.T) {
	settings := DefaultSettings()
	settings.Docker.BuildCacheUnusedHours = math.MaxInt
	if _, err := NormalizeSettings(settings); !errors.Is(err, ErrValidation) {
		t.Fatalf("build cache age error = %v, want ErrValidation", err)
	}

	settings = DefaultSettings()
	settings.Docker.UnusedImagesHours = math.MaxInt
	if _, err := NormalizeSettings(settings); !errors.Is(err, ErrValidation) {
		t.Fatalf("image age error = %v, want ErrValidation", err)
	}
}

func newTestStores(t *testing.T) (*SettingsStore, *systemsettings.Store) {
	t.Helper()
	conn := newSQLite(t)
	rawStore, err := systemsettings.NewStore(db.NewPool(conn, conn))
	if err != nil {
		t.Fatalf("new settings store: %v", err)
	}
	return NewSettingsStore(rawStore), rawStore
}

func newSQLite(t *testing.T) *sqlx.DB {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}
