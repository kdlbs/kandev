package process

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
)

// TestRescanRepositories_TransitionsSingleToMultiRepo simulates the
// MCP add_branch flow: the agent launched as single-repo with WorkDir set
// to the primary worktree, then a sibling worktree appears at the task
// root. After RescanRepositories the manager must hold one bare root
// tracker + one per-repo tracker for each child.
func TestRescanRepositories_TransitionsSingleToMultiRepo(t *testing.T) {
	taskRoot := t.TempDir()
	primary := filepath.Join(taskRoot, "kandev")
	sibling := filepath.Join(taskRoot, "kandev-feature-x")
	initGitRepoAt(t, primary)
	initGitRepoAt(t, sibling)

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	cfg := &config.InstanceConfig{WorkDir: primary}
	m := NewManager(cfg, log)

	if len(m.repoTrackers) != 0 {
		t.Fatalf("expected 0 repoTrackers in single-repo mode, got %d", len(m.repoTrackers))
	}

	if err := m.RescanRepositories(context.Background(), taskRoot); err != nil {
		t.Fatal(err)
	}

	if len(m.repoTrackers) != 2 {
		t.Fatalf("expected 2 repoTrackers after transition, got %d", len(m.repoTrackers))
	}
	names := map[string]bool{}
	for _, tr := range m.repoTrackers {
		names[tr.RepositoryName()] = true
	}
	if !names["kandev"] || !names["kandev-feature-x"] {
		t.Errorf("expected trackers for kandev + kandev-feature-x, got %v", names)
	}
	if m.cfg.WorkDir != taskRoot {
		t.Errorf("cfg.WorkDir = %q, want %q", m.cfg.WorkDir, taskRoot)
	}
	// Stop trackers so the goroutines don't outlive the test (parent
	// TestMain's goleak check would otherwise flag them).
	m.stopWorkspaceTrackers()
}

// TestRescanRepositories_AppendsNewRepoTrackerInMultiRepoMode covers the
// already-multi case: an extra sibling appears, the existing trackers
// remain, a new tracker is added for the new sibling.
func TestRescanRepositories_AppendsNewRepoTrackerInMultiRepoMode(t *testing.T) {
	taskRoot := t.TempDir()
	repoA := filepath.Join(taskRoot, "alpha")
	repoB := filepath.Join(taskRoot, "beta")
	initGitRepoAt(t, repoA)
	initGitRepoAt(t, repoB)

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	cfg := &config.InstanceConfig{WorkDir: taskRoot}
	m := NewManager(cfg, log)

	if len(m.repoTrackers) != 2 {
		t.Fatalf("expected 2 repoTrackers from construct-time scan, got %d", len(m.repoTrackers))
	}

	// New sibling added after launch (e.g. via add_branch).
	repoC := filepath.Join(taskRoot, "gamma")
	initGitRepoAt(t, repoC)

	if err := m.RescanRepositories(context.Background(), taskRoot); err != nil {
		t.Fatal(err)
	}

	if len(m.repoTrackers) != 3 {
		t.Fatalf("expected 3 repoTrackers after append, got %d", len(m.repoTrackers))
	}
	names := map[string]bool{}
	for _, tr := range m.repoTrackers {
		names[tr.RepositoryName()] = true
	}
	for _, expected := range []string{"alpha", "beta", "gamma"} {
		if !names[expected] {
			t.Errorf("missing tracker for %q (got %v)", expected, names)
		}
	}
	m.stopWorkspaceTrackers()
}

// A live remote materialization does not change the workspace root: it asks
// agentctl to rescan the root it already tracks after adding a sibling. The
// rescan must append only the new tracker, leaving existing tracker instances
// intact so their Changes subscriptions and cached state survive.
func TestRescanRepositories_EmptyWorkDirAppendsNewTrackerAtCurrentRoot(t *testing.T) {
	taskRoot := t.TempDir()
	initGitRepoAt(t, taskRoot)

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	m := NewManager(&config.InstanceConfig{WorkDir: taskRoot}, log)
	defer m.stopWorkspaceTrackers()
	if len(m.repoTrackers) != 0 {
		t.Fatalf("initial repo trackers = %d, want 0", len(m.repoTrackers))
	}
	existing := m.workspaceTracker
	sub := m.SubscribeWorkspaceStream()
	defer m.UnsubscribeWorkspaceStream(sub)
	drainChannel(sub)

	repo := filepath.Join(taskRoot, "beta")
	initGitRepoAt(t, repo)
	if err := m.RescanRepositories(context.Background(), ""); err != nil {
		t.Fatalf("rescan current root: %v", err)
	}

	if len(m.repoTrackers) != 1 {
		t.Fatalf("repo trackers after current-root rescan = %d, want 1", len(m.repoTrackers))
	}
	if m.workspaceTracker != existing {
		t.Fatal("current-root rescan replaced an existing tracker")
	}
	if m.repoTrackers[0].RepositoryName() != "beta" {
		t.Fatalf("new tracker = %q, want beta", m.repoTrackers[0].RepositoryName())
	}
	m.repoTrackers[0].workspaceSubMu.RLock()
	_, visibleInChanges := m.repoTrackers[0].workspaceStreamSubscribers[sub]
	m.repoTrackers[0].workspaceSubMu.RUnlock()
	if !visibleInChanges {
		t.Fatal("new child tracker did not inherit the Changes stream subscriber")
	}
}

func TestReconcileRepositories_PrunesRemovedTrackerAndPreservesSubscription(t *testing.T) {
	taskRoot := t.TempDir()
	original := filepath.Join(taskRoot, "original")
	rolledBack := filepath.Join(taskRoot, "rolled-back")
	initGitRepoAt(t, original)
	initGitRepoAt(t, rolledBack)

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	m := NewManager(&config.InstanceConfig{WorkDir: taskRoot}, log)
	defer m.stopWorkspaceTrackers()
	if len(m.repoTrackers) != 2 {
		t.Fatalf("initial repo trackers = %d, want 2", len(m.repoTrackers))
	}
	originalTracker := m.repoTrackers[0]
	rolledBackTracker := m.repoTrackers[0]
	if originalTracker.RepositoryName() != "original" {
		originalTracker = m.repoTrackers[1]
	}
	if rolledBackTracker.RepositoryName() != "rolled-back" {
		rolledBackTracker = m.repoTrackers[1]
	}
	sub := m.SubscribeWorkspaceStream()
	defer m.UnsubscribeWorkspaceStream(sub)
	if err := os.RemoveAll(rolledBack); err != nil {
		t.Fatal(err)
	}
	if err := m.ReconcileRepositories(context.Background()); err != nil {
		t.Fatalf("reconcile repositories: %v", err)
	}
	if got := m.RepoSubpaths(); len(got) != 1 || got[0] != "original" {
		t.Fatalf("RepoSubpaths = %v, want [original]", got)
	}
	if m.repoTrackers[0] != originalTracker {
		t.Fatal("reconcile replaced the retained tracker")
	}
	m.repoTrackers[0].workspaceSubMu.RLock()
	_, subscribed := m.repoTrackers[0].workspaceStreamSubscribers[sub]
	m.repoTrackers[0].workspaceSubMu.RUnlock()
	if !subscribed {
		t.Fatal("reconcile dropped the retained Changes subscription")
	}
	rolledBackTracker.workspaceSubMu.RLock()
	_, staleSubscribed := rolledBackTracker.workspaceStreamSubscribers[sub]
	rolledBackTracker.workspaceSubMu.RUnlock()
	if staleSubscribed {
		t.Fatal("removed tracker can still replay Changes to the subscription")
	}
}

// TestRescanRepositories_FileTreeReturnsTaskRootContentsAfterTransition
// covers the Files panel surface: after rescan flips a single-branch
// workspace to multi-repo mode, GetWorkspaceTracker().GetFileTree("", n)
// must list the sibling repo dirs (kandev/, kandev-feature-x/) instead of
// the contents of the original primary worktree. This is the bug the user
// hit where the Files panel still rendered .agents/.claude/apps/... from
// inside the primary worktree even though the task had become multi-branch.
func TestRescanRepositories_FileTreeReturnsTaskRootContentsAfterTransition(t *testing.T) {
	taskRoot := t.TempDir()
	primary := filepath.Join(taskRoot, "kandev")
	sibling := filepath.Join(taskRoot, "kandev-feature-x")
	initGitRepoAt(t, primary)
	initGitRepoAt(t, sibling)
	// Seed each worktree with a "marker" file so we can tell which one a
	// file-tree response is rooted at.
	if err := os.WriteFile(filepath.Join(primary, "primary-marker.txt"), []byte("p"), 0o644); err != nil {
		t.Fatalf("write primary marker: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sibling, "sibling-marker.txt"), []byte("s"), 0o644); err != nil {
		t.Fatalf("write sibling marker: %v", err)
	}

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	cfg := &config.InstanceConfig{WorkDir: primary}
	m := NewManager(cfg, log)
	defer m.stopWorkspaceTrackers()

	// Pre-rescan: workspaceTracker is bound to the primary, so the file
	// tree must include the primary's marker (and NOT the sibling's).
	preTree, err := m.workspaceTracker.GetFileTree("", 1)
	if err != nil {
		t.Fatalf("pre-rescan GetFileTree: %v", err)
	}
	preNames := fileTreeChildNames(preTree)
	if !preNames["primary-marker.txt"] {
		t.Errorf("pre-rescan tree missing primary marker: %v", preNames)
	}
	if preNames["sibling-marker.txt"] {
		t.Errorf("pre-rescan tree should not contain sibling marker yet: %v", preNames)
	}

	if err := m.RescanRepositories(context.Background(), taskRoot); err != nil {
		t.Fatal(err)
	}

	postTree, err := m.workspaceTracker.GetFileTree("", 1)
	if err != nil {
		t.Fatalf("post-rescan GetFileTree: %v", err)
	}
	postNames := fileTreeChildNames(postTree)
	if !postNames["kandev"] || !postNames["kandev-feature-x"] {
		t.Errorf("post-rescan tree should list sibling repo dirs, got %v", postNames)
	}
	if postNames["primary-marker.txt"] {
		t.Errorf("post-rescan tree should not include primary's inner files at root: %v", postNames)
	}
}

func TestRescanRepositories_PromotedRootTracksPlainFolderWithOneRepository(t *testing.T) {
	taskRoot := t.TempDir()
	primary := filepath.Join(taskRoot, "repo")
	folder := filepath.Join(taskRoot, "notes")
	initGitRepoAt(t, primary)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	m := NewManager(&config.InstanceConfig{WorkDir: primary}, log)
	defer m.stopWorkspaceTrackers()
	if err := m.RescanRepositories(context.Background(), taskRoot); err != nil {
		t.Fatal(err)
	}
	tree, err := m.workspaceTracker.GetFileTree("", 1)
	if err != nil {
		t.Fatal(err)
	}
	names := fileTreeChildNames(tree)
	if !names["notes"] || !names["repo"] {
		t.Fatalf("promoted root file scope = %v", names)
	}
	if len(m.repoTrackers) != 1 || m.repoTrackers[0].RepositoryName() != "repo" {
		t.Fatalf("repo trackers = %+v", m.repoTrackers)
	}
}

// Source-root policy is installed after a successful API rescan. It must
// therefore be applied to the bare root tracker that replaces the original
// primary tracker as well as every newly-created child tracker.
func TestRescanRepositories_SourceRootsApplyToPromotedTrackers(t *testing.T) {
	taskRoot := t.TempDir()
	primary := filepath.Join(taskRoot, "primary")
	sibling := filepath.Join(taskRoot, "sibling")
	source := t.TempDir()
	initGitRepoAt(t, primary)
	initGitRepoAt(t, sibling)
	for _, dir := range []string{taskRoot, primary, sibling} {
		if err := os.Symlink(source, filepath.Join(dir, "linked")); err != nil {
			t.Skip("symlinks not supported")
		}
	}

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	m := NewManager(&config.InstanceConfig{WorkDir: primary}, log)
	defer m.stopWorkspaceTrackers()
	if err := m.RescanRepositories(context.Background(), taskRoot); err != nil {
		t.Fatal(err)
	}
	m.SetWorkspaceSourceRoots([]string{source})

	assertLinkedSourceFileOperations(t, m.workspaceTracker, "linked/root.txt")
	for _, tracker := range m.repoTrackers {
		assertLinkedSourceFileOperations(t, tracker, "linked/"+tracker.RepositoryName()+".txt")
	}
}

// Empty-path rescans append a new tracker without replacing the root tracker.
// Applying policy after that reconciliation must cover both tracker kinds.
func TestRescanRepositories_SourceRootsApplyToCurrentRootAppends(t *testing.T) {
	taskRoot := t.TempDir()
	source := t.TempDir()
	initGitRepoAt(t, taskRoot)
	if err := os.Symlink(source, filepath.Join(taskRoot, "linked")); err != nil {
		t.Skip("symlinks not supported")
	}

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	m := NewManager(&config.InstanceConfig{WorkDir: taskRoot}, log)
	defer m.stopWorkspaceTrackers()
	child := filepath.Join(taskRoot, "child")
	initGitRepoAt(t, child)
	if err := os.Symlink(source, filepath.Join(child, "linked")); err != nil {
		t.Skip("symlinks not supported")
	}
	if err := m.RescanRepositories(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	m.SetWorkspaceSourceRoots([]string{source})

	assertLinkedSourceFileOperations(t, m.workspaceTracker, "linked/root.txt")
	if len(m.repoTrackers) != 1 {
		t.Fatalf("repo trackers = %d, want 1", len(m.repoTrackers))
	}
	assertLinkedSourceFileOperations(t, m.repoTrackers[0], "linked/child.txt")
}

func assertLinkedSourceFileOperations(t *testing.T, tracker *WorkspaceTracker, path string) {
	t.Helper()
	if err := tracker.CreateFile(path); err != nil {
		t.Fatalf("create %q through linked source: %v", path, err)
	}
	if _, _, err := tracker.ApplyFileDiff(context.Background(), path, "", "not a diff", stringPtr("updated")); err != nil {
		t.Fatalf("write %q through linked source: %v", path, err)
	}
	content, _, _, _, err := tracker.GetFileContent(path)
	if err != nil || content != "updated" {
		t.Fatalf("read %q through linked source = %q, %v", path, content, err)
	}
	renamed := path + ".renamed"
	if err := tracker.RenameFile(path, renamed); err != nil {
		t.Fatalf("rename %q through linked source: %v", path, err)
	}
	if err := tracker.DeleteFile(renamed); err != nil {
		t.Fatalf("delete %q through linked source: %v", renamed, err)
	}
}

// Rebinding is intentionally stronger than a rescan: a live host execution
// changes its process CWD, so no tracker rooted at the previous workspace may
// survive the switch (or a later rollback).
func TestRebindWorkspace_ReplacesTrackersAndSupportsReverseRollback(t *testing.T) {
	oldRoot := t.TempDir()
	newRoot := t.TempDir()
	initGitRepoAt(t, oldRoot)
	newRepo := filepath.Join(newRoot, "repo")
	initGitRepoAt(t, newRepo)

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	m := NewManager(&config.InstanceConfig{WorkDir: oldRoot}, log)
	defer m.stopWorkspaceTrackers()

	if err := m.RebindWorkspace(context.Background(), newRoot); err != nil {
		t.Fatalf("rebind new root: %v", err)
	}
	if m.cfg.WorkDir != newRoot || m.workspaceTracker.workDir != newRoot {
		t.Fatalf("new binding = workdir %q tracker %q, want %q", m.cfg.WorkDir, m.workspaceTracker.workDir, newRoot)
	}
	if len(m.repoTrackers) != 1 || m.repoTrackers[0].workDir != newRepo {
		t.Fatalf("new repo trackers were not replaced: %+v", m.repoTrackers)
	}

	if err := m.RebindWorkspace(context.Background(), oldRoot); err != nil {
		t.Fatalf("reverse rebind: %v", err)
	}
	if m.cfg.WorkDir != oldRoot || m.workspaceTracker.workDir != oldRoot || len(m.repoTrackers) != 0 {
		t.Fatalf("rollback binding = workdir %q tracker %q repos %d", m.cfg.WorkDir, m.workspaceTracker.workDir, len(m.repoTrackers))
	}
}

// fileTreeChildNames returns the set of top-level child names from a
// FileTreeNode. Returns an empty map for nil / leaf nodes so the caller can
// just index without nil checks.
func fileTreeChildNames(n *streams.FileTreeNode) map[string]bool {
	out := map[string]bool{}
	if n == nil {
		return out
	}
	for _, c := range n.Children {
		out[c.Name] = true
	}
	return out
}

// TestRescanRepositories_PropagatesSubscriberToNewTrackers locks in the
// subscriber-list invariant: any workspace-stream subscriber attached
// BEFORE the rescan must remain attached to the resulting tracker set so
// the Changes panel receives events from newly-discovered repos. Without
// this, the single→multi transition would silently drop the existing
// gateway subscriber when it stops the old workspaceTracker, and the UI
// would go quiet.
func TestRescanRepositories_PropagatesSubscriberToNewTrackers(t *testing.T) {
	taskRoot := t.TempDir()
	primary := filepath.Join(taskRoot, "kandev")
	sibling := filepath.Join(taskRoot, "kandev-branch-2")
	initGitRepoAt(t, primary)
	initGitRepoAt(t, sibling)

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	cfg := &config.InstanceConfig{WorkDir: primary}
	m := NewManager(cfg, log)
	defer m.stopWorkspaceTrackers()

	sub := m.SubscribeWorkspaceStream()
	defer m.UnsubscribeWorkspaceStream(sub)
	// Drain the initial replay so the channel doesn't carry stale state
	// into our post-rescan assertions.
	drainChannel(sub)

	if err := m.RescanRepositories(context.Background(), taskRoot); err != nil {
		t.Fatal(err)
	}

	// After transition, every active tracker must hold the subscriber:
	// bare root + 2 per-repo trackers = 3 attach references. The exported
	// surface doesn't expose subscriber count directly, so we verify by
	// inspecting the trackers' internal subscriber maps via the same
	// concurrency primitives the tracker uses (read lock).
	checkTrackerHasSub := func(name string, tr *WorkspaceTracker) {
		tr.workspaceSubMu.RLock()
		defer tr.workspaceSubMu.RUnlock()
		if _, ok := tr.workspaceStreamSubscribers[sub]; !ok {
			t.Errorf("subscriber missing from %q tracker after rescan", name)
		}
	}
	checkTrackerHasSub("workspaceTracker (bare root)", m.workspaceTracker)
	for _, tr := range m.repoTrackers {
		checkTrackerHasSub("repoTracker "+tr.RepositoryName(), tr)
	}
}

// drainChannel non-blocking-empties a workspace-stream subscriber so test
// assertions about post-rescan messages aren't muddied by initial replays.
func drainChannel(ch types.WorkspaceStreamSubscriber) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

// initGitRepoAt creates a git repo at dir with one commit so
// scanRepositorySubdirs (which gates on a valid `.git/index` file)
// recognizes it. git init alone leaves the index missing until the first
// add/commit.
func initGitRepoAt(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	runIn := func(name string, args ...string) {
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s %v in %s: %v\n%s", name, args, dir, err, out)
		}
	}
	runIn("git", "init", "-b", "main")
	runIn("git", "config", "user.email", "test@example.com")
	runIn("git", "config", "user.name", "Test User")
	runIn("git", "config", "commit.gpgsign", "false")
	runIn("git", "commit", "--allow-empty", "-m", "initial")
}
