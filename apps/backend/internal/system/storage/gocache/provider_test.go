package gocache

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
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
	entries    map[string]storage.QuarantineEntry
	onCreate   func(*storage.QuarantineEntry)
}

func (s *recordingStore) CreateQuarantineEntry(_ context.Context, entry *storage.QuarantineEntry) error {
	if s.createErr != nil {
		return s.createErr
	}
	if s.entries == nil {
		s.entries = make(map[string]storage.QuarantineEntry)
	}
	for _, existing := range s.entries {
		if existing.OriginalPath == entry.OriginalPath &&
			(existing.State == storage.QuarantineStateQuarantined || existing.State == storage.QuarantineStateFailed) {
			return errors.New("duplicate active quarantine original path")
		}
	}
	copy := *entry
	s.created = &copy
	s.entries[entry.ID] = copy
	if s.onCreate != nil {
		s.onCreate(&copy)
	}
	return nil
}

func (s *recordingStore) TransitionQuarantineEntry(
	_ context.Context,
	id string,
	next storage.QuarantineState,
	lastError string,
) (storage.QuarantineEntry, error) {
	s.transition = next
	entry := s.entries[id]
	entry.State = next
	entry.LastError = lastError
	s.entries[id] = entry
	return entry, nil
}

func (s *recordingStore) ListQuarantineEntries(
	_ context.Context,
	includeTerminal bool,
) ([]storage.QuarantineEntry, error) {
	entries := make([]storage.QuarantineEntry, 0, len(s.entries))
	for _, entry := range s.entries {
		if !includeTerminal && entry.State != storage.QuarantineStateQuarantined &&
			entry.State != storage.QuarantineStateFailed {
			continue
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].QuarantinedAt.Equal(entries[j].QuarantinedAt) {
			return entries[i].ID > entries[j].ID
		}
		return entries[i].QuarantinedAt.After(entries[j].QuarantinedAt)
	})
	return entries, nil
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
	if _, err := os.Stat(filepath.Join(want, markerName)); err != nil {
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

func TestCleanupDoesNotClaimReplacementDirectoryFromStaleMarker(t *testing.T) {
	home := t.TempDir()
	settings := storage.DefaultSettings()
	settings.GoCache.Enabled = true
	settings.GoCache.MaxBytes = 1
	store := &recordingStore{}
	provider := New(Config{
		HomeDir: home, TrashDir: filepath.Join(home, "trash"),
		Settings: staticSettings{settings: settings}, Store: store,
	})
	env, err := provider.ExecutionEnvironment(context.Background())
	if err != nil {
		t.Fatalf("ExecutionEnvironment() error = %v", err)
	}
	cachePath := env["GOCACHE"]
	if err := os.RemoveAll(cachePath); err != nil {
		t.Fatalf("replace managed cache: %v", err)
	}
	if err := os.MkdirAll(cachePath, 0o755); err != nil {
		t.Fatalf("create replacement cache: %v", err)
	}
	artifact := filepath.Join(cachePath, "unrelated")
	if err := os.WriteFile(artifact, []byte("keep"), 0o600); err != nil {
		t.Fatalf("seed replacement cache: %v", err)
	}

	_, err = provider.Cleanup(context.Background())
	if !errors.Is(err, ErrNotOwned) {
		t.Fatalf("Cleanup() error = %v, want ErrNotOwned", err)
	}
	if data, err := os.ReadFile(artifact); err != nil || string(data) != "keep" {
		t.Fatalf("replacement cache changed: data=%q err=%v", data, err)
	}
	if store.created != nil {
		t.Fatalf("replacement cache was quarantined: %#v", store.created)
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
	if err := os.WriteFile(filepath.Join(externalCache, markerName), []byte(markerContent), 0o600); err != nil {
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
	if err := os.Symlink(externalMarker, filepath.Join(cachePath, markerName)); err != nil {
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

func TestCleanupRetriesFailedGoCacheIntentWhenMoveNeverHappened(t *testing.T) {
	home := t.TempDir()
	settings := storage.DefaultSettings()
	settings.GoCache.Enabled = true
	settings.GoCache.MaxBytes = 1
	store := &recordingStore{}
	var firstID string
	store.onCreate = func(entry *storage.QuarantineEntry) {
		if firstID != "" {
			return
		}
		firstID = entry.ID
		if err := os.MkdirAll(entry.QuarantinePath, 0o700); err != nil {
			t.Fatalf("create blocked quarantine: %v", err)
		}
		if err := os.WriteFile(filepath.Join(entry.QuarantinePath, "blocker"), []byte("block"), 0o600); err != nil {
			t.Fatalf("write quarantine blocker: %v", err)
		}
	}
	provider := New(Config{
		HomeDir: home, TrashDir: filepath.Join(home, "trash"),
		Settings: staticSettings{settings: settings}, Store: store,
	})
	env, err := provider.ExecutionEnvironment(context.Background())
	if err != nil {
		t.Fatalf("ExecutionEnvironment() error = %v", err)
	}
	artifact := filepath.Join(env["GOCACHE"], "artifact")
	if err := os.WriteFile(artifact, []byte("1234"), 0o600); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	if _, err := provider.Cleanup(context.Background()); err == nil {
		t.Fatal("first Cleanup succeeded despite blocked quarantine destination")
	}
	if got := store.entries[firstID].State; got != storage.QuarantineStateFailed {
		t.Fatalf("first intent state = %q, want failed", got)
	}
	if _, err := provider.Cleanup(context.Background()); !errors.Is(err, storage.ErrConflict) {
		t.Fatalf("ambiguous retry error = %v, want ErrConflict", err)
	}
	if got := store.entries[firstID].State; got != storage.QuarantineStateFailed {
		t.Fatalf("ambiguous intent state = %q, want failed", got)
	}
	if err := os.RemoveAll(store.entries[firstID].QuarantinePath); err != nil {
		t.Fatalf("remove quarantine blocker: %v", err)
	}

	result, err := provider.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("retry Cleanup: %v", err)
	}
	if result.QuarantineEntry == nil || result.QuarantineEntry.ID == firstID {
		t.Fatalf("retry result = %#v, want fresh quarantine intent", result)
	}
	if got := store.entries[firstID].State; got != storage.QuarantineStateRestored {
		t.Fatalf("released intent state = %q, want restored", got)
	}
	if _, err := os.Stat(artifact); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("retry left original cache artifact: %v", err)
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

func TestIsRestorePlaceholderDoesNotModifyCache(t *testing.T) {
	tests := []struct {
		name    string
		adopted bool
	}{
		{name: "managed"},
		{name: "adopted", adopted: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cachePath := filepath.Join(t.TempDir(), "go-build")
			if err := os.MkdirAll(cachePath, 0o700); err != nil {
				t.Fatal(err)
			}
			if !test.adopted {
				if err := writeMarker(cachePath); err != nil {
					t.Fatal(err)
				}
			}

			placeholder, err := IsRestorePlaceholder(cachePath, test.adopted)
			if err != nil {
				t.Fatalf("IsRestorePlaceholder: %v", err)
			}
			if !placeholder {
				t.Fatal("expected rotation placeholder")
			}
			if _, err := os.Stat(cachePath); err != nil {
				t.Fatalf("placeholder was modified: %v", err)
			}
		})
	}
}
