package gocache

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/system/metrics"
	"github.com/kandev/kandev/internal/system/storage"
)

func TestCleanupPreservesFuzzCorpus(t *testing.T) {
	provider, cachePath := newManagedCacheForCleanupTest(t, 1)
	artifact := filepath.Join(cachePath, "00", "compiled")
	corpus := filepath.Join(cachePath, "fuzz", "FuzzDecode", "seed")
	writeCleanupFixture(t, artifact, "compiled bytes")
	writeCleanupFixture(t, corpus, "fuzz seed")

	result, err := provider.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if _, err := os.Stat(artifact); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("compiled artifact remains: %v", err)
	}
	if data, err := os.ReadFile(corpus); err != nil || string(data) != "fuzz seed" {
		t.Fatalf("fuzz corpus changed: data=%q err=%v", data, err)
	}
	if result.BytesBefore != int64(len("compiled bytes")) || result.ReclaimedBytes != int64(len("compiled bytes")) {
		t.Fatalf("cleanup counted fuzz bytes as build artifacts: %#v", result)
	}
}

func TestCleanupPreservesSameDeviceMountBoundary(t *testing.T) {
	provider, cachePath := newManagedCacheForCleanupTest(t, 1)
	artifact := filepath.Join(cachePath, "00", "compiled")
	mountedFile := filepath.Join(cachePath, "mounted", "foreign-data")
	writeCleanupFixture(t, artifact, "compiled bytes")
	writeCleanupFixture(t, mountedFile, "mounted data")

	rootDevice, err := metrics.FilesystemIdentity(cachePath)
	if err != nil {
		t.Fatalf("identify cache device: %v", err)
	}
	mountedDevice, err := metrics.FilesystemIdentity(filepath.Dir(mountedFile))
	if err != nil {
		t.Fatalf("identify nested device: %v", err)
	}
	if rootDevice != mountedDevice {
		t.Fatalf("test fixture is not on one device: root=%q mounted=%q", rootDevice, mountedDevice)
	}

	mountedPath := filepath.Dir(mountedFile)
	identity := func(path string) (string, error) {
		device, err := metrics.FilesystemIdentity(path)
		if err != nil {
			return "", err
		}
		mount := "cache-mount"
		if pathWithinTest(mountedPath, path) {
			mount = "same-device-bind-mount"
		}
		return device + "\x00" + mount, nil
	}
	result, err := provider.cleanupContentsWithOptions(
		context.Background(), cachePath, false, 1, cleanupEntryLimit, "same-device-mount-test", identity,
	)
	if err == nil || !result.Partial {
		t.Fatalf("Cleanup = (%#v, %v), want a partial result for the preserved mount", result, err)
	}
	if _, err := os.Stat(artifact); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("compiled artifact remains: %v", err)
	}
	if data, err := os.ReadFile(mountedFile); err != nil || string(data) != "mounted data" {
		t.Fatalf("same-device mounted data changed: data=%q err=%v", data, err)
	}
}

func TestCleanupContinuesThresholdScanAcrossBoundedPasses(t *testing.T) {
	provider, cachePath := newManagedCacheForCleanupTest(t, 5)
	files := []string{
		filepath.Join(cachePath, "00", "one"),
		filepath.Join(cachePath, "00", "two"),
		filepath.Join(cachePath, "00", "three"),
	}
	for _, path := range files {
		writeCleanupFixture(t, path, "xx")
	}

	cleanup := func() (CleanupResult, error) {
		return provider.cleanupContentsWithLimit(
			context.Background(), cachePath, false, 5, 2, cacheFilesystemIdentity,
		)
	}
	first, err := cleanup()
	if err == nil || !first.Skipped || !first.Partial {
		t.Fatalf("first pass = (%#v, %v), want incomplete non-destructive scan", first, err)
	}
	for _, path := range files {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("first pass deleted %s: %v", path, err)
		}
	}

	second, err := cleanup()
	if err == nil || !second.Skipped || !second.Partial {
		t.Fatalf("second pass = (%#v, %v), want continued incomplete scan", second, err)
	}
	for _, path := range files {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("second pass deleted %s: %v", path, err)
		}
	}

	third, err := cleanup()
	if err == nil || !third.Partial || third.ReclaimedBytes == 0 {
		t.Fatalf("third pass = (%#v, %v), want threshold-triggered deletion progress", third, err)
	}

	fourth, err := cleanup()
	if err != nil || fourth.Partial || fourth.BytesAfter == nil || *fourth.BytesAfter != 0 {
		t.Fatalf("fourth pass = (%#v, %v), want completed deletion and measured empty build cache", fourth, err)
	}
	if third.ReclaimedBytes+fourth.ReclaimedBytes != 6 {
		t.Fatalf("reclaimed bytes across deletion passes = %d, want 6", third.ReclaimedBytes+fourth.ReclaimedBytes)
	}
}

func newManagedCacheForCleanupTest(t *testing.T, maxBytes int64) (*Provider, string) {
	t.Helper()
	home := t.TempDir()
	settings := storage.DefaultSettings()
	settings.GoCache.Enabled = true
	settings.GoCache.MaxBytes = maxBytes
	provider := New(Config{HomeDir: home, TrashDir: filepath.Join(home, "trash"), Settings: staticSettings{settings: settings}})
	environment, err := provider.ExecutionEnvironment(context.Background())
	if err != nil {
		t.Fatalf("ExecutionEnvironment: %v", err)
	}
	return provider, environment["GOCACHE"]
}

func writeCleanupFixture(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func pathWithinTest(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && relative != "" && !filepath.IsAbs(relative) &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
