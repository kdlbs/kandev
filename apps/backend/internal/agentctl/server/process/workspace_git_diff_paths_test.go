package process

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types"
)

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.36
func TestWorkspaceGitDetailsPreservesExactPaths(t *testing.T) {
	for _, path := range numstatExactPaths {
		for _, staged := range []bool{false, true} {
			layer := "unstaged"
			if staged {
				layer = "staged"
			}
			t.Run(layer+"/"+path, func(t *testing.T) {
				skipWindowsInvalidGitPath(t, path)
				dir := setupNumstatPathRepo(t, path, "original\n")
				writeFile(t, dir, path, "original\nexpected-new-line\n")
				if staged {
					runGit(t, dir, "add", "--", path)
				}
				file := readNumstatPathDetails(t, dir, path)
				if file.Additions != 1 || file.Deletions != 0 || !strings.Contains(file.Diff, "+expected-new-line") {
					t.Errorf("file %q: +%d -%d diff=%q; want +1 -0 and actual patch", path, file.Additions, file.Deletions, file.Diff)
				}
			})
		}
	}
}

var numstatExactPaths = []string{
	"plain.txt", "café.txt", "literal => name.txt", "{old => new}.txt",
	"trailing.txt ", " leading.txt", "quote\"name.txt", "back\\slash.txt",
	"tab\tname.txt", "line\nname.txt",
}

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.9, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.36
func TestWorkspaceGitDetailsPreservesExactMixedPaths(t *testing.T) {
	for _, path := range numstatExactPaths {
		t.Run(path, func(t *testing.T) {
			skipWindowsInvalidGitPath(t, path)
			dir := setupNumstatPathRepo(t, path, "base\n")
			writeFile(t, dir, path, "base\nSTAGED_MARKER\n")
			runGit(t, dir, "add", "--", path)
			writeFile(t, dir, path, "base\nSTAGED_MARKER\nUNSTAGED_MARKER\n")
			file := readNumstatPathDetails(t, dir, path)
			if file.Additions != 2 || file.Deletions != 0 || !strings.Contains(file.Diff, "+UNSTAGED_MARKER") {
				t.Errorf("combined file = %+v, want +2 -0 and combined patch", file)
			}
			assertNumstatFacet(t, file.StagedChange, 1, "STAGED_MARKER")
			assertNumstatFacet(t, file.UnstagedChange, 1, "UNSTAGED_MARKER")
			if file.StagedChange != nil && strings.Contains(file.StagedChange.Diff, "UNSTAGED_MARKER") {
				t.Errorf("staged patch contains worktree-only content: %q", file.StagedChange.Diff)
			}
		})
	}
}

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.36
func TestWorkspaceGitDetailsPreservesRenameDestinations(t *testing.T) {
	for _, paths := range [][2]string{{"old.txt", "new.txt"}, {"dir/old/file.txt", "dir/new/file.txt"}} {
		for _, change := range []string{"unchanged", "staged", "mixed"} {
			t.Run(paths[1]+"/"+change, func(t *testing.T) {
				testNumstatRename(t, paths[0], paths[1], change)
			})
		}
	}
}

func testNumstatRename(t *testing.T, oldPath, newPath, change string) {
	t.Helper()
	const base = "one\ntwo\nthree\nfour\nfive\nsix\n"
	dir := setupNumstatPathRepo(t, oldPath, base)
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, newPath)), 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "mv", "--", oldPath, newPath)
	additions := 0
	if change == "staged" {
		writeFile(t, dir, newPath, base+"RENAME_MARKER\n")
		runGit(t, dir, "add", "--", newPath)
		additions = 1
	}
	if !strings.HasPrefix(runGit(t, dir, "diff", "--cached", "--name-status", "HEAD"), "R") {
		t.Fatal("fixture must produce a genuine Git rename")
	}
	if change == "mixed" {
		writeFile(t, dir, newPath, base+"UNSTAGED_MARKER\n")
		additions = 1
	}
	file := readNumstatPathDetails(t, dir, newPath)
	if file.OldPath != oldPath || file.Additions != additions || file.Deletions != 0 || file.Diff == "" {
		t.Errorf("rename = %+v; want origin %q, +%d -0 and patch", file, oldPath, additions)
	}
	if change == "mixed" {
		assertNumstatFacet(t, file.StagedChange, 0, "")
		assertNumstatFacet(t, file.UnstagedChange, 1, "UNSTAGED_MARKER")
		if file.StagedChange != nil && file.StagedChange.OldPath != oldPath {
			t.Errorf("staged rename origin = %q, want %q", file.StagedChange.OldPath, oldPath)
		}
	}
}

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.36
func TestWorkspaceGitDetailsPreservesBinaryPaths(t *testing.T) {
	for _, staged := range []bool{false, true} {
		for _, path := range []string{"binary.bin", "café.bin", "literal => binary.bin"} {
			t.Run(path+"/"+strconv.FormatBool(staged), func(t *testing.T) {
				skipWindowsInvalidGitPath(t, path)
				dir := setupNumstatPathRepo(t, path, "\x00original")
				writeFile(t, dir, path, "\x00modified")
				if staged {
					runGit(t, dir, "add", "--", path)
				}
				file := readNumstatPathDetails(t, dir, path)
				if file.Additions != 0 || file.Deletions != 0 || !strings.Contains(file.Diff, "Binary files") {
					t.Errorf("binary file = %+v, want zero line counts and binary patch", file)
				}
			})
		}
	}
}

func assertNumstatFacet(t *testing.T, facet *types.FileChangeFacet, additions int, marker string) {
	t.Helper()
	if facet == nil {
		t.Fatal("missing change facet")
	}
	if facet.Additions != additions || facet.Deletions != 0 || facet.DiffState != gitStatusDiffReady || facet.Diff == "" {
		t.Errorf("facet = %+v, want +%d -0, ready and patch", facet, additions)
	}
	if marker != "" && !strings.Contains(facet.Diff, "+"+marker) {
		t.Errorf("facet patch missing %q: %q", marker, facet.Diff)
	}
}

func setupNumstatPathRepo(t *testing.T, path, content string) string {
	t.Helper()
	dir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	runGit(t, dir, "config", "core.quotePath", "true")
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, path)), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, path, content)
	runGit(t, dir, "add", "--", path)
	runGit(t, dir, "commit", "-m", "Track exact path fixture")
	return dir
}

func readNumstatPathDetails(t *testing.T, dir, path string) types.FileInfo {
	t.Helper()
	tracker := NewWorkspaceTracker(dir, newTestLogger(t))
	t.Cleanup(tracker.Stop)
	status, err := tracker.GetGitStatusWithDetails(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	file, ok := status.Files[path]
	if !ok || file.Path != path || len(status.Files) != 1 {
		t.Fatalf("want exactly path %q, got %+v", path, status.Files)
	}
	if file.DiffState != gitStatusDiffReady {
		t.Fatalf("path %q: diff state %q, want ready", path, file.DiffState)
	}
	return file
}

func skipWindowsInvalidGitPath(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS == "windows" && (strings.ContainsAny(path, ">\"\\\t\n") || strings.HasSuffix(path, " ")) {
		t.Skip("filename cannot be represented by the native Windows filesystem")
	}
}
