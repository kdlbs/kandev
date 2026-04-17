package process

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
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
	got := totalDiffBytes(update)
	if got != 8 {
		t.Errorf("totalDiffBytes = %d, want 8", got)
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
