package process

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
)

func TestManager_DiscoversNestedSubmoduleScopesWithStableAnchors(t *testing.T) {
	parent, parentCleanup := setupTestRepo(t)
	t.Cleanup(parentCleanup)
	direct, directCleanup := setupTestRepo(t)
	t.Cleanup(directCleanup)
	nested, nestedCleanup := setupTestRepo(t)
	t.Cleanup(nestedCleanup)

	runGit(t, direct, "-c", "protocol.file.allow=always", "submodule", "add", nested, "vendor/inner")
	runGit(t, direct, "add", ".")
	runGit(t, direct, "commit", "-m", "add nested submodule")

	runGit(t, parent, "-c", "protocol.file.allow=always", "submodule", "add", direct, "vendor/outer")
	runGit(t, parent, "-c", "protocol.file.allow=always", "submodule", "update", "--init", "--recursive")
	runGit(t, parent, "add", ".")
	runGit(t, parent, "commit", "-m", "add direct submodule")
	runGit(t, parent, "push", "origin", "main")
	runGit(t, parent, "checkout", "-b", "feature/review")

	directAnchor := strings.TrimSpace(runGit(t, parent, "rev-parse", "origin/main:vendor/outer"))
	nestedAnchor := strings.TrimSpace(runGit(t, filepath.Join(parent, "vendor/outer"), "rev-parse", "HEAD:vendor/inner"))

	mgr := NewManager(&config.InstanceConfig{
		WorkDir:      parent,
		BaseBranches: map[string]string{"": "main"},
	}, newTestLogger(t))
	t.Cleanup(mgr.stopWorkspaceTrackers)

	if mgr.GetWorkspaceTracker().RepositoryName() != "" {
		t.Fatalf("root tracker RepositoryName() = %q, want empty root scope", mgr.GetWorkspaceTracker().RepositoryName())
	}
	if got := mgr.RepoSubpaths(); !slices.Equal(got, []string{"vendor/outer", "vendor/outer/vendor/inner"}) {
		t.Fatalf("RepoSubpaths() = %v, want nested scopes", got)
	}
	if got := mgr.RepositoryScopes(); !slices.Equal(got, []string{"", "vendor/outer", "vendor/outer/vendor/inner"}) {
		t.Fatalf("RepositoryScopes() = %v, want root and nested scopes", got)
	}

	directTracker, err := mgr.GetWorkspaceTrackerFor("vendor/outer")
	if err != nil {
		t.Fatalf("direct tracker: %v", err)
	}
	if got := directTracker.BaseBranch(); got != directAnchor {
		t.Errorf("direct BaseBranch() = %q, want parent gitlink %q", got, directAnchor)
	}

	nestedTracker, err := mgr.GetWorkspaceTrackerFor("vendor/outer/vendor/inner")
	if err != nil {
		t.Fatalf("nested tracker: %v", err)
	}
	if got := nestedTracker.BaseBranch(); got != nestedAnchor {
		t.Errorf("nested BaseBranch() = %q, want parent gitlink %q", got, nestedAnchor)
	}

	if _, err := mgr.GetWorkspaceTrackerFor("vendor/outer"); err != nil {
		t.Fatalf("repeated direct tracker lookup: %v", err)
	}

}

func TestManager_RescanDiscoversNewNestedSubmoduleAndRetainsRoot(t *testing.T) {
	parent, parentCleanup := setupTestRepo(t)
	t.Cleanup(parentCleanup)
	child, childCleanup := setupTestRepo(t)
	t.Cleanup(childCleanup)

	mgr := NewManager(&config.InstanceConfig{WorkDir: parent}, newTestLogger(t))
	t.Cleanup(mgr.stopWorkspaceTrackers)
	root := mgr.GetWorkspaceTracker()
	if got := mgr.RepoSubpaths(); len(got) != 0 {
		t.Fatalf("initial RepoSubpaths() = %v, want none", got)
	}

	runGit(t, parent, "-c", "protocol.file.allow=always", "submodule", "add", child, "vendor/lib")
	runGit(t, parent, "add", ".")
	runGit(t, parent, "commit", "-m", "add submodule")

	if err := mgr.RescanRepositories(context.Background(), parent); err != nil {
		t.Fatalf("rescan: %v", err)
	}
	if mgr.GetWorkspaceTracker() != root {
		t.Fatal("rescan replaced the root tracker for an unchanged workspace")
	}
	if got := mgr.RepoSubpaths(); !slices.Equal(got, []string{"vendor/lib"}) {
		t.Fatalf("rescanned RepoSubpaths() = %v, want vendor/lib", got)
	}
	if tracker, err := mgr.GetWorkspaceTrackerFor("vendor/lib"); err != nil || tracker == nil {
		t.Fatalf("rescanned child tracker = %v, err %v", tracker, err)
	}
}

func TestManager_SkipsUninitializedSubmoduleWithoutLosingRoot(t *testing.T) {
	parent, parentCleanup := setupTestRepo(t)
	t.Cleanup(parentCleanup)
	child, childCleanup := setupTestRepo(t)
	t.Cleanup(childCleanup)

	runGit(t, parent, "-c", "protocol.file.allow=always", "submodule", "add", child, "vendor/lib")
	runGit(t, parent, "add", ".")
	runGit(t, parent, "commit", "-m", "add submodule")
	if err := os.RemoveAll(filepath.Join(parent, "vendor/lib")); err != nil {
		t.Fatalf("remove submodule worktree: %v", err)
	}

	mgr := NewManager(&config.InstanceConfig{WorkDir: parent}, newTestLogger(t))
	t.Cleanup(mgr.stopWorkspaceTrackers)
	if got := mgr.RepoSubpaths(); len(got) != 0 {
		t.Fatalf("uninitialized RepoSubpaths() = %v, want none", got)
	}
	if mgr.GetWorkspaceTracker() == nil {
		t.Fatal("root tracker was lost when submodule was unavailable")
	}
}
