package process

import (
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

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
