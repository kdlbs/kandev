package process

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

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

func writeRepositoryFile(t *testing.T, repositoryPath, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repositoryPath, name), []byte(name+"\n"), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func statusContainsFile(status types.GitStatusUpdate, path string) bool {
	_, present := status.Files[path]
	return present
}

// assertTrackerHasFile forces one fresh status read and checks that the
// changed file is visible. The manager operation under test is invoked first;
// this read proves the repository state reached the tracker without relying on
// an arbitrary stream-observation window.
func assertTrackerHasFile(t *testing.T, tracker *WorkspaceTracker, path, description string) {
	t.Helper()
	status, err := tracker.GetGitStatus(context.Background(), true)
	if err != nil {
		t.Fatalf("%s: %v", description, err)
	}
	if !statusContainsFile(status, path) {
		keys := make([]string, 0, len(status.Files))
		for key := range status.Files {
			keys = append(keys, key)
		}
		t.Fatalf("%s: file %q missing; state=%s repo=%s files=%v", description, path, status.StatusState, status.RepositoryName, keys)
	}
}

// publishedGitStatusCollector drains the workspace stream for the whole test so
// no explicit refresh can publish an empty-name status between assertions.
type publishedGitStatusCollector struct {
	mu       sync.Mutex
	statuses []types.GitStatusUpdate
	stop     chan struct{}
	done     chan struct{}
}

func collectPublishedGitStatuses(sub types.WorkspaceStreamSubscriber) *publishedGitStatusCollector {
	collector := &publishedGitStatusCollector{
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	go func() {
		defer close(collector.done)
		for {
			select {
			case msg := <-sub:
				if msg.Type != types.WorkspaceMessageTypeGitStatus || msg.GitStatus == nil {
					continue
				}
				collector.mu.Lock()
				collector.statuses = append(collector.statuses, *msg.GitStatus)
				collector.mu.Unlock()
			case <-collector.stop:
				return
			}
		}
	}()
	return collector
}

func (c *publishedGitStatusCollector) stopAndCheckNoEmptyName(t *testing.T) {
	t.Helper()
	close(c.stop)
	<-c.done
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, status := range c.statuses {
		if status.RepositoryName == "" {
			t.Fatalf("bare parent tracker published Git status: %+v", status)
		}
	}
}

// TestManager_LiveRepositoryAttachmentRefreshScopes pins the single-to-multi
// transition: a bare task-root tracker owns Files events but must never
// publish Git status. Each explicit refresh path is driven by a real file
// change and verified through the tracker's accepted snapshot; the stream
// collector separately proves that no path published the empty repository
// name.
func TestManager_LiveRepositoryAttachmentRefreshScopes(t *testing.T) {
	t.Run("plain parent tracker stays out of Git refresh publication", func(t *testing.T) {
		taskRoot := t.TempDir()
		placeSiblingRepos(t, taskRoot, "master", "secondary")

		mgr := NewManager(&config.InstanceConfig{WorkDir: taskRoot}, newTestLogger(t))
		t.Cleanup(mgr.stopWorkspaceTrackers)
		if mgr.workspaceTracker.gitIndexPath != "" {
			t.Fatalf("task root unexpectedly owns a Git index: %s", mgr.workspaceTracker.gitIndexPath)
		}
		master := mgr.findRepositoryTracker("master")
		secondary := mgr.findRepositoryTracker("secondary")
		if master == nil || secondary == nil {
			t.Fatal("per-repository trackers were not created")
		}

		sub := mgr.SubscribeWorkspaceStream()
		defer mgr.UnsubscribeWorkspaceStream(sub)
		collector := collectPublishedGitStatuses(sub)

		writeRepositoryFile(t, filepath.Join(taskRoot, "master"), "poll-mode.txt")
		writeRepositoryFile(t, filepath.Join(taskRoot, "secondary"), "poll-mode.txt")
		mgr.SetWorkspacePollMode(context.Background(), PollModeFast)
		assertTrackerHasFile(t, master, "poll-mode.txt", "poll-mode refresh for master")
		assertTrackerHasFile(t, secondary, "poll-mode.txt", "poll-mode refresh for secondary")

		writeRepositoryFile(t, filepath.Join(taskRoot, "master"), "base-branch.txt")
		writeRepositoryFile(t, filepath.Join(taskRoot, "secondary"), "base-branch.txt")
		mgr.UpdateBaseBranches(context.Background(), map[string]string{"master": "main", "secondary": "main"})
		assertTrackerHasFile(t, master, "base-branch.txt", "base-branch refresh for master")
		assertTrackerHasFile(t, secondary, "base-branch.txt", "base-branch refresh for secondary")

		writeRepositoryFile(t, filepath.Join(taskRoot, "master"), "turn-complete.txt")
		writeRepositoryFile(t, filepath.Join(taskRoot, "secondary"), "turn-complete.txt")
		mgr.RefreshWorkspace(context.Background(), "turn_complete")
		assertTrackerHasFile(t, master, "turn-complete.txt", "turn-completion refresh for master")
		assertTrackerHasFile(t, secondary, "turn-complete.txt", "turn-completion refresh for secondary")

		collector.stopAndCheckNoEmptyName(t)
	})

	t.Run("repo-less root adopts suppression on rescan", func(t *testing.T) {
		taskRoot := t.TempDir()
		mgr := NewManager(&config.InstanceConfig{WorkDir: taskRoot}, newTestLogger(t))
		t.Cleanup(mgr.stopWorkspaceTrackers)
		placeSiblingRepos(t, taskRoot, "master", "secondary")

		if err := mgr.RescanRepositories(context.Background(), ""); err != nil {
			t.Fatalf("rescan repositories: %v", err)
		}
		if !mgr.workspaceTracker.suppressGitPublication {
			t.Fatal("rescanned bare root did not adopt Git publication suppression")
		}

		sub := mgr.SubscribeWorkspaceStream()
		defer mgr.UnsubscribeWorkspaceStream(sub)
		collector := collectPublishedGitStatuses(sub)

		writeRepositoryFile(t, filepath.Join(taskRoot, "master"), "rescan.txt")
		writeRepositoryFile(t, filepath.Join(taskRoot, "secondary"), "rescan.txt")
		mgr.SetWorkspacePollMode(context.Background(), PollModeFast)
		assertTrackerHasFile(t, mgr.findRepositoryTracker("master"), "rescan.txt", "rescanned master refresh")
		assertTrackerHasFile(t, mgr.findRepositoryTracker("secondary"), "rescan.txt", "rescanned secondary refresh")

		collector.stopAndCheckNoEmptyName(t)
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

		writeRepositoryFile(t, root, "root-change.txt")
		mgr.SetWorkspacePollMode(context.Background(), PollModeFast)

		rootTracker := mgr.GetWorkspaceTracker()
		assertTrackerHasFile(t, rootTracker, "root-change.txt", "root repository refresh")
	})
}
