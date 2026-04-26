package process

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
)

// Verifies the Manager fans out subscriptions across per-repo trackers and
// emits GitStatusUpdates tagged with RepositoryName for each repo in a
// multi-repo task root. This is the wire contract the frontend's
// per-repo Changes panel depends on.
func TestManager_SubscribeWorkspaceStream_MultiRepoEmitsPerRepoStatuses(t *testing.T) {
	taskRoot := t.TempDir()

	// Build two real git repos as siblings under the task root.
	for _, name := range []string{"frontend", "backend"} {
		repoDir, cleanup := setupTestRepo(t)
		t.Cleanup(cleanup)
		dst := filepath.Join(taskRoot, name)
		if err := os.Rename(repoDir, dst); err != nil {
			t.Fatalf("place %s: %v", name, err)
		}
	}

	mgr := NewManager(&config.InstanceConfig{WorkDir: taskRoot}, newTestLogger(t))
	if len(mgr.repoTrackers) != 2 {
		t.Fatalf("expected 2 per-repo trackers, got %d", len(mgr.repoTrackers))
	}

	// Start the trackers so the subscriber's replayed status is non-empty.
	mgr.workspaceTracker.Start(context.Background())
	for _, tr := range mgr.repoTrackers {
		tr.Start(context.Background())
	}
	t.Cleanup(func() {
		mgr.workspaceTracker.Stop()
		for _, tr := range mgr.repoTrackers {
			tr.Stop()
		}
	})

	sub := mgr.SubscribeWorkspaceStream()
	defer mgr.UnsubscribeWorkspaceStream(sub)

	// On subscribe each tracker replays its current status. Drain up to
	// 3 messages (root + 2 repos) and capture the per-repo names seen.
	deadline := time.After(2 * time.Second)
	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case msg := <-sub:
			if msg.Type != types.WorkspaceMessageTypeGitStatus || msg.GitStatus == nil {
				continue
			}
			if name := msg.GitStatus.RepositoryName; name != "" {
				seen[name] = true
			}
		case <-deadline:
			t.Fatalf("timed out waiting for per-repo status messages; got %v", seen)
		}
	}
	if !seen["frontend"] || !seen["backend"] {
		t.Errorf("expected both repos to emit; got %v", seen)
	}
}

// Single-repo workspaces must NOT spawn per-repo trackers — keeps the
// pre-multi-repo behavior verbatim and ensures GitStatusUpdate.RepositoryName
// stays empty for legacy clients.
func TestManager_SingleRepo_NoPerRepoTrackers(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	mgr := NewManager(&config.InstanceConfig{WorkDir: repoDir}, newTestLogger(t))
	if len(mgr.repoTrackers) != 0 {
		t.Errorf("single-repo workspace should not have per-repo trackers; got %d", len(mgr.repoTrackers))
	}
}
