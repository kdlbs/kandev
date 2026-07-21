package process

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

func TestIsBinaryContent(t *testing.T) {
	tests := []struct {
		name   string
		data   []byte
		binary bool
	}{
		{"empty", []byte{}, false},
		{"text", []byte("hello world\n"), false},
		{"utf8", []byte("héllo wörld\n"), false},
		{"null byte", []byte("hello\x00world"), true},
		{"null at start", []byte{0, 'h', 'i'}, true},
		{"ELF header", []byte{0x7f, 'E', 'L', 'F', 0, 0}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isBinaryContent(tt.data); got != tt.binary {
				t.Errorf("isBinaryContent(%q) = %v, want %v", tt.data, got, tt.binary)
			}
		})
	}
}

func TestTotalDiffBytes(t *testing.T) {
	update := &streams.GitStatusUpdate{
		Files: map[string]streams.FileInfo{
			"a.go": {Diff: "abc"},
			"b.go": {Diff: "defgh"},
			"c.go": {Diff: ""},
		},
	}
	got, err := totalDiffBytes(context.Background(), update)
	if err != nil {
		t.Fatalf("totalDiffBytes() error = %v", err)
	}
	if got != 8 {
		t.Errorf("totalDiffBytes = %d, want 8", got)
	}
}

func TestDiffBudget_TracksReplacedContentAtBoundary(t *testing.T) {
	update := &streams.GitStatusUpdate{
		Files: map[string]streams.FileInfo{
			"existing.go": {Diff: strings.Repeat("x", maxTotalDiffBytes-1)},
		},
	}
	budget, err := newDiffBudget(context.Background(), update)
	if err != nil {
		t.Fatalf("newDiffBudget() error = %v", err)
	}

	if budget.exhausted() {
		t.Fatal("budget exhausted before reaching the total limit")
	}
	budget.replace("", "x")
	if !budget.exhausted() {
		t.Fatal("budget not exhausted at the total limit")
	}
	budget.replace("x", "")
	if budget.exhausted() {
		t.Fatal("budget remained exhausted after replacing content with an empty diff")
	}
}

func TestCapDiffOutput_Truncation(t *testing.T) {
	isolateTestGitEnv(t)
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create a file with content larger than maxDiffOutputSize
	bigContent := strings.Repeat("x", maxDiffOutputSize+1000)
	writeFile(t, repoDir, "big.txt", bigContent)
	runGit(t, repoDir, "add", "big.txt")

	// Get diff — should be truncated
	out, truncated := capDiffOutput(context.Background(), repoDir, "diff", "--cached", "--", "big.txt")
	if !truncated {
		t.Error("expected truncated=true for large diff")
	}
	if len(out) > maxDiffOutputSize {
		t.Errorf("output len=%d exceeds maxDiffOutputSize=%d", len(out), maxDiffOutputSize)
	}
}

func TestEnrichUntrackedFileDiffs_TooLarge(t *testing.T) {
	isolateTestGitEnv(t)
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create a file just over the size limit
	bigPath := filepath.Join(repoDir, "huge.bin")
	f, err := os.Create(bigPath)
	if err != nil {
		t.Fatal(err)
	}
	// Write maxDiffFileSize + 1 bytes (all 'A')
	buf := make([]byte, 1024*1024) // 1MB chunks
	for i := range buf {
		buf[i] = 'A'
	}
	written := int64(0)
	for written < maxDiffFileSize+1 {
		chunk := buf
		if remaining := maxDiffFileSize + 1 - written; remaining < int64(len(chunk)) {
			chunk = chunk[:remaining]
		}
		n, err := f.Write(chunk)
		if err != nil {
			_ = f.Close()
			t.Fatal(err)
		}
		written += int64(n)
	}
	_ = f.Close()

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)
	update := &streams.GitStatusUpdate{
		Files: map[string]streams.FileInfo{
			"huge.bin": {Path: "huge.bin", Status: "untracked"},
		},
	}

	wt.enrichUntrackedFileDiffs(context.Background(), update)

	fi := update.Files["huge.bin"]
	if fi.DiffSkipReason != diffSkipReasonTooLarge {
		t.Errorf("DiffSkipReason = %q, want %q", fi.DiffSkipReason, diffSkipReasonTooLarge)
	}
	if fi.Diff != "" {
		t.Error("expected empty Diff for too-large file")
	}
}

func TestEnrichUntrackedFileDiffs_Binary(t *testing.T) {
	isolateTestGitEnv(t)
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create a binary file (contains null bytes)
	binPath := filepath.Join(repoDir, "image.png")
	binContent := []byte("PNG\x00\x00\x00fake binary content")
	if err := os.WriteFile(binPath, binContent, 0644); err != nil {
		t.Fatal(err)
	}

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)
	update := &streams.GitStatusUpdate{
		Files: map[string]streams.FileInfo{
			"image.png": {Path: "image.png", Status: "untracked"},
		},
	}

	wt.enrichUntrackedFileDiffs(context.Background(), update)

	fi := update.Files["image.png"]
	if fi.DiffSkipReason != diffSkipReasonBinary {
		t.Errorf("DiffSkipReason = %q, want %q", fi.DiffSkipReason, diffSkipReasonBinary)
	}
	if fi.Diff != "" {
		t.Error("expected empty Diff for binary file")
	}
}

func TestEnrichUntrackedFileDiffs_BudgetExceeded(t *testing.T) {
	isolateTestGitEnv(t)
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create files large enough that their synthetic diffs exceed maxTotalDiffBytes.
	// Each file's diff includes headers + "+line\n" per line, so content of ~maxDiffOutputSize
	// per file ensures we blow through the 2MB budget in a few files.
	fileCount := 20
	contentPerFile := maxDiffOutputSize / 2
	files := make(map[string]streams.FileInfo)

	for i := 0; i < fileCount; i++ {
		name := filepath.Join(repoDir, strings.Repeat("a", i+1)+".txt")
		content := strings.Repeat("x\n", contentPerFile/2)
		if err := os.WriteFile(name, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		relName := filepath.Base(name)
		files[relName] = streams.FileInfo{Path: relName, Status: "untracked"}
	}

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)
	update := &streams.GitStatusUpdate{Files: files}

	wt.enrichUntrackedFileDiffs(context.Background(), update)

	budgetExceededCount := 0
	for _, fi := range update.Files {
		if fi.DiffSkipReason == diffSkipReasonBudgetExceeded {
			budgetExceededCount++
		}
	}
	if budgetExceededCount == 0 {
		t.Error("expected at least one file with budget_exceeded skip reason")
	}
}

func TestEnrichUntrackedFileDiffs_SmallTextFile(t *testing.T) {
	isolateTestGitEnv(t)
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	txtPath := filepath.Join(repoDir, "hello.txt")
	if err := os.WriteFile(txtPath, []byte("hello world\n"), 0644); err != nil {
		t.Fatal(err)
	}

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)
	update := &streams.GitStatusUpdate{
		Files: map[string]streams.FileInfo{
			"hello.txt": {Path: "hello.txt", Status: "untracked"},
		},
	}

	wt.enrichUntrackedFileDiffs(context.Background(), update)

	fi := update.Files["hello.txt"]
	if fi.Diff == "" {
		t.Error("expected non-empty Diff for small text file")
	}
	if fi.DiffSkipReason != "" {
		t.Errorf("expected empty DiffSkipReason, got %q", fi.DiffSkipReason)
	}
	if fi.Additions != 1 {
		t.Errorf("Additions = %d, want 1", fi.Additions)
	}
}

func TestEnrichUntrackedFileDiffs_CanceledContextStopsBeforeEnrichment(t *testing.T) {
	isolateTestGitEnv(t)
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	writeFile(t, repoDir, "first.txt", "must not be read\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	wt := NewWorkspaceTracker(repoDir, newTestLogger(t))
	update := &streams.GitStatusUpdate{
		Files: map[string]streams.FileInfo{
			"first.txt": {Path: "first.txt", Status: fileStatusUntracked},
		},
	}

	err := wt.enrichUntrackedFileDiffs(ctx, update)

	got := update.Files["first.txt"]
	if err != context.Canceled {
		t.Fatalf("enrichUntrackedFileDiffs error = %v, want %v", err, context.Canceled)
	}
	if got.Diff != "" || got.Additions != 0 || got.DiffSkipReason != "" {
		t.Fatalf("canceled enrichment mutated file info: %+v", got)
	}
}

func TestEnrichWithDiffData_CanceledContextReturnsErrorForTrackedFiles(t *testing.T) {
	isolateTestGitEnv(t)
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	wt := NewWorkspaceTracker(repoDir, newTestLogger(t))
	update := &streams.GitStatusUpdate{
		Files: map[string]streams.FileInfo{
			"tracked.txt": {Path: "tracked.txt", Status: "modified"},
		},
	}

	err := wt.enrichWithDiffData(ctx, update, streams.GitStatusUpdate{})
	if err != context.Canceled {
		t.Fatalf("enrichWithDiffData error = %v, want %v", err, context.Canceled)
	}
}

type cancelAfterErrChecksContext struct {
	context.Context
	remaining int
	cancel    context.CancelFunc
}

func (c *cancelAfterErrChecksContext) Err() error {
	c.remaining--
	if c.remaining <= 0 {
		c.cancel()
	}
	return c.Context.Err()
}

func TestBuildUntrackedDiff_CancellationDuringLineConstruction(t *testing.T) {
	baseCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ctx := &cancelAfterErrChecksContext{Context: baseCtx, remaining: 2, cancel: cancel}

	diff, additions, truncated, err := buildUntrackedDiff(ctx, "lines.txt", []byte("first\nsecond\nthird\n"))

	if err != context.Canceled {
		t.Fatalf("buildUntrackedDiff error = %v, want %v", err, context.Canceled)
	}
	if diff != "" || additions != 0 || truncated {
		t.Fatalf("canceled diff = %q, additions = %d, truncated = %v", diff, additions, truncated)
	}
}

func TestBranchDiffTotals_CancellationStopsNumstatLoop(t *testing.T) {
	baseCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ctx := &cancelAfterErrChecksContext{Context: baseCtx, remaining: 2, cancel: cancel}

	additions, deletions, err := branchDiffTotals(
		ctx,
		[]byte("1\t2\tfirst.txt\n3\t4\tsecond.txt\n5\t6\tthird.txt\n"),
		nil,
	)

	if err != context.Canceled {
		t.Fatalf("branchDiffTotals() error = %v, want %v", err, context.Canceled)
	}
	if additions != 0 || deletions != 0 {
		t.Fatalf("canceled totals = (%d, %d), want no publishable partial result", additions, deletions)
	}
}

func TestDiffBudgetAndCarryForwardHonorCancellation(t *testing.T) {
	files := map[string]streams.FileInfo{
		"first.go":  {Path: "first.go", Diff: "first"},
		"second.go": {Path: "second.go", Diff: "second"},
		"third.go":  {Path: "third.go", Diff: "third"},
	}

	baseCtx, cancel := context.WithCancel(context.Background())
	ctx := &cancelAfterErrChecksContext{Context: baseCtx, remaining: 2, cancel: cancel}
	if _, err := newDiffBudget(ctx, &streams.GitStatusUpdate{Files: files}); err != context.Canceled {
		t.Fatalf("newDiffBudget() error = %v, want %v", err, context.Canceled)
	}

	baseCtx, cancel = context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ctx = &cancelAfterErrChecksContext{Context: baseCtx, remaining: 2, cancel: cancel}
	update := &streams.GitStatusUpdate{
		HeadCommit: "head",
		Files: map[string]streams.FileInfo{
			"first.go": {}, "second.go": {}, "third.go": {},
		},
	}
	prior := streams.GitStatusUpdate{HeadCommit: "head", Files: files}
	if err := carryForwardFileDiffs(ctx, update, prior); err != context.Canceled {
		t.Fatalf("carryForwardFileDiffs() error = %v, want %v", err, context.Canceled)
	}
}

func TestEnrichUntrackedFileDiffs_CancellationAfterFirstFileLeavesRemainingFilesUntouched(t *testing.T) {
	isolateTestGitEnv(t)
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	files := make(map[string]streams.FileInfo, 3)
	for _, path := range []string{"first.txt", "second.txt", "third.txt"} {
		writeFile(t, repoDir, path, path+" content\n")
		files[path] = streams.FileInfo{Path: path, Status: fileStatusUntracked}
	}

	ctx, cancel := context.WithCancel(context.Background())
	wt := NewWorkspaceTracker(repoDir, newTestLogger(t))
	update := &streams.GitStatusUpdate{Files: files}
	budget, err := newDiffBudget(ctx, update)
	if err != nil {
		t.Fatalf("newDiffBudget() error = %v", err)
	}
	readCount := 0
	enrichFile := func(ctx context.Context, path string, fileInfo streams.FileInfo) (streams.FileInfo, error) {
		enriched, err := wt.enrichUntrackedFile(ctx, path, fileInfo)
		if err == nil {
			readCount++
			cancel()
		}
		return enriched, err
	}

	err = enrichUntrackedFileDiffs(ctx, update, &budget, enrichFile)

	if err != context.Canceled {
		t.Fatalf("enrichUntrackedFileDiffs error = %v, want %v", err, context.Canceled)
	}
	if readCount != 1 {
		t.Fatalf("filesystem enrichments = %d, want 1", readCount)
	}
	enrichedCount := 0
	untouchedCount := 0
	for _, fileInfo := range update.Files {
		if fileInfo.Diff != "" && fileInfo.Additions == 1 {
			enrichedCount++
			continue
		}
		if fileInfo.Diff == "" && fileInfo.Additions == 0 && fileInfo.DiffSkipReason == "" {
			untouchedCount++
		}
	}
	if enrichedCount != 1 || untouchedCount != 2 {
		t.Fatalf("enriched files = %d, untouched files = %d; want 1 and 2", enrichedCount, untouchedCount)
	}
}

func BenchmarkEnrichUntrackedFileDiffs(b *testing.B) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		b.Fatalf("create logger: %v", err)
	}

	for _, fileCount := range []int{1_000, 10_000} {
		b.Run(fmt.Sprintf("files-%d", fileCount), func(b *testing.B) {
			files := make(map[string]streams.FileInfo, fileCount+1)
			files["existing.go"] = streams.FileInfo{Diff: strings.Repeat("x", maxTotalDiffBytes)}
			for i := 0; i < fileCount; i++ {
				path := fmt.Sprintf("cache/%05d", i)
				files[path] = streams.FileInfo{Path: path, Status: fileStatusUntracked}
			}

			wt := NewWorkspaceTracker(b.TempDir(), log)
			update := &streams.GitStatusUpdate{Files: files}
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if err := wt.enrichUntrackedFileDiffs(context.Background(), update); err != nil {
					b.Fatalf("enrich untracked files: %v", err)
				}
			}
		})
	}
}

func TestResolveNumstatPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"plain path", "src/main.go", "src/main.go"},
		{"simple rename", "old.txt => new.txt", "new.txt"},
		{"brace rename same dir", "{old.txt => new.txt}", "new.txt"},
		{"brace rename with prefix", "src/{old.go => new.go}", "src/new.go"},
		{"brace rename with suffix", "{old => new}/file.go", "new/file.go"},
		{"brace rename with prefix and suffix", "src/{v1 => v2}/handler.go", "src/v2/handler.go"},
		{"directory rename", "pkg/{old => new}/main.go", "pkg/new/main.go"},
		{"path with spaces", "my dir/file.txt", "my dir/file.txt"},
		{"rename with spaces", "old file.txt => new file.txt", "new file.txt"},
		// LastIndex on `{` ensures git's own brace is picked, not the filename's leading `{`.
		{"filename starts with brace", "{{page => layout}}.svelte", "{layout}.svelte"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveNumstatPath(tt.input)
			if got != tt.expected {
				t.Errorf("resolveNumstatPath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestEnrichStagedDiff_RenamedFile(t *testing.T) {
	isolateTestGitEnv(t)
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create and commit a file, then rename + modify it
	writeFile(t, repoDir, "original.txt", "line1\nline2\nline3\n")
	runGit(t, repoDir, "add", "original.txt")
	runGit(t, repoDir, "commit", "-m", "add original")

	runGit(t, repoDir, "mv", "original.txt", "renamed.txt")
	writeFile(t, repoDir, "renamed.txt", "line1\nline2 modified\nline3\nline4\n")
	runGit(t, repoDir, "add", "renamed.txt")

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)
	update := &streams.GitStatusUpdate{
		Files: map[string]streams.FileInfo{
			"renamed.txt": {
				Path:    "renamed.txt",
				Status:  "renamed",
				Staged:  true,
				OldPath: "original.txt",
			},
		},
	}

	wt.enrichWithStagedDiff(context.Background(), update, "HEAD", streams.GitStatusUpdate{})

	fi := update.Files["renamed.txt"]
	if fi.Diff == "" {
		t.Error("expected non-empty Diff for renamed+modified file")
	}
	if fi.Additions == 0 && fi.Deletions == 0 {
		t.Error("expected non-zero additions or deletions for renamed+modified file")
	}
}

// TestCarryBranchDiff covers carry-forward of branch additions/deletions when
// the `git diff --numstat <merge-base>` call fails — without it, a transient
// timeout would clear the sidebar's branch-totals count.
func TestCarryBranchDiff(t *testing.T) {
	head := "abc123"
	base := "base000"
	tests := []struct {
		name          string
		prior         streams.GitStatusUpdate
		updateHead    string
		updateBase    string
		wantAdditions int
		wantDeletions int
	}{
		{
			name:          "same head and base preserves totals",
			prior:         streams.GitStatusUpdate{HeadCommit: head, BaseCommit: base, BranchAdditions: 42, BranchDeletions: 7},
			updateHead:    head,
			updateBase:    base,
			wantAdditions: 42,
			wantDeletions: 7,
		},
		{
			name:          "different head drops totals",
			prior:         streams.GitStatusUpdate{HeadCommit: head, BaseCommit: base, BranchAdditions: 42, BranchDeletions: 7},
			updateHead:    "def456",
			updateBase:    base,
			wantAdditions: 0,
			wantDeletions: 0,
		},
		{
			name:          "different base drops totals",
			prior:         streams.GitStatusUpdate{HeadCommit: head, BaseCommit: base, BranchAdditions: 42, BranchDeletions: 7},
			updateHead:    head,
			updateBase:    "newbase",
			wantAdditions: 0,
			wantDeletions: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			update := &streams.GitStatusUpdate{HeadCommit: tt.updateHead, BaseCommit: tt.updateBase}
			carryBranchDiff(update, tt.prior)
			if update.BranchAdditions != tt.wantAdditions || update.BranchDeletions != tt.wantDeletions {
				t.Errorf("additions/deletions = %d/%d, want %d/%d",
					update.BranchAdditions, update.BranchDeletions, tt.wantAdditions, tt.wantDeletions)
			}
		})
	}
}

// TestCarryForwardFileDiffs covers per-file diff carry-forward. The numstat
// command is the gate that drives per-file enrichment; when it fails we keep
// the prior diff content visible (as long as HEAD hasn't moved) instead of
// clearing the diff panel on a transient timeout.
func TestCarryForwardFileDiffs(t *testing.T) {
	head := "abc123"
	priorDiff := "diff --git a/foo b/foo\n+new line\n"

	t.Run("preserves diff additions deletions when head matches", func(t *testing.T) {
		prior := streams.GitStatusUpdate{
			HeadCommit: head,
			Files: map[string]streams.FileInfo{
				"foo.go": {Path: "foo.go", Diff: priorDiff, Additions: 5, Deletions: 2, DiffSkipReason: ""},
			},
		}
		update := &streams.GitStatusUpdate{
			HeadCommit: head,
			Files: map[string]streams.FileInfo{
				"foo.go": {Path: "foo.go"},
			},
		}
		if err := carryForwardFileDiffs(context.Background(), update, prior); err != nil {
			t.Fatalf("carryForwardFileDiffs() error = %v", err)
		}
		got := update.Files["foo.go"]
		if got.Diff != priorDiff {
			t.Errorf("Diff = %q, want %q", got.Diff, priorDiff)
		}
		if got.Additions != 5 || got.Deletions != 2 {
			t.Errorf("Additions/Deletions = %d/%d, want 5/2", got.Additions, got.Deletions)
		}
	})

	t.Run("skips when head moved", func(t *testing.T) {
		prior := streams.GitStatusUpdate{
			HeadCommit: head,
			Files: map[string]streams.FileInfo{
				"foo.go": {Path: "foo.go", Diff: priorDiff, Additions: 5, Deletions: 2},
			},
		}
		update := &streams.GitStatusUpdate{
			HeadCommit: "different",
			Files: map[string]streams.FileInfo{
				"foo.go": {Path: "foo.go"},
			},
		}
		if err := carryForwardFileDiffs(context.Background(), update, prior); err != nil {
			t.Fatalf("carryForwardFileDiffs() error = %v", err)
		}
		got := update.Files["foo.go"]
		if got.Diff != "" || got.Additions != 0 || got.Deletions != 0 {
			t.Errorf("expected zeroed FileInfo on head mismatch, got %+v", got)
		}
	})

	t.Run("skips files not in prior", func(t *testing.T) {
		prior := streams.GitStatusUpdate{HeadCommit: head, Files: map[string]streams.FileInfo{}}
		update := &streams.GitStatusUpdate{
			HeadCommit: head,
			Files:      map[string]streams.FileInfo{"new.go": {Path: "new.go"}},
		}
		if err := carryForwardFileDiffs(context.Background(), update, prior); err != nil {
			t.Fatalf("carryForwardFileDiffs() error = %v", err)
		}
		got := update.Files["new.go"]
		if got.Diff != "" {
			t.Errorf("expected no carry-forward for new file, got Diff=%q", got.Diff)
		}
	})

	t.Run("does not overwrite already-populated update entry", func(t *testing.T) {
		newDiff := "diff --git a/foo b/foo\n+brand new\n"
		prior := streams.GitStatusUpdate{
			HeadCommit: head,
			Files: map[string]streams.FileInfo{
				"foo.go": {Path: "foo.go", Diff: priorDiff, Additions: 5, Deletions: 2},
			},
		}
		update := &streams.GitStatusUpdate{
			HeadCommit: head,
			Files: map[string]streams.FileInfo{
				"foo.go": {Path: "foo.go", Diff: newDiff, Additions: 1, Deletions: 0},
			},
		}
		if err := carryForwardFileDiffs(context.Background(), update, prior); err != nil {
			t.Fatalf("carryForwardFileDiffs() error = %v", err)
		}
		got := update.Files["foo.go"]
		if got.Diff != newDiff {
			t.Errorf("Diff overwritten; got %q, want %q", got.Diff, newDiff)
		}
		if got.Additions != 1 || got.Deletions != 0 {
			t.Errorf("counts overwritten; got %d/%d, want 1/0", got.Additions, got.Deletions)
		}
	})

	t.Run("respects skip reason set this poll", func(t *testing.T) {
		skipReasons := []string{
			diffSkipReasonBudgetExceeded,
			diffSkipReasonTooLarge,
			diffSkipReasonBinary,
		}
		for _, reason := range skipReasons {
			t.Run(reason, func(t *testing.T) {
				prior := streams.GitStatusUpdate{
					HeadCommit: head,
					Files: map[string]streams.FileInfo{
						"foo.go": {Path: "foo.go", Diff: priorDiff, Additions: 5, Deletions: 2},
					},
				}
				update := &streams.GitStatusUpdate{
					HeadCommit: head,
					Files: map[string]streams.FileInfo{
						"foo.go": {Path: "foo.go", DiffSkipReason: reason},
					},
				}
				if err := carryForwardFileDiffs(context.Background(), update, prior); err != nil {
					t.Fatalf("carryForwardFileDiffs() error = %v", err)
				}
				got := update.Files["foo.go"]
				if got.Diff != "" {
					t.Errorf("Diff carried forward despite skip reason %q; got %q", reason, got.Diff)
				}
				if got.DiffSkipReason != reason {
					t.Errorf("DiffSkipReason = %q, want %q", got.DiffSkipReason, reason)
				}
			})
		}
	})
}

func TestCarryForwardFileDiff(t *testing.T) {
	head := "abc123"
	priorDiff := "diff --git a/foo b/foo\n+kept line\n"

	t.Run("fills empty Diff from prior when head matches", func(t *testing.T) {
		prior := streams.GitStatusUpdate{
			HeadCommit: head,
			Files: map[string]streams.FileInfo{
				"foo.go": {Path: "foo.go", Diff: priorDiff, DiffSkipReason: "truncated"},
			},
		}
		update := &streams.GitStatusUpdate{
			HeadCommit: head,
			Files: map[string]streams.FileInfo{
				// numstat already populated additions/deletions; capDiffOutput then returned "".
				"foo.go": {Path: "foo.go", Additions: 7, Deletions: 3},
			},
		}
		fi := update.Files["foo.go"]
		fi = carryForwardFileDiff(fi, "foo.go", update, prior)
		if fi.Diff != priorDiff {
			t.Errorf("Diff = %q, want %q", fi.Diff, priorDiff)
		}
		if fi.DiffSkipReason != "truncated" {
			t.Errorf("DiffSkipReason = %q, want %q", fi.DiffSkipReason, "truncated")
		}
		if fi.Additions != 7 || fi.Deletions != 3 {
			t.Errorf("counts should be untouched, got %d/%d, want 7/3", fi.Additions, fi.Deletions)
		}
	})

	t.Run("no-op when head moved", func(t *testing.T) {
		prior := streams.GitStatusUpdate{
			HeadCommit: head,
			Files: map[string]streams.FileInfo{
				"foo.go": {Path: "foo.go", Diff: priorDiff},
			},
		}
		update := &streams.GitStatusUpdate{
			HeadCommit: "different",
			Files: map[string]streams.FileInfo{
				"foo.go": {Path: "foo.go"},
			},
		}
		fi := carryForwardFileDiff(update.Files["foo.go"], "foo.go", update, prior)
		if fi.Diff != "" {
			t.Errorf("expected no carry-forward on head mismatch, got Diff=%q", fi.Diff)
		}
	})

	t.Run("no-op when file absent from prior", func(t *testing.T) {
		prior := streams.GitStatusUpdate{HeadCommit: head, Files: map[string]streams.FileInfo{}}
		update := &streams.GitStatusUpdate{
			HeadCommit: head,
			Files:      map[string]streams.FileInfo{"new.go": {Path: "new.go"}},
		}
		fi := carryForwardFileDiff(update.Files["new.go"], "new.go", update, prior)
		if fi.Diff != "" {
			t.Errorf("expected no carry-forward for new file, got Diff=%q", fi.Diff)
		}
	})

	t.Run("no-op when prior diff empty", func(t *testing.T) {
		prior := streams.GitStatusUpdate{
			HeadCommit: head,
			Files: map[string]streams.FileInfo{
				"foo.go": {Path: "foo.go", Additions: 1, Deletions: 1},
			},
		}
		update := &streams.GitStatusUpdate{
			HeadCommit: head,
			Files:      map[string]streams.FileInfo{"foo.go": {Path: "foo.go"}},
		}
		fi := carryForwardFileDiff(update.Files["foo.go"], "foo.go", update, prior)
		if fi.Diff != "" || fi.DiffSkipReason != "" {
			t.Errorf("expected no carry-forward when prior Diff empty, got %+v", fi)
		}
	})

	t.Run("no-op when prior head empty", func(t *testing.T) {
		prior := streams.GitStatusUpdate{
			Files: map[string]streams.FileInfo{
				"foo.go": {Path: "foo.go", Diff: priorDiff},
			},
		}
		update := &streams.GitStatusUpdate{
			HeadCommit: head,
			Files:      map[string]streams.FileInfo{"foo.go": {Path: "foo.go"}},
		}
		fi := carryForwardFileDiff(update.Files["foo.go"], "foo.go", update, prior)
		if fi.Diff != "" {
			t.Errorf("expected no carry-forward on empty prior head, got Diff=%q", fi.Diff)
		}
	})
}
