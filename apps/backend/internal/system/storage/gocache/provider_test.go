package gocache

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/system/storage"
)

func int64Pointer(value int64) *int64 {
	return &value
}

type staticSettings struct {
	settings storage.StorageMaintenanceSettings
}

func (s staticSettings) GetSettings(context.Context) (storage.StorageMaintenanceSettings, error) {
	return s.settings, nil
}

func TestAnalysisJSONUsesStorageAPISnakeCase(t *testing.T) {
	encoded, err := json.Marshal(Analysis{
		Path: "/cache", SizeBytes: 42, Owned: true, Enabled: false,
		UnmanagedPath: "/user-cache", UnmanagedSizeBytes: int64Pointer(24),
	})
	if err != nil {
		t.Fatalf("Marshal Analysis: %v", err)
	}
	want := `{"path":"/cache","size_bytes":42,"owned":true,"enabled":false,"unmanaged_path":"/user-cache","unmanaged_size_bytes":24}`
	if string(encoded) != want {
		t.Fatalf("Analysis JSON = %s, want %s", encoded, want)
	}
}

func TestCleanupResultJSONUsesStorageAPISnakeCase(t *testing.T) {
	encoded, err := json.Marshal(CleanupResult{
		Path: "/cache", BytesBefore: 100, BytesBeforeComplete: true,
		BytesAfter: int64Pointer(20), ReclaimedBytes: 80,
	})
	if err != nil {
		t.Fatalf("Marshal CleanupResult: %v", err)
	}
	want := `{"path":"/cache","skipped":false,"bytes_before":100,"bytes_before_complete":true,"bytes_after":20,"reclaimed_bytes":80,"quarantine_entry":null}`
	if string(encoded) != want {
		t.Fatalf("CleanupResult JSON = %s, want %s", encoded, want)
	}
}

func TestAnalyzeSerializesMeasuredZeroForUnmanagedCache(t *testing.T) {
	home := t.TempDir()
	userCache := filepath.Join(t.TempDir(), "go-build")
	if err := os.MkdirAll(userCache, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOCACHE", userCache)
	provider := New(Config{
		HomeDir:  home,
		TrashDir: filepath.Join(home, "trash"),
		Settings: staticSettings{settings: storage.DefaultSettings()},
	})

	analysis, err := provider.Analyze(context.Background())
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if analysis.UnmanagedPath != userCache || analysis.UnmanagedSizeBytes == nil || *analysis.UnmanagedSizeBytes != 0 {
		t.Fatalf("analysis = %#v, want an explicit measured zero", analysis)
	}
	encoded, err := json.Marshal(analysis)
	if err != nil {
		t.Fatalf("Marshal Analysis: %v", err)
	}
	if got := string(encoded); !strings.Contains(got, `"unmanaged_size_bytes":0`) {
		t.Fatalf("serialized analysis = %s, want unmanaged_size_bytes zero", encoded)
	}
}

func TestExecutionEnvironmentCreatesOwnedManagedCache(t *testing.T) {
	home := t.TempDir()
	settings := storage.DefaultSettings()
	settings.GoCache.Enabled = true
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
	if !hasValidMarker(want) {
		t.Fatal("ownership marker was not created")
	}
}

func TestAnalyzeReportsUnmanagedDefaultGoCacheReadOnly(t *testing.T) {
	home := t.TempDir()
	userCache := filepath.Join(t.TempDir(), "go-build")
	if err := os.MkdirAll(userCache, 0o755); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(userCache, "artifact")
	if err := os.WriteFile(artifact, []byte("user cache bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	externalArtifact := filepath.Join(t.TempDir(), "external")
	if err := os.WriteFile(externalArtifact, []byte("external bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(externalArtifact, filepath.Join(userCache, "external-link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	t.Setenv("GOCACHE", userCache)
	provider := New(Config{
		HomeDir:  home,
		TrashDir: filepath.Join(home, "trash"),
		Settings: staticSettings{settings: storage.DefaultSettings()},
	})

	analysis, err := provider.Analyze(context.Background())
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if analysis.UnmanagedPath != userCache || analysis.UnmanagedSizeBytes == nil || *analysis.UnmanagedSizeBytes != int64(len("user cache bytes")) {
		t.Fatalf("analysis = %#v, want unmanaged cache %s", analysis, userCache)
	}
	if _, err := os.Stat(artifact); err != nil {
		t.Fatalf("Analyze modified unmanaged cache: %v", err)
	}
	if _, err := os.Stat(externalArtifact); err != nil {
		t.Fatalf("Analyze followed or modified unmanaged cache symlink: %v", err)
	}
}

func TestCleanupSkipsBelowThreshold(t *testing.T) {
	home := t.TempDir()
	settings := storage.DefaultSettings()
	settings.GoCache.Enabled = true
	settings.GoCache.MaxBytes = 100
	provider := New(Config{
		HomeDir:  home,
		TrashDir: filepath.Join(home, "trash"),
		Settings: staticSettings{settings: settings},
	})
	env, err := provider.ExecutionEnvironment(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(env["GOCACHE"], "artifact")
	if err := os.WriteFile(artifact, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := provider.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if result.ReclaimedBytes != 0 || result.BytesAfter == nil || *result.BytesAfter != 4 {
		t.Fatalf("cleanup result = %#v, want measured unchanged cache", result)
	}
	if _, err := os.Stat(artifact); err != nil {
		t.Fatalf("below-threshold cache changed: %v", err)
	}
}

func TestCleanupNeverClaimsUnmarkedManagedPath(t *testing.T) {
	home := t.TempDir()
	cachePath := filepath.Join(home, "cache", "go-build")
	if err := os.MkdirAll(cachePath, 0o755); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(cachePath, "artifact")
	if err := os.WriteFile(artifact, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	settings := storage.DefaultSettings()
	settings.GoCache.Enabled = true
	settings.GoCache.MaxBytes = 1
	provider := New(Config{
		HomeDir:  home,
		TrashDir: filepath.Join(home, "trash"),
		Settings: staticSettings{settings: settings},
	})

	if _, err := provider.Cleanup(context.Background()); !errors.Is(err, ErrNotOwned) {
		t.Fatalf("Cleanup error = %v, want ErrNotOwned", err)
	}
	if _, err := os.Stat(artifact); err != nil {
		t.Fatalf("unowned cache changed: %v", err)
	}
}

func TestCleanupRejectsStaleOwnershipMarker(t *testing.T) {
	home := t.TempDir()
	cachePath := filepath.Join(home, "cache", "go-build")
	if err := os.MkdirAll(cachePath, 0o755); err != nil {
		t.Fatal(err)
	}
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
		t.Fatal(err)
	}
	cachePath = env["GOCACHE"]
	if err := os.Remove(filepath.Join(cachePath, markerName)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(cachePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(cachePath, 0o755); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(cachePath, "unrelated")
	if err := os.WriteFile(artifact, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Cleanup(context.Background()); !errors.Is(err, ErrNotOwned) {
		t.Fatalf("Cleanup error = %v, want ErrNotOwned", err)
	}
	if data, err := os.ReadFile(artifact); err != nil || string(data) != "keep" {
		t.Fatalf("replacement cache changed: data=%q err=%v", data, err)
	}
}

func TestCleanupRejectsSymlinkedManagedCacheAncestor(t *testing.T) {
	home := t.TempDir()
	external := t.TempDir()
	externalCache := filepath.Join(external, "go-build")
	if err := os.MkdirAll(externalCache, 0o755); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(externalCache, "artifact")
	if err := os.WriteFile(artifact, []byte("leave external data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(externalCache, markerName), []byte(markerContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(home, "cache")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	settings := storage.DefaultSettings()
	settings.GoCache.Enabled = true
	settings.GoCache.MaxBytes = 1
	provider := New(Config{
		HomeDir:  home,
		TrashDir: filepath.Join(home, "trash"),
		Settings: staticSettings{settings: settings},
	})

	if _, err := provider.Cleanup(context.Background()); err == nil {
		t.Fatal("Cleanup succeeded through a symlinked managed-cache ancestor")
	}
	if data, err := os.ReadFile(artifact); err != nil || string(data) != "leave external data" {
		t.Fatalf("external cache changed: data=%q err=%v", data, err)
	}
}

func TestCleanupRejectsSymlinkedOwnershipMarker(t *testing.T) {
	home := t.TempDir()
	cachePath := filepath.Join(home, "cache", "go-build")
	if err := os.MkdirAll(cachePath, 0o755); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(cachePath, "artifact")
	if err := os.WriteFile(artifact, []byte("keep cache"), 0o600); err != nil {
		t.Fatal(err)
	}
	externalMarker := filepath.Join(t.TempDir(), "marker")
	if err := os.WriteFile(externalMarker, []byte(markerContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(externalMarker, filepath.Join(cachePath, markerName)); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	settings := storage.DefaultSettings()
	settings.GoCache.Enabled = true
	settings.GoCache.MaxBytes = 1
	provider := New(Config{
		HomeDir:  home,
		TrashDir: filepath.Join(home, "trash"),
		Settings: staticSettings{settings: settings},
	})

	if _, err := provider.Cleanup(context.Background()); !errors.Is(err, ErrNotOwned) {
		t.Fatalf("Cleanup error = %v, want ErrNotOwned", err)
	}
	if data, err := os.ReadFile(artifact); err != nil || string(data) != "keep cache" {
		t.Fatalf("cache changed despite symlinked marker: data=%q err=%v", data, err)
	}
}

func TestCleanupPreservesSymlinkTargetsAndReportsUnknownRemainingBytes(t *testing.T) {
	home := t.TempDir()
	external := filepath.Join(t.TempDir(), "external")
	if err := os.WriteFile(external, []byte("outside data"), 0o600); err != nil {
		t.Fatal(err)
	}
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
		t.Fatal(err)
	}
	cachePath := env["GOCACHE"]
	artifact := filepath.Join(cachePath, "compiled")
	if err := os.WriteFile(artifact, []byte("cache data"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(cachePath, "external-link")
	if err := os.Symlink(external, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	result, err := provider.Cleanup(context.Background())
	if err == nil || !result.Partial {
		t.Fatalf("Cleanup = (%#v, %v), want a reported partial result", result, err)
	}
	if result.ReclaimedBytes != int64(len("cache data")) || result.BytesAfter != nil {
		t.Fatalf("cleanup measurements = %#v, want known removals and unknown remainder", result)
	}
	if _, err := os.Stat(artifact); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cache artifact remains: %v", err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("cache symlink changed: info=%v err=%v", info, err)
	}
	if data, err := os.ReadFile(external); err != nil || string(data) != "outside data" {
		t.Fatalf("external target changed: data=%q err=%v", data, err)
	}
}

func TestCleanupCancellationPreservesCache(t *testing.T) {
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
		t.Fatal(err)
	}
	artifact := filepath.Join(env["GOCACHE"], "compiled")
	if err := os.WriteFile(artifact, []byte("cache data"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := provider.Cleanup(ctx)
	if !errors.Is(err, context.Canceled) || !result.Partial || result.BytesAfter != nil {
		t.Fatalf("Cleanup = (%#v, %v), want cancelled partial result and unknown remainder", result, err)
	}
	if _, err := os.Stat(artifact); err != nil {
		t.Fatalf("cancelled cleanup changed cache: %v", err)
	}
}

func TestCleanupStopsWhenCacheRootIsReplaced(t *testing.T) {
	home := t.TempDir()
	settings := storage.DefaultSettings()
	settings.GoCache.Enabled = true
	provider := New(Config{
		HomeDir:  home,
		TrashDir: filepath.Join(home, "trash"),
		Settings: staticSettings{settings: settings},
	})
	env, err := provider.ExecutionEnvironment(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cachePath := env["GOCACHE"]
	root, rootInfo, _, err := openOwnedCacheRoot(cachePath, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	moved := cachePath + ".moved"
	if err := os.Rename(cachePath, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(cachePath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeMarker(cachePath); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(cachePath, "replacement")
	if err := os.WriteFile(artifact, []byte("keep replacement"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := verifyCacheRoot(root, cachePath, rootInfo, false); err == nil {
		t.Fatal("verification succeeded after the path changed to another directory")
	}
	if data, err := os.ReadFile(artifact); err != nil || string(data) != "keep replacement" {
		t.Fatalf("replacement cache changed: data=%q err=%v", data, err)
	}
}

func TestCleanupRejectsReplacementWithRetainedSession(t *testing.T) {
	provider, cachePath := newManagedCacheForCleanupTest(t, 1)
	writeCleanupFixture(t, filepath.Join(cachePath, "candidate"), "selected cache")
	first, err := provider.cleanupContentsWithLimit(
		context.Background(), cachePath, false, 1, 1, cacheFilesystemIdentity,
	)
	if err == nil || !first.Partial {
		t.Fatalf("initial bounded cleanup = (%#v, %v), want retained partial session", first, err)
	}

	moved := cachePath + ".moved"
	if err := os.Rename(cachePath, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(cachePath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeMarker(cachePath); err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(cachePath, "replacement")
	if err := os.WriteFile(replacement, []byte("keep replacement"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := provider.Cleanup(context.Background())
	if err == nil || !result.Partial {
		t.Fatalf("Cleanup with replaced root = (%#v, %v), want path-change failure", result, err)
	}
	if data, err := os.ReadFile(replacement); err != nil || string(data) != "keep replacement" {
		t.Fatalf("replacement cache changed: data=%q err=%v", data, err)
	}
}

func TestScanCacheStopsAtEntryLimit(t *testing.T) {
	home := t.TempDir()
	cachePath := filepath.Join(home, "cache")
	if err := os.Mkdir(cachePath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeMarker(cachePath); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one", "two", "three"} {
		if err := os.WriteFile(filepath.Join(cachePath, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	root, err := os.OpenRoot(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	filesystem, err := filesystemIdentity(cachePath)
	if err != nil {
		t.Fatal(err)
	}

	snapshot := scanCache(context.Background(), root, cachePath, filesystem, 2)
	if snapshot.examined > 2 || len(snapshot.issues) == 0 || !snapshot.limitReached {
		t.Fatalf("scan snapshot = %#v, want bounded incomplete scan", snapshot)
	}
}

func filesystemIdentity(path string) (string, error) {
	return cacheFilesystemIdentity(path)
}

func TestAdoptedCacheCleanupDoesNotRequireOwnershipMarker(t *testing.T) {
	home := t.TempDir()
	cachePath := filepath.Join(t.TempDir(), "adopted-cache")
	if err := os.Mkdir(cachePath, 0o700); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(cachePath, "compiled")
	if err := os.WriteFile(artifact, []byte("cache data"), 0o600); err != nil {
		t.Fatal(err)
	}
	settings := storage.DefaultSettings()
	settings.GoCache.Enabled = true
	settings.GoCache.MaxBytes = 1
	settings.GoCache.AdoptedPath = cachePath
	provider := New(Config{
		HomeDir:  home,
		TrashDir: filepath.Join(home, "trash"),
		Settings: staticSettings{settings: settings},
	})
	if err := provider.ValidateAdoption(context.Background(), cachePath, "ADOPT"); err != nil {
		t.Fatalf("ValidateAdoption: %v", err)
	}

	result, err := provider.Cleanup(context.Background())
	if err != nil || result.ReclaimedBytes != int64(len("cache data")) {
		t.Fatalf("Cleanup = (%#v, %v)", result, err)
	}
	if _, err := os.Stat(artifact); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("adopted cache artifact remains: %v", err)
	}
}

func TestCleanupDoesNotDiscoverUserDefaultCache(t *testing.T) {
	home := t.TempDir()
	userCache := filepath.Join(t.TempDir(), ".cache", "go-build")
	if err := os.MkdirAll(userCache, 0o755); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(userCache, "artifact")
	if err := os.WriteFile(artifact, []byte("leave me"), 0o600); err != nil {
		t.Fatal(err)
	}
	settings := storage.DefaultSettings()
	settings.GoCache.Enabled = true
	settings.GoCache.MaxBytes = 1
	provider := New(Config{
		HomeDir:  home,
		TrashDir: filepath.Join(home, "trash"),
		Settings: staticSettings{settings: settings},
	})
	if _, err := provider.ExecutionEnvironment(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Cleanup(context.Background()); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if _, err := os.Stat(artifact); err != nil {
		t.Fatalf("unadopted user cache changed: %v", err)
	}
}

func TestIsRestorePlaceholderDoesNotModifyCache(t *testing.T) {
	for _, adopted := range []bool{false, true} {
		t.Run(map[bool]string{false: "managed", true: "adopted"}[adopted], func(t *testing.T) {
			cachePath := filepath.Join(t.TempDir(), "go-build")
			if err := os.Mkdir(cachePath, 0o700); err != nil {
				t.Fatal(err)
			}
			if !adopted {
				if err := writeMarker(cachePath); err != nil {
					t.Fatal(err)
				}
			}
			placeholder, err := IsRestorePlaceholder(cachePath, adopted)
			if err != nil || !placeholder {
				t.Fatalf("IsRestorePlaceholder = (%v, %v), want true", placeholder, err)
			}
			if _, err := os.Stat(cachePath); err != nil {
				t.Fatalf("placeholder was modified: %v", err)
			}
		})
	}
}
