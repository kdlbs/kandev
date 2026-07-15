package gocache

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/system/storage"
)

func TestAnalysisJSONUsesStorageAPISnakeCase(t *testing.T) {
	encoded, err := json.Marshal(Analysis{Path: "/cache", SizeBytes: 42, Owned: true, Enabled: false})
	if err != nil {
		t.Fatalf("Marshal Analysis: %v", err)
	}
	want := `{"path":"/cache","size_bytes":42,"owned":true,"enabled":false}`
	if string(encoded) != want {
		t.Fatalf("Analysis JSON = %s, want %s", encoded, want)
	}
}

func TestCleanupResultJSONUsesStorageAPISnakeCase(t *testing.T) {
	encoded, err := json.Marshal(CleanupResult{
		Path: "/cache", BytesBefore: 100, BytesAfter: 20, ReclaimedBytes: 80,
	})
	if err != nil {
		t.Fatalf("Marshal CleanupResult: %v", err)
	}
	want := `{"path":"/cache","bytes_before":100,"bytes_after":20,"reclaimed_bytes":80,"quarantine_entry":null}`
	if string(encoded) != want {
		t.Fatalf("CleanupResult JSON = %s, want %s", encoded, want)
	}
}

type recordingStore struct {
	created    *storage.QuarantineEntry
	createErr  error
	transition storage.QuarantineState
}

func (s *recordingStore) CreateQuarantineEntry(_ context.Context, entry *storage.QuarantineEntry) error {
	if s.createErr != nil {
		return s.createErr
	}
	copy := *entry
	s.created = &copy
	return nil
}

func (s *recordingStore) TransitionQuarantineEntry(
	_ context.Context,
	_ string,
	next storage.QuarantineState,
	_ string,
) (storage.QuarantineEntry, error) {
	s.transition = next
	return storage.QuarantineEntry{}, nil
}

type staticSettings struct {
	settings storage.StorageMaintenanceSettings
}

func (s staticSettings) GetSettings(context.Context) (storage.StorageMaintenanceSettings, error) {
	return s.settings, nil
}

func TestExecutionEnvironmentCreatesOwnedManagedCache(t *testing.T) {
	home := t.TempDir()
	settings := storage.DefaultSettings()
	settings.GoCache.Enabled = true
	settings.GoCache.MaxBytes = 1
	provider := New(Config{
		HomeDir:  home,
		TrashDir: filepath.Join(home, "trash"),
		Settings: staticSettings{settings: settings},
	})

	env, err := provider.ExecutionEnvironment(context.Background())
	if err != nil {
		t.Fatalf("ExecutionEnvironment() error = %v", err)
	}
	want := filepath.Join(home, "cache", "go-build")
	if got := env["GOCACHE"]; got != want {
		t.Fatalf("GOCACHE = %q, want %q", got, want)
	}
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Fatalf("managed cache directory was not created: info=%v err=%v", info, err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(want), ".go-build.kandev-owned")); err != nil {
		t.Fatalf("ownership marker was not created: %v", err)
	}
}

func TestCleanupRotatesOwnedCacheAboveThreshold(t *testing.T) {
	home := t.TempDir()
	settings := storage.DefaultSettings()
	settings.GoCache.Enabled = true
	settings.GoCache.MaxBytes = 1
	store := &recordingStore{}
	provider := New(Config{
		HomeDir:  home,
		TrashDir: filepath.Join(home, "trash"),
		Settings: staticSettings{settings: settings},
		Store:    store,
	})
	env, err := provider.ExecutionEnvironment(context.Background())
	if err != nil {
		t.Fatalf("ExecutionEnvironment() error = %v", err)
	}
	cachePath := env["GOCACHE"]
	if err := os.WriteFile(filepath.Join(cachePath, "artifact"), []byte("1234"), 0o600); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	result, err := provider.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if result.ReclaimedBytes != 4 {
		t.Fatalf("ReclaimedBytes = %d, want 4", result.ReclaimedBytes)
	}
	if _, err := os.Stat(filepath.Join(cachePath, "artifact")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old cache artifact still exists: %v", err)
	}
	if info, err := os.Stat(cachePath); err != nil || !info.IsDir() || info.Mode().Perm()&0o200 == 0 {
		t.Fatalf("replacement cache is not writable: info=%v err=%v", info, err)
	}
	if store.created == nil || store.created.SizeBytes != 4 {
		t.Fatalf("quarantine intent = %#v, want 4-byte entry", store.created)
	}
	if _, err := os.Stat(filepath.Join(store.created.QuarantinePath, "artifact")); err != nil {
		t.Fatalf("quarantined artifact missing: %v", err)
	}
}

func TestCleanupNeverClaimsUnmarkedManagedPath(t *testing.T) {
	home := t.TempDir()
	cachePath := filepath.Join(home, "cache", "go-build")
	if err := os.MkdirAll(cachePath, 0o755); err != nil {
		t.Fatalf("create cache: %v", err)
	}
	artifact := filepath.Join(cachePath, "artifact")
	if err := os.WriteFile(artifact, []byte("1234"), 0o600); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	settings := storage.DefaultSettings()
	settings.GoCache.Enabled = true
	settings.GoCache.MaxBytes = 1
	provider := New(Config{
		HomeDir:  home,
		TrashDir: filepath.Join(home, "trash"),
		Settings: staticSettings{settings: settings},
		Store:    &recordingStore{},
	})

	_, err := provider.Cleanup(context.Background())
	if !errors.Is(err, ErrNotOwned) {
		t.Fatalf("Cleanup() error = %v, want ErrNotOwned", err)
	}
	if _, err := os.Stat(artifact); err != nil {
		t.Fatalf("unowned cache was modified: %v", err)
	}
}

func TestCleanupRejectsSymlinkedManagedCacheAncestor(t *testing.T) {
	home := t.TempDir()
	external := t.TempDir()
	externalCache := filepath.Join(external, "go-build")
	if err := os.MkdirAll(externalCache, 0o755); err != nil {
		t.Fatalf("create external cache: %v", err)
	}
	artifact := filepath.Join(externalCache, "artifact")
	if err := os.WriteFile(artifact, []byte("leave external data"), 0o600); err != nil {
		t.Fatalf("seed external cache: %v", err)
	}
	if err := os.WriteFile(filepath.Join(external, markerName), []byte(markerContent), 0o600); err != nil {
		t.Fatalf("seed external marker: %v", err)
	}
	if err := os.Symlink(external, filepath.Join(home, "cache")); err != nil {
		t.Fatalf("symlink cache ancestor: %v", err)
	}
	settings := storage.DefaultSettings()
	settings.GoCache.Enabled = true
	settings.GoCache.MaxBytes = 1
	store := &recordingStore{}
	provider := New(Config{
		HomeDir: home, TrashDir: filepath.Join(home, "trash"),
		Settings: staticSettings{settings: settings}, Store: store,
	})

	if _, err := provider.Cleanup(context.Background()); err == nil {
		t.Fatal("Cleanup succeeded through a symlinked managed-cache ancestor")
	}
	if data, err := os.ReadFile(artifact); err != nil || string(data) != "leave external data" {
		t.Fatalf("external cache changed: data=%q err=%v", data, err)
	}
	if store.created != nil {
		t.Fatalf("quarantine intent persisted for unsafe cache: %#v", store.created)
	}
}

func TestCleanupRejectsSymlinkedTrashAncestor(t *testing.T) {
	home := t.TempDir()
	external := t.TempDir()
	trashLink := filepath.Join(home, "trash-link")
	if err := os.Symlink(external, trashLink); err != nil {
		t.Fatalf("symlink trash ancestor: %v", err)
	}
	settings := storage.DefaultSettings()
	settings.GoCache.Enabled = true
	settings.GoCache.MaxBytes = 1
	store := &recordingStore{}
	provider := New(Config{
		HomeDir: home, TrashDir: filepath.Join(trashLink, "nested"),
		Settings: staticSettings{settings: settings}, Store: store,
	})
	env, err := provider.ExecutionEnvironment(context.Background())
	if err != nil {
		t.Fatalf("ExecutionEnvironment() error = %v", err)
	}
	artifact := filepath.Join(env["GOCACHE"], "artifact")
	if err := os.WriteFile(artifact, []byte("keep cache"), 0o600); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	if _, err := provider.Cleanup(context.Background()); err == nil {
		t.Fatal("Cleanup succeeded through a symlinked trash ancestor")
	}
	if data, err := os.ReadFile(artifact); err != nil || string(data) != "keep cache" {
		t.Fatalf("cache changed despite unsafe trash: data=%q err=%v", data, err)
	}
	if store.created != nil {
		t.Fatalf("quarantine intent persisted for unsafe trash: %#v", store.created)
	}
	if _, err := os.Stat(filepath.Join(external, "nested")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("external trash target changed: %v", err)
	}
}

func TestCleanupRejectsSymlinkedOwnershipMarker(t *testing.T) {
	home := t.TempDir()
	cachePath := filepath.Join(home, "cache", "go-build")
	if err := os.MkdirAll(cachePath, 0o755); err != nil {
		t.Fatalf("create cache: %v", err)
	}
	artifact := filepath.Join(cachePath, "artifact")
	if err := os.WriteFile(artifact, []byte("keep cache"), 0o600); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	externalMarker := filepath.Join(t.TempDir(), "marker")
	if err := os.WriteFile(externalMarker, []byte(markerContent), 0o600); err != nil {
		t.Fatalf("seed external marker: %v", err)
	}
	if err := os.Symlink(externalMarker, filepath.Join(filepath.Dir(cachePath), markerName)); err != nil {
		t.Fatalf("symlink ownership marker: %v", err)
	}
	settings := storage.DefaultSettings()
	settings.GoCache.Enabled = true
	settings.GoCache.MaxBytes = 1
	store := &recordingStore{}
	provider := New(Config{
		HomeDir: home, TrashDir: filepath.Join(home, "trash"),
		Settings: staticSettings{settings: settings}, Store: store,
	})

	if _, err := provider.Cleanup(context.Background()); !errors.Is(err, ErrNotOwned) {
		t.Fatalf("Cleanup() error = %v, want ErrNotOwned", err)
	}
	if data, err := os.ReadFile(artifact); err != nil || string(data) != "keep cache" {
		t.Fatalf("cache changed despite symlinked marker: data=%q err=%v", data, err)
	}
	if store.created != nil {
		t.Fatalf("quarantine intent persisted for symlinked marker: %#v", store.created)
	}
}

func TestCleanupPersistsIntentBeforeRename(t *testing.T) {
	home := t.TempDir()
	settings := storage.DefaultSettings()
	settings.GoCache.Enabled = true
	settings.GoCache.MaxBytes = 1
	storeErr := errors.New("database unavailable")
	provider := New(Config{
		HomeDir:  home,
		TrashDir: filepath.Join(home, "trash"),
		Settings: staticSettings{settings: settings},
		Store:    &recordingStore{createErr: storeErr},
	})
	env, err := provider.ExecutionEnvironment(context.Background())
	if err != nil {
		t.Fatalf("ExecutionEnvironment() error = %v", err)
	}
	artifact := filepath.Join(env["GOCACHE"], "artifact")
	if err := os.WriteFile(artifact, []byte("1234"), 0o600); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	_, err = provider.Cleanup(context.Background())
	if !errors.Is(err, storeErr) {
		t.Fatalf("Cleanup() error = %v, want wrapped store error", err)
	}
	if _, err := os.Stat(artifact); err != nil {
		t.Fatalf("cache moved before intent persisted: %v", err)
	}
}

func TestDisabledEnvironmentDoesNotDeleteExistingManagedCache(t *testing.T) {
	home := t.TempDir()
	settings := storage.DefaultSettings()
	settings.GoCache.Enabled = true
	settings.GoCache.MaxBytes = 1
	provider := New(Config{
		HomeDir:  home,
		TrashDir: filepath.Join(home, "trash"),
		Settings: staticSettings{settings: settings},
		Store:    &recordingStore{},
	})
	env, err := provider.ExecutionEnvironment(context.Background())
	if err != nil {
		t.Fatalf("ExecutionEnvironment() error = %v", err)
	}
	artifact := filepath.Join(env["GOCACHE"], "artifact")
	if err := os.WriteFile(artifact, []byte("keep"), 0o600); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	settings.GoCache.Enabled = false
	provider.config.Settings = staticSettings{settings: settings}

	disabledEnv, err := provider.ExecutionEnvironment(context.Background())
	if err != nil {
		t.Fatalf("disabled ExecutionEnvironment() error = %v", err)
	}
	if _, exists := disabledEnv["GOCACHE"]; exists {
		t.Fatalf("disabled environment injected GOCACHE: %#v", disabledEnv)
	}
	if _, err := provider.Cleanup(context.Background()); err != nil {
		t.Fatalf("disabled Cleanup() error = %v", err)
	}
	if _, err := os.Stat(artifact); err != nil {
		t.Fatalf("disabling the cache deleted existing data: %v", err)
	}
	result, err := provider.CleanupExplicit(context.Background())
	if err != nil {
		t.Fatalf("disabled CleanupExplicit() error = %v", err)
	}
	if result.ReclaimedBytes == 0 {
		t.Fatalf("disabled CleanupExplicit() result = %#v, want reclaimed bytes", result)
	}
	if _, err := os.Stat(artifact); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("explicit cleanup left the managed cache artifact: %v", err)
	}
}

func TestValidateAdoptionRequiresExplicitConfirmation(t *testing.T) {
	home := t.TempDir()
	cachePath := filepath.Join(home, "user-go-cache")
	if err := os.Mkdir(cachePath, 0o755); err != nil {
		t.Fatalf("create cache: %v", err)
	}
	provider := New(Config{HomeDir: home, TrashDir: filepath.Join(home, "trash")})

	err := provider.ValidateAdoption(context.Background(), cachePath, "")
	if !errors.Is(err, ErrAdoptionConfirmation) {
		t.Fatalf("ValidateAdoption() error = %v, want ErrAdoptionConfirmation", err)
	}
}

func TestAdoptedCacheCanBeRotatedWithoutManagedMarker(t *testing.T) {
	home := t.TempDir()
	cachePath := filepath.Join(home, "user-go-cache")
	if err := os.Mkdir(cachePath, 0o755); err != nil {
		t.Fatalf("create cache: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "artifact"), []byte("1234"), 0o600); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	settings := storage.DefaultSettings()
	settings.GoCache.Enabled = true
	settings.GoCache.MaxBytes = 1
	settings.GoCache.AdoptedPath = cachePath
	store := &recordingStore{}
	provider := New(Config{
		HomeDir:  home,
		TrashDir: filepath.Join(home, "trash"),
		Settings: staticSettings{settings: settings},
		Store:    store,
	})
	if err := provider.ValidateAdoption(context.Background(), cachePath, "ADOPT"); err != nil {
		t.Fatalf("ValidateAdoption() error = %v", err)
	}

	result, err := provider.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if result.ReclaimedBytes != 4 || store.created == nil {
		t.Fatalf("adopted cleanup result=%#v entry=%#v", result, store.created)
	}
}

func TestCleanupDoesNotDiscoverUserDefaultCache(t *testing.T) {
	home := t.TempDir()
	userCache := filepath.Join(t.TempDir(), ".cache", "go-build")
	if err := os.MkdirAll(userCache, 0o755); err != nil {
		t.Fatalf("create user cache: %v", err)
	}
	artifact := filepath.Join(userCache, "artifact")
	if err := os.WriteFile(artifact, []byte("leave me"), 0o600); err != nil {
		t.Fatalf("seed user cache: %v", err)
	}
	settings := storage.DefaultSettings()
	settings.GoCache.Enabled = true
	settings.GoCache.MaxBytes = 1
	provider := New(Config{
		HomeDir:  home,
		TrashDir: filepath.Join(home, "trash"),
		Settings: staticSettings{settings: settings},
		Store:    &recordingStore{},
	})
	if _, err := provider.ExecutionEnvironment(context.Background()); err != nil {
		t.Fatalf("ExecutionEnvironment() error = %v", err)
	}

	if _, err := provider.Cleanup(context.Background()); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if _, err := os.Stat(artifact); err != nil {
		t.Fatalf("unadopted user cache was modified: %v", err)
	}
}
