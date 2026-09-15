package tempstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/system/storage/filescan"
)

type testMountReader struct {
	identity func(string) (string, error)
}

func (r testMountReader) Identity(path string) (string, error) {
	if r.identity == nil {
		return "mount", nil
	}
	return r.identity(path)
}

type dynamicMountReader struct {
	nested  string
	mounted bool
}

func (r *dynamicMountReader) Identity(path string) (string, error) {
	if r.mounted && pathWithin(r.nested, path) {
		return "nested", nil
	}
	return "root", nil
}

type testScanner struct {
	measure func(context.Context, []filescan.Root, filescan.MeasureOptions, func(filescan.Progress)) []filescan.Result
}

func (s testScanner) MeasureWithOptions(
	ctx context.Context,
	roots []filescan.Root,
	options filescan.MeasureOptions,
	notify func(filescan.Progress),
) []filescan.Result {
	return s.measure(ctx, roots, options, notify)
}

func TestAnalyzeTemporaryRootsScopesAndCollapsesAliases(t *testing.T) {
	effective := t.TempDir()
	if err := os.WriteFile(filepath.Join(effective, "owned"), []byte("owned"), 0o600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(effective, alias); err != nil {
		t.Fatal(err)
	}

	provider := NewProvider(Config{
		GOOS:    "linux",
		Scanner: filescan.NewLimiter(1),
		Mounts:  testMountReader{},
		RootResolver: func(context.Context) ([]RootCandidate, error) {
			return []RootCandidate{
				{RequestedPath: alias},
				{RequestedPath: effective},
			}, nil
		},
	})

	analysis, err := provider.Analyze(context.Background())
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if analysis.Status != StatusMeasured || analysis.SizeBytes == nil || *analysis.SizeBytes != int64(len("owned")) {
		t.Fatalf("analysis = %#v, want one measured root", analysis)
	}
	if analysis.IncludedInTotal {
		t.Fatal("temporary footprint contributes to classified total")
	}
	if len(analysis.Roots) != 1 {
		t.Fatalf("roots = %#v, want one collapsed effective root", analysis.Roots)
	}
	if len(analysis.Roots[0].Aliases) != 1 || analysis.Roots[0].Path != effective {
		t.Fatalf("collapsed root = %#v", analysis.Roots[0])
	}
}

func TestAnalyzeTemporaryRootsCollapsesNestedRootsRegardlessOfOrder(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "nested")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "parent"), []byte("parent"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "child"), []byte("child"), 0o600); err != nil {
		t.Fatal(err)
	}

	provider := NewProvider(Config{
		GOOS:   "linux",
		Mounts: testMountReader{},
		RootResolver: func(context.Context) ([]RootCandidate, error) {
			return []RootCandidate{
				{RequestedPath: child},
				{RequestedPath: parent},
			}, nil
		},
		Scanner: filescan.NewLimiter(1),
	})

	analysis, err := provider.Analyze(context.Background())
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if analysis.SizeBytes == nil || *analysis.SizeBytes != int64(len("parentchild")) {
		t.Fatalf("size = %v, want one parent traversal", analysis.SizeBytes)
	}
	if len(analysis.Roots) != 1 || len(analysis.Roots[0].Aliases) != 1 {
		t.Fatalf("roots = %#v, want nested child collapsed into parent", analysis.Roots)
	}
}

func TestAnalyzeTemporaryRefreshesNestedMountBoundaryBetweenScans(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "parent"), []byte("parent"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "child"), []byte("child"), 0o600); err != nil {
		t.Fatal(err)
	}

	mounts := &dynamicMountReader{nested: nested}
	provider := NewProvider(Config{
		GOOS:    "linux",
		Mounts:  mounts,
		Scanner: filescan.NewLimiter(1),
		RootResolver: func(context.Context) ([]RootCandidate, error) {
			return []RootCandidate{{RequestedPath: root}}, nil
		},
	})

	first, err := provider.Analyze(context.Background())
	if err != nil {
		t.Fatalf("first Analyze: %v", err)
	}
	if first.Status != StatusMeasured || first.SizeBytes == nil || *first.SizeBytes != int64(len("parentchild")) {
		t.Fatalf("first analysis = %#v, want complete parent and child footprint", first)
	}

	mounts.mounted = true
	second, err := provider.Analyze(context.Background())
	if err != nil {
		t.Fatalf("second Analyze: %v", err)
	}
	if second.Status != StatusPartial || second.SizeBytes == nil || *second.SizeBytes != int64(len("parent")) {
		t.Fatalf("second analysis = %#v, want parent-only partial footprint", second)
	}
	if len(second.Roots) != 1 || second.Roots[0].SkippedCount != 1 {
		t.Fatalf("second roots = %#v, want one skipped nested mount", second.Roots)
	}
}

func TestMountGuardFailsClosedWhenRootMountIsReplaced(t *testing.T) {
	root := t.TempDir()
	guard := mountGuard{
		path: root, identity: "original", mounts: testMountReader{
			identity: func(string) (string, error) { return "replacement", nil },
		},
	}
	if _, err := guard.shouldSkip(root, nil); err == nil {
		t.Fatal("root mount replacement was accepted")
	}
}

func TestAnalyzeTemporaryReportsPartialAndUnavailableWithoutZeroingBytes(t *testing.T) {
	available := t.TempDir()
	missing := filepath.Join(t.TempDir(), "missing")
	provider := NewProvider(Config{
		GOOS:   "linux",
		Mounts: testMountReader{},
		RootResolver: func(context.Context) ([]RootCandidate, error) {
			return []RootCandidate{
				{RequestedPath: available},
				{RequestedPath: missing},
			}, nil
		},
		Scanner: testScanner{measure: func(
			_ context.Context,
			roots []filescan.Root,
			_ filescan.MeasureOptions,
			_ func(filescan.Progress),
		) []filescan.Result {
			if len(roots) != 1 {
				t.Fatalf("scanner roots = %d, want one available root", len(roots))
			}
			return []filescan.Result{
				{Bytes: 17, Partial: true, SkippedCount: 2, Warnings: []string{"unreadable entry"}},
			}
		}},
	})

	analysis, err := provider.Analyze(context.Background())
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if analysis.Status != StatusPartial || analysis.SizeBytes == nil || *analysis.SizeBytes != 17 {
		t.Fatalf("analysis = %#v, want partial 17-byte footprint", analysis)
	}
	if len(analysis.Warnings) == 0 || analysis.Roots[0].SkippedCount != 2 {
		t.Fatalf("analysis warnings/partial root = %#v", analysis)
	}
	if analysis.Roots[1].Status != StatusUnavailable || analysis.Roots[1].SizeBytes != nil {
		t.Fatalf("unavailable root = %#v, want no size", analysis.Roots[1])
	}
}

func TestAnalyzeTemporaryMarksMissingOptionalRootNotApplicable(t *testing.T) {
	effective := t.TempDir()
	provider := NewProvider(Config{
		GOOS:   "linux",
		Mounts: testMountReader{},
		RootResolver: func(context.Context) ([]RootCandidate, error) {
			return []RootCandidate{
				{RequestedPath: effective},
				{RequestedPath: filepath.Join(t.TempDir(), "tmp"), Optional: true},
			}, nil
		},
		Scanner: filescan.NewLimiter(1),
	})

	analysis, err := provider.Analyze(context.Background())
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(analysis.Roots) != 2 || analysis.Roots[1].Status != StatusNotApplicable {
		t.Fatalf("roots = %#v, want optional missing root not applicable", analysis.Roots)
	}
}

func TestAnalyzeTemporaryHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	provider := NewProvider(Config{RootResolver: func(context.Context) ([]RootCandidate, error) {
		return []RootCandidate{{RequestedPath: t.TempDir()}}, nil
	}})
	if _, err := provider.Analyze(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Analyze error = %v, want context.Canceled", err)
	}
}

func TestAnalyzeTemporaryDeadlineReturnsPartialSample(t *testing.T) {
	root := t.TempDir()
	provider := NewProvider(Config{
		GOOS:     "linux",
		Mounts:   testMountReader{},
		Deadline: time.Millisecond,
		RootResolver: func(context.Context) ([]RootCandidate, error) {
			return []RootCandidate{{RequestedPath: root}}, nil
		},
		Scanner: testScanner{measure: func(
			ctx context.Context,
			_ []filescan.Root,
			_ filescan.MeasureOptions,
			_ func(filescan.Progress),
		) []filescan.Result {
			<-ctx.Done()
			return []filescan.Result{{Bytes: 17, Err: ctx.Err()}}
		}},
	})

	analysis, err := provider.Analyze(context.Background())
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if analysis.Status != StatusPartial || analysis.SizeBytes == nil || *analysis.SizeBytes != 17 {
		t.Fatalf("analysis = %#v, want partial 17-byte sample", analysis)
	}
	if analysis.Reason != "deadline" || analysis.Roots[0].Reason != "deadline" {
		t.Fatalf("analysis = %#v, want deadline reason", analysis)
	}
}
