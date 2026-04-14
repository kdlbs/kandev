package process

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types"
)

// makeUntrackedUpdate creates a GitStatusUpdate with untracked files.
func makeUntrackedUpdate(paths ...string) types.GitStatusUpdate {
	files := make(map[string]types.FileInfo, len(paths))
	untracked := make([]string, 0, len(paths))
	for _, p := range paths {
		files[p] = types.FileInfo{Status: fileStatusUntracked}
		untracked = append(untracked, p)
	}
	return types.GitStatusUpdate{
		Files:     files,
		Untracked: untracked,
	}
}

func TestEnrichUntrackedFileDiffs_SmallTextFile(t *testing.T) {
	dir := t.TempDir()
	content := "line1\nline2\nline3\n"
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(dir, log)
	update := makeUntrackedUpdate("hello.txt")

	wt.enrichUntrackedFileDiffs(context.Background(), &update)

	fi := update.Files["hello.txt"]
	if fi.Diff == "" {
		t.Fatal("expected diff to be populated for small text file")
	}
	if fi.Additions == 0 {
		t.Fatal("expected additions > 0")
	}
	if !strings.Contains(fi.Diff, "+line1") {
		t.Errorf("diff should contain +line1, got: %s", fi.Diff[:min(200, len(fi.Diff))])
	}
}

func TestEnrichUntrackedFileDiffs_SkipsBinaryFile(t *testing.T) {
	dir := t.TempDir()
	// Binary content: contains null bytes.
	binary := make([]byte, 1024)
	binary[100] = 0
	binary[200] = 0
	copy(binary, []byte("ELF"))
	if err := os.WriteFile(filepath.Join(dir, "app.bin"), binary, 0644); err != nil {
		t.Fatal(err)
	}

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(dir, log)
	update := makeUntrackedUpdate("app.bin")

	wt.enrichUntrackedFileDiffs(context.Background(), &update)

	fi := update.Files["app.bin"]
	if fi.Diff != "" {
		t.Error("expected binary file to be skipped, but diff was populated")
	}
	if fi.DiffSkipReason != "binary" {
		t.Errorf("expected DiffSkipReason=%q, got %q", "binary", fi.DiffSkipReason)
	}
}

func TestEnrichUntrackedFileDiffs_SkipsLargeFile(t *testing.T) {
	dir := t.TempDir()
	// Create a file larger than maxUntrackedFileSize (1 MB).
	large := make([]byte, maxUntrackedFileSize+1)
	for i := range large {
		large[i] = 'x'
	}
	if err := os.WriteFile(filepath.Join(dir, "big.dat"), large, 0644); err != nil {
		t.Fatal(err)
	}

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(dir, log)
	update := makeUntrackedUpdate("big.dat")

	wt.enrichUntrackedFileDiffs(context.Background(), &update)

	fi := update.Files["big.dat"]
	if fi.Diff != "" {
		t.Error("expected large file to be skipped, but diff was populated")
	}
	if fi.DiffSkipReason != "too_large" {
		t.Errorf("expected DiffSkipReason=%q, got %q", "too_large", fi.DiffSkipReason)
	}
}

func TestEnrichUntrackedFileDiffs_SkipsNonexistentFile(t *testing.T) {
	dir := t.TempDir()
	log := newTestLogger(t)
	wt := NewWorkspaceTracker(dir, log)
	update := makeUntrackedUpdate("does-not-exist.txt")

	// Should not panic or error.
	wt.enrichUntrackedFileDiffs(context.Background(), &update)

	fi := update.Files["does-not-exist.txt"]
	if fi.Diff != "" {
		t.Error("expected missing file to be skipped")
	}
}
