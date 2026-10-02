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

	identity := metrics.FilesystemIdentity
	rootHandle, err := provider.openValidatedCacheRoot(cachePath, false, identity)
	if err != nil {
		t.Fatalf("open cache root: %v", err)
	}
	actualDescriptorIdentity := rootHandle.descriptorIdentity
	rootHandle.descriptorIdentity = func(file *os.File) (string, error) {
		mountIdentity, err := actualDescriptorIdentity(file)
		if err != nil {
			return "", err
		}
		name, err := filepath.Rel(cachePath, file.Name())
		if err == nil && pathWithinTest("mounted", name) {
			return mountIdentity + "\x00same-device-bind-mount", nil
		}
		return mountIdentity, nil
	}
	result, err := provider.cleanupContentsWithPreparedRoot(
		context.Background(), cachePath, false, 1, cleanupEntryLimit,
		"same-device-mount-test", identity, rootHandle,
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
	if result.BytesBefore != int64(len("compiled bytes")) || result.BytesAfter != nil {
		t.Fatalf("same-device mounted data was included in cache accounting: %#v", result)
	}
	if !strings.Contains(strings.Join(result.Errors, ","), "filesystem_boundary_skipped") {
		t.Fatalf("same-device mount boundary was not reported: %#v", result)
	}
}

func TestCleanupRemovalRechecksSameDeviceMountBoundary(t *testing.T) {
	provider, cachePath := newManagedCacheForCleanupTest(t, 1)
	mountedFile := filepath.Join(cachePath, "mounted", "foreign-data")
	writeCleanupFixture(t, mountedFile, "mounted data")
	identity := metrics.FilesystemIdentity
	rootHandle, err := provider.openValidatedCacheRoot(cachePath, false, identity)
	if err != nil {
		t.Fatalf("open cache root: %v", err)
	}
	actualDescriptorIdentity := rootHandle.descriptorIdentity
	rootHandle.descriptorIdentity = func(file *os.File) (string, error) {
		mountIdentity, err := actualDescriptorIdentity(file)
		if err != nil {
			return "", err
		}
		name, err := filepath.Rel(cachePath, file.Name())
		if err == nil && filepath.Clean(name) == filepath.Join("mounted", "foreign-data") {
			return mountIdentity + "\x00same-device-bind-mount", nil
		}
		return mountIdentity, nil
	}
	session, err := newCleanupSessionWithRoot(
		cachePath, false, 1, "same-device-removal-test", identity, rootHandle,
	)
	if err != nil {
		t.Fatalf("open cleanup session: %v", err)
	}
	defer session.close()
	directoryInfo, err := os.Stat(filepath.Dir(mountedFile))
	if err != nil {
		t.Fatalf("stat mounted directory: %v", err)
	}
	session.directories["mounted"] = directoryInfo
	fileInfo, err := os.Stat(mountedFile)
	if err != nil {
		t.Fatalf("stat mounted file: %v", err)
	}
	if _, err := session.removeEntry(cleanupEntry{relative: filepath.Join("mounted", "foreign-data"), info: fileInfo}); err == nil {
		t.Fatal("cleanup removed a file after its mount identity changed")
	}
	if data, err := os.ReadFile(mountedFile); err != nil || string(data) != "mounted data" {
		t.Fatalf("mounted data changed: data=%q err=%v", data, err)
	}
}

func TestCleanupRejectsAncestorRedirectAfterPathValidation(t *testing.T) {
	home := t.TempDir()
	external := t.TempDir()
	externalCache := filepath.Join(external, "go-build")
	if err := os.MkdirAll(externalCache, 0o700); err != nil {
		t.Fatal(err)
	}
	externalArtifact := filepath.Join(externalCache, "artifact")
	writeCleanupFixture(t, externalArtifact, "external build data")
	if err := writeMarker(externalCache); err != nil {
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
	cachePath := filepath.Join(home, ".cache", "kandev", "go-build")
	if err := os.MkdirAll(cachePath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeMarker(cachePath); err != nil {
		t.Fatal(err)
	}
	if err := provider.validateCachePath(cachePath); err != nil {
		t.Fatalf("validate cache before ancestor replacement: %v", err)
	}

	parent := filepath.Dir(cachePath)
	movedParent := parent + ".moved"
	if err := os.Rename(parent, movedParent); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, parent); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := provider.cleanupContents(context.Background(), cachePath, false, 1); err == nil {
		t.Fatal("cleanup succeeded after an ancestor redirected the validated path")
	}
	if data, err := os.ReadFile(externalArtifact); err != nil || string(data) != "external build data" {
		t.Fatalf("external cache changed: data=%q err=%v", data, err)
	}
}

func TestCleanupRetainsValidatedAnchorAcrossAncestorRedirect(t *testing.T) {
	provider, cachePath := newManagedCacheForCleanupTest(t, 1)
	artifact := filepath.Join(cachePath, "compiled")
	writeCleanupFixture(t, artifact, "selected cache data")

	rootHandle, err := provider.openValidatedCacheRoot(cachePath, false, cacheFilesystemIdentity)
	if err != nil {
		t.Fatalf("open selected cache root: %v", err)
	}
	parent := filepath.Dir(cachePath)
	movedParent := parent + ".moved"
	if err := os.Rename(parent, movedParent); err != nil {
		rootHandle.close()
		t.Fatal(err)
	}
	external := t.TempDir()
	externalCache := filepath.Join(external, filepath.Base(cachePath))
	writeCleanupFixture(t, filepath.Join(externalCache, "compiled"), "external cache data")
	if err := writeMarker(externalCache); err != nil {
		rootHandle.close()
		t.Fatal(err)
	}
	if err := os.Symlink(external, parent); err != nil {
		rootHandle.close()
		t.Skipf("symlinks unavailable: %v", err)
	}

	result, err := provider.cleanupContentsWithPreparedRoot(
		context.Background(), cachePath, false, 1, cleanupEntryLimit,
		"filesystem", cacheFilesystemIdentity, rootHandle,
	)
	if err == nil || !result.Partial {
		t.Fatalf("Cleanup = (%#v, %v), want a partial path-change result", result, err)
	}
	movedArtifact := filepath.Join(movedParent, filepath.Base(cachePath), filepath.Base(artifact))
	if data, err := os.ReadFile(movedArtifact); err != nil || string(data) != "selected cache data" {
		t.Fatalf("the retained cache changed after its path was redirected: data=%q err=%v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(externalCache, "compiled")); err != nil || string(data) != "external cache data" {
		t.Fatalf("external cache changed after redirect: data=%q err=%v", data, err)
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
	var reclaimed int64
	completed := false
	for pass := 0; pass < 8; pass++ {
		result, err := cleanup()
		reclaimed += result.ReclaimedBytes
		if result.BytesBefore <= 5 {
			if result.ReclaimedBytes != 0 {
				t.Fatalf("pass %d reclaimed bytes before crossing threshold: %#v", pass+1, result)
			}
			for _, path := range files {
				if _, statErr := os.Stat(path); statErr != nil {
					t.Fatalf("pass %d deleted %s before crossing threshold: %v", pass+1, path, statErr)
				}
			}
		}
		if !result.Partial && err == nil {
			completed = true
			break
		}
	}
	if !completed {
		t.Fatal("cleanup did not complete within eight bounded passes")
	}
	if reclaimed != 6 {
		t.Fatalf("reclaimed bytes = %d, want 6", reclaimed)
	}
}

func TestCleanupSharesEntryBudgetAcrossDiscoveryAndDeletion(t *testing.T) {
	provider, cachePath := newManagedCacheForCleanupTest(t, 1)
	artifacts := []string{
		filepath.Join(cachePath, "artifact-a"),
		filepath.Join(cachePath, "artifact-b"),
	}
	for _, path := range artifacts {
		writeCleanupFixture(t, path, "xx")
	}

	result, err := provider.cleanupContentsWithLimit(
		context.Background(), cachePath, false, 3, 2, cacheFilesystemIdentity,
	)
	if err == nil || !result.Partial || result.ReclaimedBytes != 0 {
		t.Fatalf("first pass = (%#v, %v), want a bounded partial pass with no budget left to delete", result, err)
	}
	for _, path := range artifacts {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("threshold scan deleted %s after consuming its entry budget: %v", path, err)
		}
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
