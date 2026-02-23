package process

import (
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// setupTestRepo creates a git repository with a remote for testing.
// Returns the repo path and a cleanup function.
func setupTestRepo(t *testing.T) (string, func()) {
	t.Helper()

	// Create temp directory for the "remote" bare repo
	remoteDir, err := os.MkdirTemp("", "test-remote-*")
	if err != nil {
		t.Fatalf("failed to create remote dir: %v", err)
	}

	// Create temp directory for the local repo
	localDir, err := os.MkdirTemp("", "test-local-*")
	if err != nil {
		_ = os.RemoveAll(remoteDir)
		t.Fatalf("failed to create local dir: %v", err)
	}

	cleanup := func() {
		_ = os.RemoveAll(remoteDir)
		_ = os.RemoveAll(localDir)
	}

	// Initialize bare remote repo with explicit branch name for consistency
	runGit(t, remoteDir, "init", "--bare", "--initial-branch=main")

	// Initialize local repo with explicit branch name
	runGit(t, localDir, "init", "--initial-branch=main")
	runGit(t, localDir, "config", "user.email", "test@test.com")
	runGit(t, localDir, "config", "user.name", "Test User")
	runGit(t, localDir, "config", "core.hooksPath", "/dev/null") // Disable hooks in test repo

	// Create initial commit
	writeFile(t, localDir, "README.md", "# Test Repo")
	runGit(t, localDir, "add", ".")
	runGit(t, localDir, "commit", "-m", "Initial commit")

	// Add remote and push
	runGit(t, localDir, "remote", "add", "origin", remoteDir)
	runGit(t, localDir, "push", "-u", "origin", "main")

	return localDir, cleanup
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	// Use git -C flag to ensure we're in the right directory
	// This is more reliable than cmd.Dir as it prevents git from
	// walking up to find a parent .git directory
	fullArgs := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", fullArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\nOutput: %s", args, err, out)
	}
	return string(out)
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write file %s: %v", name, err)
	}
}

func TestIsOnRemote(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)
	ctx := context.Background()

	// Get the initial commit SHA (which is on origin/main)
	initialSHA := runGit(t, repoDir, "rev-parse", "HEAD")
	initialSHA = initialSHA[:len(initialSHA)-1] // trim newline

	// Test: initial commit should be on remote
	if !wt.isOnRemote(ctx, initialSHA) {
		t.Errorf("expected initial commit %s to be on remote", initialSHA)
	}

	// Create a local commit (not pushed)
	writeFile(t, repoDir, "local.txt", "local content")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "Local commit")

	localSHA := runGit(t, repoDir, "rev-parse", "HEAD")
	localSHA = localSHA[:len(localSHA)-1]

	// Test: local commit should NOT be on remote
	if wt.isOnRemote(ctx, localSHA) {
		t.Errorf("expected local commit %s to NOT be on remote", localSHA)
	}

	// Push the local commit
	runGit(t, repoDir, "push")

	// Test: after push, commit should be on remote
	if !wt.isOnRemote(ctx, localSHA) {
		t.Errorf("expected pushed commit %s to be on remote", localSHA)
	}
}

func TestFilterLocalCommits(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)
	ctx := context.Background()

	// Get initial commit SHA
	initialSHA := runGit(t, repoDir, "rev-parse", "HEAD")
	initialSHA = initialSHA[:len(initialSHA)-1]

	// Create two local commits
	writeFile(t, repoDir, "file1.txt", "content1")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "Local commit 1")
	local1SHA := runGit(t, repoDir, "rev-parse", "HEAD")
	local1SHA = local1SHA[:len(local1SHA)-1]

	writeFile(t, repoDir, "file2.txt", "content2")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "Local commit 2")
	local2SHA := runGit(t, repoDir, "rev-parse", "HEAD")
	local2SHA = local2SHA[:len(local2SHA)-1]

	// Create test commits slice with mix of remote and local commits
	commits := []*streams.GitCommitNotification{
		{CommitSHA: initialSHA, Message: "Initial commit", Timestamp: time.Now()},
		{CommitSHA: local1SHA, Message: "Local commit 1", Timestamp: time.Now()},
		{CommitSHA: local2SHA, Message: "Local commit 2", Timestamp: time.Now()},
	}

	// Filter commits
	filtered := wt.filterLocalCommits(ctx, commits)

	// Should only have the 2 local commits, not the initial (remote) commit
	if len(filtered) != 2 {
		t.Errorf("expected 2 local commits, got %d", len(filtered))
	}

	for _, c := range filtered {
		if c.CommitSHA == initialSHA {
			t.Errorf("initial commit should have been filtered out")
		}
	}
}

func TestGetGitStatus_AheadBehindWithoutUpstream(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)
	ctx := context.Background()

	// Create a new branch without setting upstream
	runGit(t, repoDir, "checkout", "-b", "feature-branch")

	// Make a local commit
	writeFile(t, repoDir, "feature.txt", "feature content")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "Feature commit")

	// Get git status - should still calculate ahead/behind against origin/main
	status, err := wt.getGitStatus(ctx)
	if err != nil {
		t.Fatalf("failed to get git status: %v", err)
	}

	// Should be 1 ahead of origin/main
	if status.Ahead != 1 {
		t.Errorf("expected ahead=1, got %d", status.Ahead)
	}

	// Should be 0 behind
	if status.Behind != 0 {
		t.Errorf("expected behind=0, got %d", status.Behind)
	}

	// Branch should be feature-branch
	if status.Branch != "feature-branch" {
		t.Errorf("expected branch=feature-branch, got %s", status.Branch)
	}
}

// TestFilterLocalCommits_PullAndResetScenario tests the exact scenario where:
// 1. Session starts at commit X
// 2. Upstream (origin/main) gets new commits
// 3. User does git fetch && git reset --hard origin/main
// 4. The upstream commits should be filtered out
func TestFilterLocalCommits_PullAndResetScenario(t *testing.T) {
	// Create temp directories
	remoteDir, err := os.MkdirTemp("", "test-remote-*")
	if err != nil {
		t.Fatalf("failed to create remote dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(remoteDir) }()

	localDir, err := os.MkdirTemp("", "test-local-*")
	if err != nil {
		t.Fatalf("failed to create local dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(localDir) }()

	// Another clone to simulate upstream changes
	upstreamClone, err := os.MkdirTemp("", "test-upstream-*")
	if err != nil {
		t.Fatalf("failed to create upstream clone dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(upstreamClone) }()

	// Initialize bare remote repo with explicit branch name
	runGit(t, remoteDir, "init", "--bare", "--initial-branch=main")

	// Initialize local repo (the "session" repo) with explicit branch name
	runGit(t, localDir, "init", "--initial-branch=main")
	runGit(t, localDir, "config", "user.email", "test@test.com")
	runGit(t, localDir, "config", "user.name", "Test User")
	runGit(t, localDir, "config", "core.hooksPath", "/dev/null") // Disable hooks in test repo
	writeFile(t, localDir, "README.md", "# Test Repo")
	runGit(t, localDir, "add", ".")
	runGit(t, localDir, "commit", "-m", "Initial commit (X)")
	runGit(t, localDir, "remote", "add", "origin", remoteDir)
	runGit(t, localDir, "push", "-u", "origin", "main")

	// Record the starting point (commit X)
	startingSHA := runGit(t, localDir, "rev-parse", "HEAD")
	startingSHA = startingSHA[:len(startingSHA)-1]

	// Clone to upstream clone and make commits there (simulating main evolving)
	runGit(t, upstreamClone, "clone", remoteDir, ".")
	runGit(t, upstreamClone, "config", "user.email", "upstream@test.com")
	runGit(t, upstreamClone, "config", "user.name", "Upstream User")
	runGit(t, upstreamClone, "config", "core.hooksPath", "/dev/null") // Disable hooks in test repo

	// Make upstream commits Y and Z
	writeFile(t, upstreamClone, "upstream1.txt", "upstream content 1")
	runGit(t, upstreamClone, "add", ".")
	runGit(t, upstreamClone, "commit", "-m", "Upstream commit Y")
	// Note: We don't need to capture Y's SHA, just Z's for verification

	writeFile(t, upstreamClone, "upstream2.txt", "upstream content 2")
	runGit(t, upstreamClone, "add", ".")
	runGit(t, upstreamClone, "commit", "-m", "Upstream commit Z")
	upstreamZ := runGit(t, upstreamClone, "rev-parse", "HEAD")
	upstreamZ = upstreamZ[:len(upstreamZ)-1]

	// Push upstream commits
	runGit(t, upstreamClone, "push")

	// Now in the local repo (session), fetch and reset to origin/main
	runGit(t, localDir, "fetch", "origin")
	runGit(t, localDir, "reset", "--hard", "origin/main")

	// Verify HEAD is now at Z
	currentHead := runGit(t, localDir, "rev-parse", "HEAD")
	currentHead = currentHead[:len(currentHead)-1]
	if currentHead != upstreamZ {
		t.Fatalf("expected HEAD to be at %s, got %s", upstreamZ, currentHead)
	}

	// Create workspace tracker and test filtering
	log := newTestLogger(t)
	wt := NewWorkspaceTracker(localDir, log)
	ctx := context.Background()

	// Simulate what checkGitChanges would do: get commits since starting point
	commits := wt.getCommitsSince(ctx, startingSHA)

	// Should have 2 commits (Y and Z)
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits since starting point, got %d", len(commits))
	}

	// Filter local commits - should filter out ALL of them since they're on remote
	filtered := wt.filterLocalCommits(ctx, commits)

	// Should have 0 commits after filtering (all are upstream commits)
	if len(filtered) != 0 {
		t.Errorf("expected 0 local commits after filtering upstream, got %d", len(filtered))
		for _, c := range filtered {
			t.Errorf("  unexpected commit: %s - %s", c.CommitSHA[:8], c.Message)
		}
	}
}

func TestGetFileContent_BinaryDetection(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)

	// Test 1: Valid UTF-8 text file returns isBinary=false with raw content
	textContent := "Hello, world!\nLine 2\n"
	writeFile(t, repoDir, "text.txt", textContent)

	content, size, isBinary, err := wt.GetFileContent("text.txt")
	if err != nil {
		t.Fatalf("GetFileContent(text.txt) error: %v", err)
	}
	if isBinary {
		t.Error("expected isBinary=false for text file")
	}
	if content != textContent {
		t.Errorf("expected content %q, got %q", textContent, content)
	}
	if size != int64(len(textContent)) {
		t.Errorf("expected size %d, got %d", len(textContent), size)
	}

	// Test 2: Non-UTF-8 binary file returns isBinary=true with base64-encoded content
	binaryContent := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0xFF, 0xFE}
	if err := os.WriteFile(filepath.Join(repoDir, "image.png"), binaryContent, 0o644); err != nil {
		t.Fatalf("failed to write binary file: %v", err)
	}

	content, size, isBinary, err = wt.GetFileContent("image.png")
	if err != nil {
		t.Fatalf("GetFileContent(image.png) error: %v", err)
	}
	if !isBinary {
		t.Error("expected isBinary=true for binary file")
	}
	if size != int64(len(binaryContent)) {
		t.Errorf("expected size %d, got %d", len(binaryContent), size)
	}

	// Verify content is valid base64 that decodes to original bytes
	decoded, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		t.Fatalf("failed to decode base64 content: %v", err)
	}
	if string(decoded) != string(binaryContent) {
		t.Errorf("decoded content doesn't match original binary")
	}

	// Test 3: Empty file returns isBinary=false
	writeFile(t, repoDir, "empty.txt", "")

	_, _, isBinary, err = wt.GetFileContent("empty.txt")
	if err != nil {
		t.Fatalf("GetFileContent(empty.txt) error: %v", err)
	}
	if isBinary {
		t.Error("expected isBinary=false for empty file")
	}

	// Test 4: Path traversal is rejected
	_, _, _, err = wt.GetFileContent("../../etc/passwd")
	if err == nil {
		t.Error("expected error for path traversal, got nil")
	}
}

// setupTestDir creates a temp directory with a WorkspaceTracker (no git required).
func setupTestDir(t *testing.T) (string, *WorkspaceTracker) {
	t.Helper()
	dir := t.TempDir()
	log := newTestLogger(t)
	wt := NewWorkspaceTracker(dir, log)
	return dir, wt
}

func TestCreateFile(t *testing.T) {
	t.Run("basic creation", func(t *testing.T) {
		dir, wt := setupTestDir(t)
		err := wt.CreateFile("hello.txt")
		if err != nil {
			t.Fatalf("CreateFile failed: %v", err)
		}
		info, err := os.Stat(filepath.Join(dir, "hello.txt"))
		if err != nil {
			t.Fatalf("file not found after creation: %v", err)
		}
		if info.Size() != 0 {
			t.Errorf("expected empty file, got size %d", info.Size())
		}
	})

	t.Run("content is empty", func(t *testing.T) {
		dir, wt := setupTestDir(t)
		_ = wt.CreateFile("empty.txt")
		data, err := os.ReadFile(filepath.Join(dir, "empty.txt"))
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		if len(data) != 0 {
			t.Errorf("expected empty content, got %d bytes", len(data))
		}
	})

	t.Run("path traversal rejected", func(t *testing.T) {
		_, wt := setupTestDir(t)
		err := wt.CreateFile("../../etc/passwd")
		if err == nil {
			t.Fatal("expected error for path traversal, got nil")
		}
		if !strings.Contains(err.Error(), "path traversal") {
			t.Errorf("expected path traversal error, got: %v", err)
		}
	})

	t.Run("already existing file", func(t *testing.T) {
		dir, wt := setupTestDir(t)
		writeFile(t, dir, "exists.txt", "content")
		err := wt.CreateFile("exists.txt")
		if err == nil {
			t.Fatal("expected error for existing file, got nil")
		}
		if !strings.Contains(err.Error(), "already exists") {
			t.Errorf("expected 'already exists' error, got: %v", err)
		}
	})

	t.Run("intermediate directory creation", func(t *testing.T) {
		dir, wt := setupTestDir(t)
		err := wt.CreateFile("subdir/deep/file.txt")
		if err != nil {
			t.Fatalf("CreateFile with subdirs failed: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "subdir", "deep", "file.txt")); err != nil {
			t.Fatalf("nested file not found: %v", err)
		}
	})
}

func TestDeleteFile(t *testing.T) {
	t.Run("basic deletion", func(t *testing.T) {
		dir, wt := setupTestDir(t)
		writeFile(t, dir, "doomed.txt", "bye")
		err := wt.DeleteFile("doomed.txt")
		if err != nil {
			t.Fatalf("DeleteFile failed: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "doomed.txt")); !os.IsNotExist(err) {
			t.Errorf("expected file to be deleted, stat returned: %v", err)
		}
	})

	t.Run("path traversal rejected", func(t *testing.T) {
		_, wt := setupTestDir(t)
		err := wt.DeleteFile("../../etc/passwd")
		if err == nil {
			t.Fatal("expected error for path traversal, got nil")
		}
		if !strings.Contains(err.Error(), "path traversal") {
			t.Errorf("expected path traversal error, got: %v", err)
		}
	})

	t.Run("non-existent file", func(t *testing.T) {
		_, wt := setupTestDir(t)
		err := wt.DeleteFile("ghost.txt")
		if err == nil {
			t.Fatal("expected error for non-existent file, got nil")
		}
		if !strings.Contains(err.Error(), "does not exist") {
			t.Errorf("expected 'does not exist' error, got: %v", err)
		}
	})

	t.Run("directory deletion rejected", func(t *testing.T) {
		dir, wt := setupTestDir(t)
		if err := os.Mkdir(filepath.Join(dir, "mydir"), 0o755); err != nil {
			t.Fatalf("mkdir failed: %v", err)
		}
		err := wt.DeleteFile("mydir")
		if err == nil {
			t.Fatal("expected error for directory deletion, got nil")
		}
		if !strings.Contains(err.Error(), "cannot delete directory") {
			t.Errorf("expected 'cannot delete directory' error, got: %v", err)
		}
	})
}

func TestFsOpToString(t *testing.T) {
	tests := []struct {
		op   fsnotify.Op
		want string
	}{
		{fsnotify.Create, types.FileOpCreate},
		{fsnotify.Write, types.FileOpWrite},
		{fsnotify.Remove, types.FileOpRemove},
		{fsnotify.Rename, types.FileOpRename},
		{fsnotify.Chmod, types.FileOpRefresh},
	}

	for _, tt := range tests {
		t.Run(tt.op.String(), func(t *testing.T) {
			got := fsOpToString(tt.op)
			if got != tt.want {
				t.Errorf("fsOpToString(%v) = %q, want %q", tt.op, got, tt.want)
			}
		})
	}
}

func TestRewriteDiffPaths(t *testing.T) {
	tests := []struct {
		name    string
		diff    string
		oldPath string
		newPath string
		want    string
	}{
		{
			name:    "p0 format (no a/ b/ prefix)",
			diff:    "--- CLAUDE.md\n+++ CLAUDE.md\n@@ -1,2 +1,2 @@\n-old\n+new",
			oldPath: "CLAUDE.md",
			newPath: "real/CLAUDE.md",
			want:    "--- real/CLAUDE.md\n+++ real/CLAUDE.md\n@@ -1,2 +1,2 @@\n-old\n+new",
		},
		{
			name:    "with a/ b/ prefix",
			diff:    "--- a/CLAUDE.md\n+++ b/CLAUDE.md\n@@ -1 +1 @@\n-old\n+new",
			oldPath: "CLAUDE.md",
			newPath: "real/CLAUDE.md",
			want:    "--- real/CLAUDE.md\n+++ real/CLAUDE.md\n@@ -1 +1 @@\n-old\n+new",
		},
		{
			name:    "no match leaves diff unchanged",
			diff:    "--- other.txt\n+++ other.txt\n@@ -1 +1 @@\n-old\n+new",
			oldPath: "CLAUDE.md",
			newPath: "real/CLAUDE.md",
			want:    "--- other.txt\n+++ other.txt\n@@ -1 +1 @@\n-old\n+new",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rewriteDiffPaths(tt.diff, tt.oldPath, tt.newPath)
			if got != tt.want {
				t.Errorf("rewriteDiffPaths() =\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestApplyFileDiff_RegularFile(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Resolve symlinks so workDir matches git's toplevel (macOS /var -> /private/var)
	repoDir, _ = filepath.EvalSymlinks(repoDir)
	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)

	// Create a tracked file and commit it (git apply needs the file in the index)
	original := "line1\nline2\nline3\n"
	writeFile(t, repoDir, "test.txt", original)
	runGit(t, repoDir, "add", "test.txt")
	runGit(t, repoDir, "commit", "-m", "add test.txt")

	// Build a unified diff that changes line2 -> modified
	diff := "--- test.txt\n+++ test.txt\n@@ -1,3 +1,3 @@\n line1\n-line2\n+modified\n line3\n"

	hash, err := wt.ApplyFileDiff("test.txt", diff, "")
	if err != nil {
		t.Fatalf("ApplyFileDiff failed: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}

	// Verify file content
	content, err := os.ReadFile(filepath.Join(repoDir, "test.txt"))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	expected := "line1\nmodified\nline3\n"
	if string(content) != expected {
		t.Errorf("content = %q, want %q", string(content), expected)
	}
}

func TestApplyFileDiff_Symlink(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Resolve symlinks so workDir matches git's toplevel (macOS /var -> /private/var)
	repoDir, _ = filepath.EvalSymlinks(repoDir)
	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)

	// Create a real file in a subdirectory
	realDir := filepath.Join(repoDir, "real")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatalf("failed to create real dir: %v", err)
	}
	original := "line1\nline2\nline3\n"
	writeFile(t, repoDir, "real/target.md", original)

	// Create a symlink at the repo root pointing to the real file
	symlinkPath := filepath.Join(repoDir, "LINK.md")
	if err := os.Symlink(filepath.Join(repoDir, "real", "target.md"), symlinkPath); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	// Commit both files so git apply works
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "add real file and symlink")

	// Build a diff targeting the symlink path
	diff := "--- LINK.md\n+++ LINK.md\n@@ -1,3 +1,3 @@\n line1\n-line2\n+patched\n line3\n"

	hash, err := wt.ApplyFileDiff("LINK.md", diff, "")
	if err != nil {
		t.Fatalf("ApplyFileDiff through symlink failed: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}

	// Verify the real file was patched (not replaced by a regular file)
	realContent, err := os.ReadFile(filepath.Join(repoDir, "real", "target.md"))
	if err != nil {
		t.Fatalf("failed to read real file: %v", err)
	}
	expected := "line1\npatched\nline3\n"
	if string(realContent) != expected {
		t.Errorf("real file content = %q, want %q", string(realContent), expected)
	}

	// Verify the symlink still exists (not replaced)
	linkInfo, err := os.Lstat(symlinkPath)
	if err != nil {
		t.Fatalf("failed to lstat symlink: %v", err)
	}
	if linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Error("LINK.md is no longer a symlink after patching")
	}

	// Verify reading through the symlink gives the same content
	symlinkContent, err := os.ReadFile(symlinkPath)
	if err != nil {
		t.Fatalf("failed to read symlink: %v", err)
	}
	if string(symlinkContent) != expected {
		t.Errorf("symlink content = %q, want %q", string(symlinkContent), expected)
	}
}

func TestApplyFileDiff_ConflictDetection(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Resolve symlinks so workDir matches git's toplevel (macOS /var -> /private/var)
	repoDir, _ = filepath.EvalSymlinks(repoDir)
	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)

	writeFile(t, repoDir, "conflict.txt", "original\n")
	runGit(t, repoDir, "add", "conflict.txt")
	runGit(t, repoDir, "commit", "-m", "add conflict.txt")

	// Get hash of the original content
	origHash := calculateSHA256("original\n")

	// Now modify the file behind the diff's back
	writeFile(t, repoDir, "conflict.txt", "modified-by-someone-else\n")

	diff := "--- conflict.txt\n+++ conflict.txt\n@@ -1 +1 @@\n-original\n+patched\n"

	_, err := wt.ApplyFileDiff("conflict.txt", diff, origHash)
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}
	if !strings.Contains(err.Error(), "conflict detected") {
		t.Errorf("expected 'conflict detected' error, got: %v", err)
	}
}

func TestEmitFileChanges(t *testing.T) {
	t.Run("zero changes sends refresh", func(t *testing.T) {
		_, wt := setupTestDir(t)
		sub := wt.SubscribeWorkspaceStream()
		defer wt.UnsubscribeWorkspaceStream(sub)

		// Drain initial git status message from subscribe
		select {
		case <-sub:
		case <-time.After(time.Second):
		}

		wt.emitFileChanges(nil)

		select {
		case msg := <-sub:
			if msg.FileChange == nil {
				t.Fatal("expected FileChange, got nil")
			}
			if msg.FileChange.Operation != types.FileOpRefresh {
				t.Errorf("expected refresh op, got %q", msg.FileChange.Operation)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for message")
		}
	})

	t.Run("specific changes sent individually", func(t *testing.T) {
		_, wt := setupTestDir(t)
		sub := wt.SubscribeWorkspaceStream()
		defer wt.UnsubscribeWorkspaceStream(sub)

		// Drain the initial git status message from subscribe
		select {
		case <-sub:
		case <-time.After(time.Second):
		}

		changes := []types.FileChangeNotification{
			{Timestamp: time.Now(), Path: "a.txt", Operation: types.FileOpCreate},
			{Timestamp: time.Now(), Path: "b.txt", Operation: types.FileOpWrite},
		}
		wt.emitFileChanges(changes)

		for i, want := range changes {
			select {
			case msg := <-sub:
				if msg.FileChange == nil {
					t.Fatalf("change %d: expected FileChange, got nil", i)
				}
				if msg.FileChange.Path != want.Path {
					t.Errorf("change %d: path = %q, want %q", i, msg.FileChange.Path, want.Path)
				}
			case <-time.After(time.Second):
				t.Fatalf("change %d: timed out", i)
			}
		}
	})

	t.Run("over 50 changes sends refresh", func(t *testing.T) {
		_, wt := setupTestDir(t)
		sub := wt.SubscribeWorkspaceStream()
		defer wt.UnsubscribeWorkspaceStream(sub)

		// Drain initial message
		select {
		case <-sub:
		case <-time.After(time.Second):
		}

		changes := make([]types.FileChangeNotification, 51)
		for i := range changes {
			changes[i] = types.FileChangeNotification{
				Timestamp: time.Now(),
				Path:      "file.txt",
				Operation: types.FileOpWrite,
			}
		}
		wt.emitFileChanges(changes)

		select {
		case msg := <-sub:
			if msg.FileChange == nil {
				t.Fatal("expected FileChange, got nil")
			}
			if msg.FileChange.Operation != types.FileOpRefresh {
				t.Errorf("expected refresh op, got %q", msg.FileChange.Operation)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for message")
		}
	})
}
