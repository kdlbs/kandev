package process

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
)

// TestManagerUpdateBaseBranches_StampsRootTracker confirms a single-repo
// task's root tracker receives the new value when the kandev backend pushes
// an updated map (changes-panel "Compare against" picker path). The legacy
// empty-key entry maps to the root.
func TestManagerUpdateBaseBranches_StampsRootTracker(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	mgr := NewManager(&config.InstanceConfig{WorkDir: repoDir}, log)

	root := mgr.GetWorkspaceTracker()
	if root == nil {
		t.Fatal("expected root tracker for single-repo workspace")
	}
	if got := root.BaseBranch(); got != "" {
		t.Fatalf("precondition: root baseBranch should start empty, got %q", got)
	}

	mgr.UpdateBaseBranches(context.Background(), map[string]string{"": "develop"})

	if got := root.BaseBranch(); got != "develop" {
		t.Errorf("root tracker baseBranch = %q, want %q after update", got, "develop")
	}
}

// TestManagerUpdateBaseBranches_PersistsToCfg verifies the new map is stored
// on cfg so trackers spawned later (rescan path, lazy subpath lookup) see
// the latest value without a second push.
func TestManagerUpdateBaseBranches_PersistsToCfg(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	mgr := NewManager(&config.InstanceConfig{WorkDir: repoDir}, newTestLogger(t))
	mgr.UpdateBaseBranches(context.Background(), map[string]string{"alpha": "main", "beta": "develop"})

	branches := mgr.getBaseBranches()
	if got := branches["alpha"]; got != "main" {
		t.Errorf("getBaseBranches()[alpha] = %q, want main", got)
	}
	if got := branches["beta"]; got != "develop" {
		t.Errorf("getBaseBranches()[beta] = %q, want develop", got)
	}
}

// TestManagerUpdateBaseBranches_ClearedMapResetsTracker covers the "user
// cleared the override" path: pushing nil should leave the tracker with no
// baseBranch (fallback to the hardcoded integration-branch list).
func TestManagerUpdateBaseBranches_ClearedMapResetsTracker(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	mgr := NewManager(&config.InstanceConfig{WorkDir: repoDir}, newTestLogger(t))
	root := mgr.GetWorkspaceTracker()

	// Seed
	mgr.UpdateBaseBranches(context.Background(), map[string]string{"": "develop"})
	if got := root.BaseBranch(); got != "develop" {
		t.Fatalf("seeding failed: got %q", got)
	}

	// Clear
	mgr.UpdateBaseBranches(context.Background(), nil)
	if got := root.BaseBranch(); got != "" {
		t.Errorf("baseBranch after clear = %q, want empty", got)
	}
}
