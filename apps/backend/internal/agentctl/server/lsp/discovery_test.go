package lsp

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	sharedlsp "github.com/kandev/kandev/internal/lsp"
)

func TestDiscoverLanguagesRecognizesRegisteredSignals(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		language string
	}{
		{name: "typescript manifest", path: "tsconfig.json", language: "typescript"},
		{name: "javascript extension", path: "src/app.mjs", language: "typescript"},
		{name: "python manifest", path: "pyproject.toml", language: "python"},
		{name: "python extension", path: "src/main.py", language: "python"},
		{name: "go manifest", path: "go.mod", language: "go"},
		{name: "go extension", path: "cmd/main.go", language: "go"},
		{name: "rust manifest", path: "Cargo.toml", language: "rust"},
		{name: "rust extension", path: "src/lib.rs", language: "rust"},
		{name: "kotlin manifest", path: "settings.gradle.kts", language: "kotlin"},
		{name: "kotlin extension", path: "src/Main.kt", language: "kotlin"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeDiscoveryFile(t, root, test.path)

			result := DiscoverLanguages(context.Background(), root, nil)

			if !slices.Equal(result.Languages, []string{test.language}) {
				t.Fatalf("languages = %v, want %s", result.Languages, test.language)
			}
			if result.State != sharedlsp.DetectionComplete || result.Truncated || result.ScannedAt.IsZero() {
				t.Fatalf("discovery evidence = %#v", result)
			}
		})
	}
}

func TestDiscoveryIsDeterministicAcrossOrderedDuplicateRoots(t *testing.T) {
	root := t.TempDir()
	writeDiscoveryFile(t, root, "repo-b/src/Main.kt")
	writeDiscoveryFile(t, root, "repo-a/web/app.ts")
	writeDiscoveryFile(t, root, "repo-a/api/main.go")

	first := DiscoverLanguages(context.Background(), root, []string{"repo-b", "repo-a", "repo-b"})
	second := DiscoverLanguages(context.Background(), root, []string{"repo-a", "repo-b"})
	want := []string{"go", "kotlin", "typescript"}
	if !slices.Equal(first.Languages, want) || !slices.Equal(second.Languages, want) {
		t.Fatalf("languages first=%v second=%v want=%v", first.Languages, second.Languages, want)
	}
	if first.EntriesScanned != second.EntriesScanned {
		t.Fatalf("duplicate/ordered roots changed entry count: first=%d second=%d",
			first.EntriesScanned, second.EntriesScanned)
	}
}

func TestDiscoverLanguagesAtRootsSharesOneBoundedDeterministicScan(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	writeDiscoveryFile(t, rootA, "src/Main.kt")
	writeDiscoveryFile(t, rootB, "api/main.go")

	first := DiscoverLanguagesAtRoots(context.Background(), []string{rootB, rootA, rootB})
	second := DiscoverLanguagesAtRoots(context.Background(), []string{rootA, rootB})
	want := []string{"go", "kotlin"}
	if !slices.Equal(first.Languages, want) || !slices.Equal(second.Languages, want) {
		t.Fatalf("languages first=%v second=%v want=%v", first.Languages, second.Languages, want)
	}
	if first.EntriesScanned != second.EntriesScanned {
		t.Fatalf("root order/duplicates changed entry count: first=%d second=%d",
			first.EntriesScanned, second.EntriesScanned)
	}

	limited := discoverLanguagesAtRoots(context.Background(), []string{rootB, rootA}, discoveryOptions{
		maxDuration: time.Second, maxEntries: 2, maxDepth: 6,
	})
	if limited.EntriesScanned != 2 || !limited.Truncated ||
		!slices.Contains(limited.Reasons, DiscoveryReasonEntryLimit) {
		t.Fatalf("shared-budget result = %#v", limited)
	}
}

func TestDiscoverLanguagesAtRootsRejectsRelativeAndFilesystemRoots(t *testing.T) {
	result := DiscoverLanguagesAtRoots(context.Background(), []string{"relative", string(filepath.Separator)})

	if result.State != sharedlsp.DetectionUnavailable || !result.Truncated ||
		!slices.Contains(result.Reasons, DiscoveryReasonInvalidRoot) {
		t.Fatalf("unsafe-root result = %#v", result)
	}
}

func TestDiscoverySkipsIgnoredDirectories(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{
		"node_modules/pkg/index.ts",
		"target/generated.rs",
		".git/hooks/check.py",
		".gradle/cache/Main.kt",
		"vendor/example/main.go",
		"build/generated/settings.gradle.kts",
	} {
		writeDiscoveryFile(t, root, path)
	}

	result := DiscoverLanguages(context.Background(), root, nil)

	if len(result.Languages) != 0 || result.State != sharedlsp.DetectionComplete || result.Truncated {
		t.Fatalf("ignored discovery = %#v", result)
	}
}

func TestDiscoveryDoesNotFollowDirectorySymlinksOrEscapeRoot(t *testing.T) {
	root := t.TempDir()
	escape := t.TempDir()
	writeDiscoveryFile(t, escape, "Main.kt")
	if err := os.Symlink(escape, filepath.Join(root, "linked")); err != nil {
		t.Skipf("directory symlink unavailable: %v", err)
	}
	if err := os.Symlink(root, filepath.Join(root, "loop")); err != nil {
		t.Skipf("loop symlink unavailable: %v", err)
	}

	result := DiscoverLanguages(context.Background(), root, []string{"../" + filepath.Base(escape)})

	if len(result.Languages) != 0 {
		t.Fatalf("symlink/escape discovered languages: %#v", result)
	}
	if result.State != sharedlsp.DetectionPartial || !result.Truncated ||
		!slices.Contains(result.Reasons, DiscoveryReasonInvalidRoot) {
		t.Fatalf("symlink/escape evidence = %#v", result)
	}
}

func TestDiscoveryEntryAndDepthBudgetsReturnPartialEvidence(t *testing.T) {
	t.Run("entry budget", func(t *testing.T) {
		root := t.TempDir()
		writeDiscoveryFile(t, root, "a.txt")
		writeDiscoveryFile(t, root, "b.txt")
		writeDiscoveryFile(t, root, "z.kt")

		result := discoverLanguages(context.Background(), root, nil, discoveryOptions{
			maxDuration: time.Second, maxEntries: 2, maxDepth: 6,
		})

		if result.EntriesScanned != 2 || !result.Truncated ||
			!slices.Contains(result.Reasons, DiscoveryReasonEntryLimit) || len(result.Languages) != 0 {
			t.Fatalf("entry-limited result = %#v", result)
		}
	})

	t.Run("depth budget", func(t *testing.T) {
		root := t.TempDir()
		writeDiscoveryFile(t, root, "one/two/Main.kt")

		result := discoverLanguages(context.Background(), root, nil, discoveryOptions{
			maxDuration: time.Second, maxEntries: 100, maxDepth: 1,
		})

		if !result.Truncated || !slices.Contains(result.Reasons, DiscoveryReasonDepthLimit) ||
			len(result.Languages) != 0 {
			t.Fatalf("depth-limited result = %#v", result)
		}
	})
}

func TestDiscoveryCancellationIsBoundedAndTruthful(t *testing.T) {
	root := t.TempDir()
	writeDiscoveryFile(t, root, "Main.kt")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	started := time.Now()
	result := DiscoverLanguages(ctx, root, nil)

	if time.Since(started) > 100*time.Millisecond {
		t.Fatalf("canceled discovery took too long: %s", time.Since(started))
	}
	if result.State != sharedlsp.DetectionUnavailable || !result.Truncated ||
		!slices.Contains(result.Reasons, DiscoveryReasonCanceled) {
		t.Fatalf("canceled result = %#v", result)
	}
}

func TestDiscoveryExpiredDeadlineIsDistinguishedFromCancellation(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	result := DiscoverLanguages(ctx, root, nil)

	if result.State != sharedlsp.DetectionUnavailable || !result.Truncated ||
		!slices.Contains(result.Reasons, DiscoveryReasonDeadline) ||
		slices.Contains(result.Reasons, DiscoveryReasonCanceled) {
		t.Fatalf("deadline result = %#v", result)
	}
}

func TestDiscoveryEmptyAndUnreadableRoots(t *testing.T) {
	root := t.TempDir()
	empty := DiscoverLanguages(context.Background(), root, nil)
	if len(empty.Languages) != 0 || empty.State != sharedlsp.DetectionComplete || empty.Truncated {
		t.Fatalf("empty result = %#v", empty)
	}

	missing := DiscoverLanguages(context.Background(), filepath.Join(root, "missing"), nil)
	if missing.State != sharedlsp.DetectionUnavailable || !missing.Truncated ||
		!slices.Contains(missing.Reasons, DiscoveryReasonUnreadable) {
		t.Fatalf("missing result = %#v", missing)
	}
}

func writeDiscoveryFile(t *testing.T, root, relative string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("signal only"), 0o600); err != nil {
		t.Fatal(err)
	}
}
