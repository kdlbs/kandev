package gocache

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/system/storage"
	"github.com/kandev/kandev/internal/system/storage/filescan"
)

func TestAnalyzeUsesConfiguredBoundedScanner(t *testing.T) {
	home := t.TempDir()
	settings := storage.DefaultSettings()
	settings.GoCache.Enabled = true
	provider := New(Config{
		HomeDir: home, TrashDir: filepath.Join(home, "trash"),
		Settings: staticSettings{settings: settings},
	})
	env, err := provider.ExecutionEnvironment(context.Background())
	if err != nil {
		t.Fatalf("ExecutionEnvironment: %v", err)
	}
	t.Setenv("GOCACHE", env["GOCACHE"])
	if err := os.WriteFile(filepath.Join(env["GOCACHE"], "artifact"), []byte("cache"), 0o600); err != nil {
		t.Fatal(err)
	}
	var completed atomic.Int32
	provider.config.Scanner = filescan.NewLimiter(1)
	provider.config.OnProgress = func(progress filescan.Progress) {
		if progress.Phase == filescan.RootCompleted {
			completed.Add(1)
		}
	}

	if _, err := provider.Analyze(context.Background()); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if completed.Load() != 2 {
		t.Fatalf("completed roots = %d, want managed build and fuzz roots", completed.Load())
	}
}

func TestAnalyzeSeparatesCleanupEligibleGoCacheBytes(t *testing.T) {
	home := t.TempDir()
	settings := storage.DefaultSettings()
	settings.GoCache.Enabled = true
	provider := New(Config{
		HomeDir: home, TrashDir: filepath.Join(home, "trash"),
		Settings: staticSettings{settings: settings},
	})
	env, err := provider.ExecutionEnvironment(context.Background())
	if err != nil {
		t.Fatalf("ExecutionEnvironment: %v", err)
	}
	t.Setenv("GOCACHE", env["GOCACHE"])
	if err := os.WriteFile(filepath.Join(env["GOCACHE"], "compiled"), []byte("build"), 0o600); err != nil {
		t.Fatal(err)
	}
	fuzzSeed := filepath.Join(env["GOCACHE"], fuzzDirectoryName, "FuzzDecode", "seed")
	if err := os.MkdirAll(filepath.Dir(fuzzSeed), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fuzzSeed, []byte("corpus"), 0o600); err != nil {
		t.Fatal(err)
	}

	analysis, err := provider.Analyze(context.Background())
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if analysis.SizeBytes != int64(len("build")+len("corpus")) {
		t.Fatalf("physical size = %d, want build plus fuzz bytes", analysis.SizeBytes)
	}
	payload, err := json.Marshal(analysis)
	if err != nil {
		t.Fatalf("marshal analysis: %v", err)
	}
	var summary map[string]any
	if err := json.Unmarshal(payload, &summary); err != nil {
		t.Fatalf("unmarshal analysis: %v", err)
	}
	if eligible, ok := summary["cleanup_eligible_size_bytes"]; !ok || eligible != float64(len("build")) {
		t.Fatalf("cleanup-eligible bytes = %v, want %d", summary["cleanup_eligible_size_bytes"], len("build"))
	}
}

func TestAnalyzeExcludesNestedMountsFromGoCacheUsage(t *testing.T) {
	home := t.TempDir()
	settings := storage.DefaultSettings()
	settings.GoCache.Enabled = true
	provider := New(Config{
		HomeDir: home, TrashDir: filepath.Join(home, "trash"),
		Settings: staticSettings{settings: settings},
	})
	env, err := provider.ExecutionEnvironment(context.Background())
	if err != nil {
		t.Fatalf("ExecutionEnvironment: %v", err)
	}
	t.Setenv("GOCACHE", env["GOCACHE"])
	if err := os.WriteFile(filepath.Join(env["GOCACHE"], "compiled"), []byte("build"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(env["GOCACHE"], "fuzz", "FuzzDecode"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(env["GOCACHE"], "fuzz", "FuzzDecode", "seed"), []byte("corpus"), 0o600); err != nil {
		t.Fatal(err)
	}
	mountedPath := filepath.Join(env["GOCACHE"], "mounted")
	if err := os.MkdirAll(mountedPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mountedPath, "foreign"), []byte("external mounted data"), 0o600); err != nil {
		t.Fatal(err)
	}
	provider.config.mountID = func(path string) (string, error) {
		relative, err := filepath.Rel(env["GOCACHE"], path)
		if err != nil {
			return "", err
		}
		if relative == "mounted" || strings.HasPrefix(relative, "mounted"+string(filepath.Separator)) {
			return "simulated-bind-mount", nil
		}
		return "cache-mount", nil
	}

	analysis, err := provider.Analyze(context.Background())
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if analysis.SizeBytes != int64(len("build")+len("corpus")) {
		t.Fatalf("cache size includes mounted data: %d", analysis.SizeBytes)
	}
	if analysis.CleanupEligibleSizeBytes == nil || *analysis.CleanupEligibleSizeBytes != int64(len("build")) {
		t.Fatalf("cleanup-eligible size includes mounted or fuzz data: %#v", analysis.CleanupEligibleSizeBytes)
	}
}

func TestAnalyzeKeepsProgressMonotonicAcrossManagedAndUnmanagedCaches(t *testing.T) {
	home := t.TempDir()
	unmanagedPath := filepath.Join(t.TempDir(), "go-build")
	if err := os.MkdirAll(unmanagedPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unmanagedPath, "unmanaged"), []byte("unmanaged"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOCACHE", unmanagedPath)
	settings := storage.DefaultSettings()
	settings.GoCache.Enabled = true
	provider := New(Config{
		HomeDir: home, TrashDir: filepath.Join(home, "trash"),
		Settings: staticSettings{settings: settings}, Scanner: filescan.NewLimiter(1),
	})
	env, err := provider.ExecutionEnvironment(context.Background())
	if err != nil {
		t.Fatalf("ExecutionEnvironment: %v", err)
	}
	if err := os.WriteFile(filepath.Join(env["GOCACHE"], "managed"), []byte("managed"), 0o600); err != nil {
		t.Fatal(err)
	}
	var progressMu sync.Mutex
	var bytesScanned []int64
	provider.config.OnProgress = func(progress filescan.Progress) {
		progressMu.Lock()
		bytesScanned = append(bytesScanned, progress.BytesScanned)
		progressMu.Unlock()
	}

	if _, err := provider.Analyze(context.Background()); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	progressMu.Lock()
	defer progressMu.Unlock()
	for index := 1; index < len(bytesScanned); index++ {
		if bytesScanned[index] < bytesScanned[index-1] {
			t.Fatalf("progress bytes regressed at %d: %v", index, bytesScanned)
		}
	}
}

func TestMeasurementRootsIncludeAdoptedAndUnmanagedCaches(t *testing.T) {
	home := t.TempDir()
	adoptedPath := filepath.Join(t.TempDir(), "adopted-go-build")
	unmanagedPath := filepath.Join(t.TempDir(), "user-go-build")
	settings := storage.DefaultSettings()
	settings.GoCache.AdoptedPath = adoptedPath
	t.Setenv("GOCACHE", unmanagedPath)
	provider := New(Config{HomeDir: home})

	roots, err := provider.MeasurementRoots(settings)
	if err != nil {
		t.Fatalf("MeasurementRoots: %v", err)
	}
	want := []string{adoptedPath, unmanagedPath}
	if len(roots) != len(want) {
		t.Fatalf("roots = %v, want %v", roots, want)
	}
	for index := range want {
		if roots[index] != want[index] {
			t.Fatalf("roots = %v, want %v", roots, want)
		}
	}
}
