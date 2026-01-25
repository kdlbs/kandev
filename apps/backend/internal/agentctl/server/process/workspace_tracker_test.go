package process

import (
	"context"
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

	// Initialize bare remote repo
	runGit(t, remoteDir, "init", "--bare")

	// Initialize local repo
	runGit(t, localDir, "init")
	runGit(t, localDir, "config", "user.email", "test@test.com")
	runGit(t, localDir, "config", "user.name", "Test User")

	// Create initial commit
	writeFile(t, localDir, "README.md", "# Test Repo")
	runGit(t, localDir, "add", ".")
	runGit(t, localDir, "commit", "-m", "Initial commit")

	// Add remote and push
	runGit(t, localDir, "remote", "add", "origin", remoteDir)
	runGit(t, localDir, "push", "-u", "origin", "master")

	return localDir, cleanup
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\nOutput: %s", args, err, out)
	}
	return string(out)
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write file %s: %v", name, err)
	}
}

func TestIsOnRemote(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)
	ctx := context.Background()

	// Get the initial commit SHA (which is on origin/master)
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

	// Get git status - should still calculate ahead/behind against origin/master
	status, err := wt.getGitStatus(ctx)
	if err != nil {
		t.Fatalf("failed to get git status: %v", err)
	}

	// Should be 1 ahead of origin/master
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
// 2. Upstream (origin/master) gets new commits
// 3. User does git fetch && git reset --hard origin/master
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

	// Initialize bare remote repo
	runGit(t, remoteDir, "init", "--bare")

	// Initialize local repo (the "session" repo)
	runGit(t, localDir, "init")
	runGit(t, localDir, "config", "user.email", "test@test.com")
	runGit(t, localDir, "config", "user.name", "Test User")
	writeFile(t, localDir, "README.md", "# Test Repo")
	runGit(t, localDir, "add", ".")
	runGit(t, localDir, "commit", "-m", "Initial commit (X)")
	runGit(t, localDir, "remote", "add", "origin", remoteDir)
	runGit(t, localDir, "push", "-u", "origin", "master")

	// Record the starting point (commit X)
	startingSHA := runGit(t, localDir, "rev-parse", "HEAD")
	startingSHA = startingSHA[:len(startingSHA)-1]

	// Clone to upstream clone and make commits there (simulating main evolving)
	runGit(t, upstreamClone, "clone", remoteDir, ".")
	runGit(t, upstreamClone, "config", "user.email", "upstream@test.com")
	runGit(t, upstreamClone, "config", "user.name", "Upstream User")

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

	// Now in the local repo (session), fetch and reset to origin/master
	runGit(t, localDir, "fetch", "origin")
	runGit(t, localDir, "reset", "--hard", "origin/master")

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

