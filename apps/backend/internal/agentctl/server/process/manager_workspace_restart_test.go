package process

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/pkg/agent"
)

const restartTestFileName = "README.md"

func newRestartManager(t *testing.T, cfg *config.InstanceConfig) *Manager {
	t.Helper()
	mgr := NewManager(cfg, newTestLogger(t))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := mgr.Stop(ctx); err != nil {
			t.Errorf("stop restart fixture: %v", err)
		}
	})
	return mgr
}

func persistentRestartConfig(repoDir string) *config.InstanceConfig {
	return &config.InstanceConfig{
		WorkDir:   repoDir,
		AgentArgs: fixtureArgs(),
		AgentEnv:  fixtureEnvSlice("cat"),
		Protocol:  agent.ProtocolACP,
	}
}

func startRestartManager(t *testing.T, mgr *Manager) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("start agent: %v", err)
	}
}

func stopRestartManager(t *testing.T, mgr *Manager) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := mgr.Stop(ctx); err != nil {
		t.Fatalf("stop agent: %v", err)
	}
}

func getRestartStatus(t *testing.T, tracker *WorkspaceTracker) types.GitStatusUpdate {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	status, err := tracker.GetGitStatusWithDetails(ctx, true)
	if err != nil {
		t.Fatalf("get fresh status for %q: %v", tracker.workDir, err)
	}
	return status
}

func assertRestartFile(t *testing.T, status types.GitStatusUpdate, path, marker string) {
	t.Helper()
	if marker == "" {
		t.Fatal("assertRestartFile requires a non-empty marker")
	}
	file, ok := status.Files[path]
	if !ok {
		t.Fatalf("status files missing %q: %+v", path, status.Files)
	}
	if !strings.Contains(file.Diff, marker) {
		t.Fatalf("file %q diff does not contain %q: %q", path, marker, file.Diff)
	}
}

func waitForSubscriberStatus(
	t *testing.T,
	sub types.WorkspaceStreamSubscriber,
	description string,
	predicate func(types.GitStatusUpdate) bool,
) types.GitStatusUpdate {
	t.Helper()
	deadline := time.After(30 * time.Second)
	for {
		select {
		case msg, ok := <-sub:
			if !ok {
				t.Fatalf("%s: subscriber channel closed before a matching status", description)
			}
			if msg.Type != types.WorkspaceMessageTypeGitStatus || msg.GitStatus == nil {
				continue
			}
			if predicate(*msg.GitStatus) {
				return *msg.GitStatus
			}
		case <-deadline:
			t.Fatalf("%s: timed out waiting for a matching status", description)
		}
	}
}

func TestManagerWorkspaceRestartSubscriberHandoffRace(t *testing.T) {
	tests := []struct {
		name         string
		replaceGraph func(t *testing.T, mgr *Manager, repoDir string)
	}{
		{
			name: "reconcile",
			replaceGraph: func(t *testing.T, mgr *Manager, _ string) {
				t.Helper()
				mgr.stopWorkspaceTrackers()
				mgr.renewWorkspaceTrackersForStart(context.Background())
			},
		},
		{
			name: "rebind",
			replaceGraph: func(t *testing.T, mgr *Manager, repoDir string) {
				t.Helper()
				if err := mgr.RebindWorkspace(context.Background(), repoDir); err != nil {
					t.Fatalf("rebind workspace: %v", err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repoDir, cleanup := setupTestRepo(t)
			defer cleanup()
			mgr := newRestartManager(t, &config.InstanceConfig{WorkDir: repoDir})
			oldRoot := mgr.GetWorkspaceTracker()
			sub := mgr.SubscribeWorkspaceStream()
			if got := len(oldRoot.workspaceStreamSubscribers); got != 1 {
				t.Fatalf("initial root subscriber count = %d, want 1", got)
			}

			unsubscribed := make(chan struct{})
			subscribed := make(chan types.WorkspaceStreamSubscriber, 1)
			mgr.workspaceGraphAfterPublish = func() {
				mgr.workspaceGraphAfterPublish = nil
				// Both calls block on streamSubscribersMu until the handoff
				// finishes detaching the retired tracker.
				go func() {
					mgr.UnsubscribeWorkspaceStream(sub)
					close(unsubscribed)
				}()
				go func() {
					subscribed <- mgr.SubscribeWorkspaceStream()
				}()
			}

			test.replaceGraph(t, mgr, repoDir)

			waitForRestartSignal(t, unsubscribed, "concurrent unsubscribe")
			var successorSub types.WorkspaceStreamSubscriber
			select {
			case successorSub = <-subscribed:
			case <-time.After(5 * time.Second):
				t.Fatal("timed out waiting for the delayed subscribe")
			}
			t.Cleanup(func() { mgr.UnsubscribeWorkspaceStream(successorSub) })

			newRoot := mgr.GetWorkspaceTracker()
			if newRoot == oldRoot {
				t.Fatal("graph handoff reused the retired root tracker")
			}
			if got := len(oldRoot.workspaceStreamSubscribers); got != 0 {
				t.Fatalf("retired tracker retained %d subscriptions, want 0", got)
			}
			if got := len(newRoot.workspaceStreamSubscribers); got != 1 {
				t.Fatalf("replacement tracker subscriber count = %d, want 1", got)
			}
			assertWorkspaceStreamClosed(t, sub, "unsubscribed predecessor")

			// A late publication from the retired tracker must not panic or
			// reach the successor channel.
			oldRoot.notifyWorkspaceStreamGitStatus(types.GitStatusUpdate{Branch: "retired-after-unsubscribe"})
			newRoot.notifyWorkspaceStreamGitStatus(types.GitStatusUpdate{Branch: "successor-proof"})
			deadline := time.After(5 * time.Second)
			for {
				select {
				case msg, ok := <-successorSub:
					if !ok {
						t.Fatal("successor subscriber closed before the replacement publication")
					}
					if msg.GitStatus == nil {
						continue
					}
					if msg.GitStatus.Branch == "retired-after-unsubscribe" {
						t.Fatal("retired publication reached the successor subscriber")
					}
					if msg.GitStatus.Branch == "successor-proof" {
						return
					}
				case <-deadline:
					t.Fatal("timed out waiting for the replacement publication")
				}
			}
		})
	}
}

func waitForRestartSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func assertWorkspaceStreamClosed(
	t *testing.T,
	sub types.WorkspaceStreamSubscriber,
	description string,
) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-sub:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatalf("%s workspace stream channel stayed open", description)
		}
	}
}

func TestManagerWorkspaceRestartFreshStatus(t *testing.T) {
	t.Run("persistent", func(t *testing.T) {
		repoDir, cleanup := setupTestRepo(t)
		defer cleanup()
		mgr := newRestartManager(t, persistentRestartConfig(repoDir))

		startRestartManager(t, mgr)
		oldRoot := mgr.GetWorkspaceTracker()
		if oldRoot == nil || !oldRoot.live() {
			t.Fatal("first start did not produce a live root tracker")
		}
		oldID := oldRoot.gitStatusTrackerID

		stopRestartManager(t, mgr)
		if oldRoot.live() {
			t.Fatal("Stop left the root tracker live")
		}
		writeFile(t, repoDir, restartTestFileName, "RESTART_PERSISTENT\n")

		startRestartManager(t, mgr)
		newRoot := mgr.GetWorkspaceTracker()
		status := getRestartStatus(t, newRoot)
		assertRestartFile(t, status, restartTestFileName, "RESTART_PERSISTENT")
		if newRoot == oldRoot {
			t.Fatal("restart reused the terminal root tracker")
		}
		if newRoot.gitStatusTrackerID == oldID {
			t.Fatal("replacement root tracker reused the retired tracker identity")
		}
		if mgr.processRunner.workspaceTracker != newRoot {
			t.Fatal("restart did not update process output routing to the replacement root tracker")
		}
	})

	t.Run("one-shot", func(t *testing.T) {
		repoDir, cleanup := setupTestRepo(t)
		defer cleanup()
		cfg := &config.InstanceConfig{
			WorkDir:      repoDir,
			AgentArgs:    []string{os.Args[0]},
			ContinueArgs: []string{os.Args[0]},
		}
		mgr := NewManager(cfg, newTestLogger(t))
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := mgr.Stop(ctx); err != nil {
				t.Errorf("stop one-shot restart fixture: %v", err)
			}
		})
		mgr.createAdapterFn = func() error {
			mgr.adapter = newOneShotStubAdapter()
			mgr.adapterCfg.OneShotConfig = &adapter.OneShotConfig{
				InitialArgs:  cfg.AgentArgs,
				ContinueArgs: cfg.ContinueArgs,
			}
			return nil
		}

		startRestartManager(t, mgr)
		oldRoot := mgr.GetWorkspaceTracker()
		stopRestartManager(t, mgr)
		writeFile(t, repoDir, restartTestFileName, "RESTART_ONE_SHOT\n")
		startRestartManager(t, mgr)
		if mgr.GetWorkspaceTracker() == oldRoot {
			t.Fatal("one-shot restart reused the terminal root tracker")
		}
		status := getRestartStatus(t, mgr.GetWorkspaceTracker())
		assertRestartFile(t, status, restartTestFileName, "RESTART_ONE_SHOT")
	})

	t.Run("stop before first start", func(t *testing.T) {
		repoDir, cleanup := setupTestRepo(t)
		defer cleanup()
		mgr := newRestartManager(t, persistentRestartConfig(repoDir))

		stopRestartManager(t, mgr)
		writeFile(t, repoDir, restartTestFileName, "RESTART_BEFORE_START\n")
		startRestartManager(t, mgr)
		status := getRestartStatus(t, mgr.GetWorkspaceTracker())
		assertRestartFile(t, status, restartTestFileName, "RESTART_BEFORE_START")
	})
}

func TestManagerWorkspaceRestartExistingSubscriber(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()
	mgr := newRestartManager(t, persistentRestartConfig(repoDir))

	startRestartManager(t, mgr)
	oldRoot := mgr.GetWorkspaceTracker()
	getRestartStatus(t, oldRoot)
	sub := mgr.SubscribeWorkspaceStream()
	defer mgr.UnsubscribeWorkspaceStream(sub)

	stopRestartManager(t, mgr)
	writeFile(t, repoDir, restartTestFileName, "RESTART_SUBSCRIBER\n")
	startRestartManager(t, mgr)

	status := waitForSubscriberStatus(t, sub, "restart subscriber", func(status types.GitStatusUpdate) bool {
		file, ok := status.Files[restartTestFileName]
		return status.DetailState == gitStatusDetailReady && ok && strings.Contains(file.Diff, "RESTART_SUBSCRIBER")
	})
	if status.TrackerID == oldRoot.gitStatusTrackerID {
		t.Fatal("subscriber received a snapshot from the retired tracker identity")
	}
	if !status.Timestamp.After(time.Time{}) {
		t.Fatal("subscriber received an empty replacement snapshot")
	}
}

func TestManagerWorkspaceRestartLiveGraph(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()
	mgr := newRestartManager(t, persistentRestartConfig(repoDir))

	startRestartManager(t, mgr)
	root := mgr.GetWorkspaceTracker()
	if err := os.Mkdir(filepath.Join(repoDir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	lazy, err := mgr.GetWorkspaceTrackerFor("nested")
	if err != nil {
		t.Fatalf("create live lazy tracker: %v", err)
	}
	sub := mgr.SubscribeWorkspaceStream()
	defer mgr.UnsubscribeWorkspaceStream(sub)

	mgr.renewWorkspaceTrackersForStart(context.Background())

	if mgr.GetWorkspaceTracker() != root {
		t.Fatal("renewal replaced a live root tracker")
	}
	reusedLazy, err := mgr.GetWorkspaceTrackerFor("nested")
	if err != nil {
		t.Fatalf("re-read live lazy tracker: %v", err)
	}
	if reusedLazy != lazy {
		t.Fatal("renewal replaced a live lazy tracker")
	}
	if got := len(root.workspaceStreamSubscribers); got != 1 {
		t.Fatalf("root subscriber count after renewal = %d, want 1", got)
	}
}

func TestManagerWorkspaceRestartMixedGraph(t *testing.T) {
	taskRoot := t.TempDir()
	healthy := filepath.Join(taskRoot, "healthy")
	stopped := filepath.Join(taskRoot, "stopped")
	initGitRepoAt(t, healthy)
	initGitRepoAt(t, stopped)
	mgr := newRestartManager(t, persistentRestartConfig(taskRoot))

	startRestartManager(t, mgr)
	if len(mgr.repoTrackers) != 2 {
		t.Fatalf("initial repository trackers = %d, want 2", len(mgr.repoTrackers))
	}
	root := mgr.GetWorkspaceTracker()
	healthyTracker := mgr.findRepositoryTracker("healthy")
	stoppedTracker := mgr.findRepositoryTracker("stopped")
	stoppedTracker.Stop()
	writeFile(t, healthy, "healthy.txt", "HEALTHY_SCOPE\n")
	writeFile(t, stopped, "stopped.txt", "STOPPED_SCOPE\n")

	mgr.renewWorkspaceTrackersForStart(context.Background())

	if mgr.GetWorkspaceTracker() != root {
		t.Fatal("mixed renewal replaced the live root tracker")
	}
	if mgr.findRepositoryTracker("healthy") != healthyTracker {
		t.Fatal("mixed renewal replaced the live repository tracker")
	}
	replacement := mgr.findRepositoryTracker("stopped")
	if replacement == nil || replacement == stoppedTracker || !replacement.live() {
		t.Fatal("mixed renewal did not replace the stopped repository tracker")
	}
	if replacement.gitStatusTrackerID == stoppedTracker.gitStatusTrackerID {
		t.Fatal("replacement repository tracker reused the retired tracker identity")
	}
	if healthyTracker.gitStatusTrackerID == "" || !healthyTracker.live() {
		t.Fatal("healthy repository tracker lost its live identity")
	}
	assertRestartFile(t, getRestartStatus(t, healthyTracker), "healthy.txt", "HEALTHY_SCOPE")
	assertRestartFile(t, getRestartStatus(t, replacement), "stopped.txt", "STOPPED_SCOPE")
}

func TestManagerWorkspaceRestartRetiresOldPublication(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()
	mgr := newRestartManager(t, persistentRestartConfig(repoDir))

	startRestartManager(t, mgr)
	oldRoot := mgr.GetWorkspaceTracker()
	getRestartStatus(t, oldRoot)
	sub := mgr.SubscribeWorkspaceStream()
	defer mgr.UnsubscribeWorkspaceStream(sub)

	stopRestartManager(t, mgr)
	mgr.renewWorkspaceTrackersForStart(context.Background())
	newRoot := mgr.GetWorkspaceTracker()
	if newRoot == oldRoot {
		t.Fatal("renewal reused the terminal tracker")
	}
	if got := len(oldRoot.workspaceStreamSubscribers); got != 0 {
		t.Fatalf("retired tracker still holds %d subscribers", got)
	}

	oldRoot.notifyWorkspaceStreamGitStatus(types.GitStatusUpdate{Branch: "retired-proof"})
	select {
	case msg := <-sub:
		if msg.GitStatus != nil && msg.GitStatus.Branch == "retired-proof" {
			t.Fatal("retired tracker published into the replacement lifetime")
		}
	case <-time.After(100 * time.Millisecond):
	}
	newRoot.notifyWorkspaceStreamGitStatus(types.GitStatusUpdate{Branch: "replacement-proof"})
	status := waitForSubscriberStatus(t, sub, "replacement publication", func(status types.GitStatusUpdate) bool {
		return status.Branch == "replacement-proof"
	})
	if status.Branch != "replacement-proof" {
		t.Fatalf("replacement publication branch = %q", status.Branch)
	}
}

func TestManagerWorkspaceRestartScopes(t *testing.T) {
	taskRoot := t.TempDir()
	healthy := filepath.Join(taskRoot, "healthy")
	broken := filepath.Join(taskRoot, "broken")
	initGitRepoAt(t, healthy)
	initGitRepoAt(t, broken)
	mgr := newRestartManager(t, persistentRestartConfig(taskRoot))
	if got := mgr.RepoSubpaths(); len(got) != 2 {
		t.Fatalf("initial repository scopes = %v, want two", got)
	}

	stopRestartManager(t, mgr)
	writeFile(t, healthy, "healthy.txt", "SCOPE_AFTER_RESTART\n")
	if err := os.WriteFile(filepath.Join(broken, ".git", "index"), []byte("corrupt"), 0o644); err != nil {
		t.Fatalf("corrupt repository index: %v", err)
	}
	mgr.renewWorkspaceTrackersForStart(context.Background())

	scopes := mgr.RepoSubpaths()
	if len(scopes) != 2 || scopes[0] != "broken" || scopes[1] != "healthy" {
		t.Fatalf("repository scopes after restart = %v, want [broken healthy]", scopes)
	}
	healthyTracker := mgr.findRepositoryTracker("healthy")
	if healthyTracker == nil || !healthyTracker.live() {
		t.Fatal("healthy repository tracker is not live after restart")
	}
	assertRestartFile(t, getRestartStatus(t, healthyTracker), "healthy.txt", "SCOPE_AFTER_RESTART")
	brokenTracker := mgr.findRepositoryTracker("broken")
	if brokenTracker == nil {
		t.Fatal("broken repository scope was dropped instead of remaining unavailable")
	}
	if _, err := brokenTracker.GetGitStatus(context.Background(), true); err == nil {
		t.Fatal("broken repository unexpectedly produced a usable status")
	}
}

func TestManagerWorkspaceRestartPlainTaskRoot(t *testing.T) {
	taskRoot := t.TempDir()
	writeFile(t, taskRoot, "notes.txt", "PLAIN_TASK_ROOT\n")
	mgr := newRestartManager(t, persistentRestartConfig(taskRoot))

	startRestartManager(t, mgr)
	stopRestartManager(t, mgr)
	writeFile(t, taskRoot, "notes.txt", "PLAIN_TASK_ROOT_UPDATED\n")
	mgr.renewWorkspaceTrackersForStart(context.Background())

	root := mgr.GetWorkspaceTracker()
	if !root.live() || root.gitIndexPath != "" {
		t.Fatal("plain task root became a Git scope after restart")
	}
	if len(mgr.repoTrackers) != 0 || len(mgr.RepositoryScopes()) != 0 {
		t.Fatalf("plain task root gained repository scopes: %v", mgr.RepositoryScopes())
	}
	tree, err := root.GetFileTree("", 1)
	if err != nil {
		t.Fatalf("read plain task root files: %v", err)
	}
	found := false
	for _, child := range tree.Children {
		if child.Path == "notes.txt" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("plain task root tree does not include notes.txt: %+v", tree.Children)
	}
}

func TestManagerWorkspaceRestartSubmoduleScope(t *testing.T) {
	parent, parentCleanup := setupTestRepo(t)
	defer parentCleanup()
	child, childCleanup := setupTestRepo(t)
	defer childCleanup()
	runGit(t, parent, "-c", "protocol.file.allow=always", "submodule", "add", child, "vendor/lib")
	runGit(t, parent, "add", ".")
	runGit(t, parent, "commit", "-m", "add submodule")
	runGit(t, parent, "push", "origin", "main")

	cfg := persistentRestartConfig(parent)
	cfg.BaseBranches = map[string]string{"": "main"}
	mgr := newRestartManager(t, cfg)
	oldTracker := mgr.findRepositoryTracker("vendor/lib")
	if oldTracker == nil || !oldTracker.IsSubmodule() {
		t.Fatal("initial graph did not build an initialized submodule tracker")
	}
	anchor := oldTracker.BaseBranch()
	if anchor == "" {
		t.Fatal("initial submodule tracker has no comparison anchor")
	}

	startRestartManager(t, mgr)
	stopRestartManager(t, mgr)
	writeFile(t, filepath.Join(parent, "vendor", "lib"), "submodule.txt", "SUBMODULE_AFTER_RESTART\n")
	mgr.renewWorkspaceTrackersForStart(context.Background())

	newTracker := mgr.findRepositoryTracker("vendor/lib")
	if newTracker == nil || newTracker == oldTracker || !newTracker.live() {
		t.Fatal("submodule tracker was not replaced after restart")
	}
	if !newTracker.IsSubmodule() || newTracker.BaseBranch() != anchor {
		t.Fatalf("replacement submodule anchor = %q, want %q", newTracker.BaseBranch(), anchor)
	}
	status := getRestartStatus(t, newTracker)
	if status.RepositoryName != "vendor/lib" || !status.IsSubmodule {
		t.Fatalf("replacement submodule status identity = %+v", status)
	}
	assertRestartFile(t, status, "submodule.txt", "SUBMODULE_AFTER_RESTART")
}

func TestGetWorkspaceTrackerForReplacesStoppedLazyTracker(t *testing.T) {
	taskRoot := t.TempDir()
	lazyRepo := filepath.Join(taskRoot, "nested", "repo")
	initGitRepoAt(t, lazyRepo)
	mgr := newRestartManager(t, &config.InstanceConfig{WorkDir: taskRoot})

	oldLazy, err := mgr.GetWorkspaceTrackerFor("nested/repo")
	if err != nil {
		t.Fatalf("create lazy tracker: %v", err)
	}
	oldLazy.Stop()
	replacement, err := mgr.GetWorkspaceTrackerFor("nested/repo")
	if err != nil {
		t.Fatalf("replace stopped lazy tracker: %v", err)
	}
	if replacement == oldLazy || !replacement.live() {
		t.Fatal("lazy lookup reused a terminal tracker")
	}
}

func TestGetWorkspaceTrackerForRejectsCreationAfterTeardown(t *testing.T) {
	taskRoot := t.TempDir()
	initGitRepoAt(t, filepath.Join(taskRoot, "nested", "repo"))
	mgr := newRestartManager(t, &config.InstanceConfig{WorkDir: taskRoot})

	mgr.CloseAdmission()
	if _, err := mgr.GetWorkspaceTrackerFor("nested/repo"); !errors.Is(err, ErrManagerStopping) {
		t.Fatalf("lazy creation after admission close = %v, want ErrManagerStopping", err)
	}
}

func TestManagerWorkspaceRestartLazyTracker(t *testing.T) {
	taskRoot := t.TempDir()
	lazyRepo := filepath.Join(taskRoot, "nested", "repo")
	initGitRepoAt(t, lazyRepo)
	mgr := newRestartManager(t, persistentRestartConfig(taskRoot))

	startRestartManager(t, mgr)
	oldLazy, err := mgr.GetWorkspaceTrackerFor("nested/repo")
	if err != nil {
		t.Fatalf("create lazy tracker: %v", err)
	}
	mgr.renewWorkspaceTrackersForStart(context.Background())
	liveLazy, err := mgr.GetWorkspaceTrackerFor("nested/repo")
	if err != nil || liveLazy != oldLazy {
		t.Fatalf("live lazy tracker was not retained: tracker=%p err=%v", liveLazy, err)
	}

	stopRestartManager(t, mgr)
	writeFile(t, lazyRepo, "lazy.txt", "LAZY_AFTER_RESTART\n")
	mgr.renewWorkspaceTrackersForStart(context.Background())
	if _, ok := mgr.workspaceTrackersBySubpath["nested/repo"]; ok {
		t.Fatal("stopped lazy tracker was not pruned from the cache")
	}
	newLazy, err := mgr.GetWorkspaceTrackerFor("nested/repo")
	if err != nil {
		t.Fatalf("recreate lazy tracker: %v", err)
	}
	if newLazy == oldLazy || !newLazy.live() {
		t.Fatal("stopped lazy tracker was not replaced with a live tracker")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	status, err := newLazy.GetGitStatus(ctx, true)
	if err != nil {
		t.Fatalf("get lazy status after restart: %v", err)
	}
	if _, ok := status.Files["lazy.txt"]; !ok && !slices.Contains(status.Untracked, "lazy.txt") {
		t.Fatalf("lazy status does not report lazy.txt: files=%+v untracked=%v", status.Files, status.Untracked)
	}
}

func TestManagerWorkspaceRestartRebindsCachedGitOperators(t *testing.T) {
	taskRoot := t.TempDir()
	initGitRepoAt(t, filepath.Join(taskRoot, "alpha"))
	initGitRepoAt(t, filepath.Join(taskRoot, "beta"))
	mgr := newRestartManager(t, &config.InstanceConfig{WorkDir: taskRoot})

	rootOperator := mgr.GitOperator()
	alphaOperator, err := mgr.GitOperatorFor("alpha")
	if err != nil {
		t.Fatalf("create alpha operator: %v", err)
	}
	if rootOperator.currentWorkspaceTracker() != mgr.GetWorkspaceTracker() {
		t.Fatal("root operator did not bind the initial root tracker")
	}
	if alphaOperator.currentWorkspaceTracker() != mgr.findRepositoryTracker("alpha") {
		t.Fatal("alpha operator did not bind the initial repository tracker")
	}

	mgr.stopWorkspaceTrackers()
	mgr.renewWorkspaceTrackersForStart(context.Background())

	if mgr.GitOperator() != rootOperator {
		t.Fatal("renewal recreated the cached root operator instead of rebinding it")
	}
	if rootOperator.currentWorkspaceTracker() != mgr.GetWorkspaceTracker() {
		t.Fatal("cached root operator still publishes through the retired tracker")
	}
	if alphaOperator.currentWorkspaceTracker() != mgr.findRepositoryTracker("alpha") {
		t.Fatal("cached repository operator still publishes through the retired tracker")
	}
}

func TestManagerWorkspaceRestartPreparesComparisonTarget(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()
	target := comparisonTargetProcessTestTarget()
	mgr, _ := newComparisonTargetTestManager(t, repoDir, target, nil)

	mgr.stopWorkspaceTrackers()
	mgr.renewWorkspaceTrackersForStart(context.Background())

	resolution := waitForComparisonResolution(t, mgr.GetWorkspaceTracker(), comparisonTargetStatusReady, "")
	if resolution.Ref != target.ComparisonRef() {
		t.Fatalf("replacement comparison ref = %q, want %q", resolution.Ref, target.ComparisonRef())
	}
}

func TestManagerWorkspaceRestartConfiguration(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()
	mgr := newRestartManager(t, persistentRestartConfig(repoDir))
	excludedPath := filepath.Join(repoDir, "recovery-artifact")
	sourceRoot := canonicalSourceRoot(t, repoDir)

	mgr.SetWorkspaceSourceRoots([]string{repoDir})
	mgr.SetWorkspaceFileExclusions([]string{excludedPath})
	mgr.UpdateBaseBranches(context.Background(), map[string]string{"": "main"})
	mgr.SetWorkspacePollMode(context.Background(), PollModePaused)

	stopRestartManager(t, mgr)
	mgr.renewWorkspaceTrackersForStart(context.Background())
	root := mgr.GetWorkspaceTracker()
	if got := root.BaseBranch(); got != "main" {
		t.Fatalf("base branch after restart = %q, want main", got)
	}
	if len(root.allowedSourceRoots) != 1 || root.allowedSourceRoots[0] != sourceRoot {
		t.Fatalf("allowed source roots after restart = %q, want %q", root.allowedSourceRoots, sourceRoot)
	}
	if len(root.recoveryArtifactExclusions) != 1 || root.recoveryArtifactExclusions[0] != "recovery-artifact" {
		t.Fatalf("recovery exclusions after restart = %q, want [recovery-artifact]", root.recoveryArtifactExclusions)
	}
	if got := root.GetPollMode(); got != PollModePaused {
		t.Fatalf("poll mode after restart = %q, want %q", got, PollModePaused)
	}
}

func TestManagerWorkspaceRestartCleanup(t *testing.T) {
	t.Run("repeated cycles", func(t *testing.T) {
		repoDir, cleanup := setupTestRepo(t)
		defer cleanup()
		mgr := newRestartManager(t, persistentRestartConfig(repoDir))

		seen := make(map[*WorkspaceTracker]struct{})
		for cycle := 0; cycle < 3; cycle++ {
			startRestartManager(t, mgr)
			root := mgr.GetWorkspaceTracker()
			if _, duplicate := seen[root]; duplicate {
				t.Fatalf("cycle %d reused a retired tracker", cycle)
			}
			seen[root] = struct{}{}
			writeFile(t, repoDir, restartTestFileName, "RESTART_CYCLE\n")
			assertRestartFile(t, getRestartStatus(t, root), restartTestFileName, "RESTART_CYCLE")
			stopRestartManager(t, mgr)
		}
		stopRestartManager(t, mgr)
		if mgr.GetWorkspaceTracker().live() {
			t.Fatal("final Stop left a live root tracker")
		}
		if _, err := mgr.GetWorkspaceTracker().GetGitStatus(context.Background(), true); !errors.Is(err, context.Canceled) {
			t.Fatalf("final tracker read after Stop = %v, want context.Canceled", err)
		}
		if gitIndexPath := mgr.GetWorkspaceTracker().gitIndexPath; gitIndexPath != "" {
			assertNoIndexSnapshots(t, filepath.Dir(gitIndexPath))
		}
	})

	t.Run("canceled waiter and unsubscribe", func(t *testing.T) {
		repoDir, cleanup := setupTestRepo(t)
		defer cleanup()
		mgr := newRestartManager(t, persistentRestartConfig(repoDir))

		startRestartManager(t, mgr)
		stopRestartManager(t, mgr)
		mgr.renewWorkspaceTrackersForStart(context.Background())
		root := mgr.GetWorkspaceTracker()

		canceledCtx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := root.GetGitStatus(canceledCtx, true); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled caller error = %v, want context.Canceled", err)
		}
		getRestartStatus(t, root)

		sub := mgr.SubscribeWorkspaceStream()
		mgr.UnsubscribeWorkspaceStream(sub)
		deadline := time.After(5 * time.Second)
		closed := false
		for !closed {
			select {
			case _, ok := <-sub:
				closed = !ok
			case <-deadline:
				t.Fatal("unsubscribed workspace stream channel stayed open")
			}
		}
		mgr.renewWorkspaceTrackersForStart(context.Background())
		if !root.live() {
			t.Fatal("unsubscribe race stopped the live replacement tracker")
		}
	})
}
