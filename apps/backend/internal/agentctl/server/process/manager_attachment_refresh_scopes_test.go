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

func placeSiblingRepos(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, name := range names {
		repoDir, cleanup := setupTestRepo(t)
		t.Cleanup(cleanup)
		if err := os.Rename(repoDir, filepath.Join(root, name)); err != nil {
			t.Fatalf("place %s: %v", name, err)
		}
	}
}

func collectGitStatuses(t *testing.T, sub <-chan types.WorkspaceStreamMessage, window time.Duration) []types.GitStatusUpdate {
	t.Helper()
	deadline := time.After(window)
	var statuses []types.GitStatusUpdate
	for {
		select {
		case msg := <-sub:
			if msg.Type == types.WorkspaceMessageTypeGitStatus && msg.GitStatus != nil {
				statuses = append(statuses, *msg.GitStatus)
			}
		case <-deadline:
			return statuses
		}
	}
}

func requireNoEmptyRepositoryStatus(t *testing.T, statuses []types.GitStatusUpdate, operation string) {
	t.Helper()
	for _, status := range statuses {
		if status.RepositoryName == "" {
			t.Fatalf("%s published a Git status for the plain parent tracker: %+v", operation, status)
		}
	}
}

// TestManager_LiveRepositoryAttachmentRefreshScopes pins the single-to-multi
// transition: a bare task-root tracker owns Files events but must never
// publish Git status. Explicit refresh paths are the ones that previously
// manufactured an empty-name refresh failure after the Files root was
// promoted above the agent's checkout.
func TestManager_LiveRepositoryAttachmentRefreshScopes(t *testing.T) {
	t.Run("plain parent tracker stays out of Git refresh publication", func(t *testing.T) {
		taskRoot := t.TempDir()
		placeSiblingRepos(t, taskRoot, "master", "secondary")

		mgr := NewManager(&config.InstanceConfig{WorkDir: taskRoot}, newTestLogger(t))
		t.Cleanup(mgr.stopWorkspaceTrackers)
		if mgr.workspaceTracker.gitIndexPath != "" {
			t.Fatalf("task root unexpectedly owns a Git index: %s", mgr.workspaceTracker.gitIndexPath)
		}

		statusCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		for _, tracker := range mgr.repoTrackers {
			if _, err := tracker.GetGitStatus(statusCtx, true); err != nil {
				t.Fatalf("capture %s status: %v", tracker.RepositoryName(), err)
			}
		}

		sub := mgr.SubscribeWorkspaceStream()
		defer mgr.UnsubscribeWorkspaceStream(sub)
		// Drain the initial per-repository replay before observing refreshes.
		collectGitStatuses(t, sub, 200*time.Millisecond)

		mgr.SetWorkspacePollMode(context.Background(), PollModeFast)
		requireNoEmptyRepositoryStatus(t, collectGitStatuses(t, sub, 700*time.Millisecond), "poll mode")

		mgr.UpdateBaseBranches(context.Background(), map[string]string{"master": "main", "secondary": "main"})
		requireNoEmptyRepositoryStatus(t, collectGitStatuses(t, sub, 700*time.Millisecond), "base branch")

		mgr.RefreshWorkspace(context.Background(), "turn_complete")
		requireNoEmptyRepositoryStatus(t, collectGitStatuses(t, sub, 700*time.Millisecond), "turn completion")
	})

	t.Run("real root repository still publishes beside named children", func(t *testing.T) {
		root, cleanup := setupTestRepo(t)
		t.Cleanup(cleanup)
		placeSiblingRepos(t, root, "secondary")

		mgr := NewManager(&config.InstanceConfig{WorkDir: root}, newTestLogger(t))
		t.Cleanup(mgr.stopWorkspaceTrackers)
		if mgr.workspaceTracker.gitIndexPath == "" {
			t.Fatal("expected the workspace root to own a Git index")
		}
		if err := mgr.RescanRepositories(context.Background(), ""); err != nil {
			t.Fatalf("rescan attached repository: %v", err)
		}
		scopes := mgr.RepositoryScopes()
		if len(scopes) != 2 || scopes[0] != "" || scopes[1] != "secondary" {
			t.Fatalf("RepositoryScopes = %v, want [\"\" secondary]", scopes)
		}

		statusCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := mgr.workspaceTracker.GetGitStatus(statusCtx, true); err != nil {
			t.Fatalf("capture root status: %v", err)
		}

		sub := mgr.SubscribeWorkspaceStream()
		defer mgr.UnsubscribeWorkspaceStream(sub)
		collectGitStatuses(t, sub, 200*time.Millisecond)

		// Change the root checkout so the explicit refresh cannot be deduped
		// against the status already replayed on subscribe.
		if err := os.WriteFile(filepath.Join(root, "attachment-change.txt"), []byte("changed\n"), 0o600); err != nil {
			t.Fatalf("write root change: %v", err)
		}
		mgr.SetWorkspacePollMode(context.Background(), PollModeFast)
		statuses := collectGitStatuses(t, sub, 2*time.Second)
		sawRoot := false
		for _, status := range statuses {
			if status.RepositoryName == "" && status.StatusState == gitStatusStateReady {
				sawRoot = true
			}
		}
		if !sawRoot {
			t.Fatalf("refresh skipped the valid root Git scope: %+v", statuses)
		}
	})
}
