package lifecycle

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

func TestMainCheckoutWorkspaceRestore(t *testing.T) {
	isolateGitEnv(t)
	const (
		taskID        = "task-main-checkout-restore"
		sessionID     = "session-main-checkout-restore"
		environmentID = "environment-main-checkout-restore"
		repositoryID  = "repository-main-checkout-restore"
		worktreeID    = "worktree-main-checkout-restore"
	)
	repositoryPath, before := newLifecycleMainCheckout(t)
	worktreeRecord := &worktree.Worktree{
		ID: worktreeID, TaskID: taskID, SessionID: sessionID, TaskEnvironmentID: environmentID,
		TaskDirName: "main-checkout-restore", RepositoryID: repositoryID, BranchSlug: "main",
		RepositoryPath: repositoryPath, Path: repositoryPath, Branch: before.branch,
		Status: worktree.StatusActive,
	}
	store := &lifecycleMainCheckoutStore{worktree: worktreeRecord}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	t.Cleanup(func() { _ = log.Close() })
	worktreeManager, err := worktree.NewManager(worktree.Config{
		Enabled: true, TasksBasePath: filepath.Join(t.TempDir(), "managed-tasks"), BranchPrefix: "feature/",
	}, store, log)
	if err != nil {
		t.Fatalf("create worktree manager: %v", err)
	}

	info := &WorkspaceInfo{
		TaskID: taskID, SessionID: sessionID, TaskEnvironmentID: environmentID,
		EnvironmentOwnerTaskID: taskID, OwnershipGeneration: 1,
		ValidatedTaskEnvironmentID: environmentID, ValidatedExecutorType: string(models.ExecutorTypeWorktree),
		ValidatedTaskEnvironmentGeneration: 1,
		WorkspacePath:                      repositoryPath, WorkspaceID: "workspace-main-checkout-restore",
		TaskDirName: "main-checkout-restore", ExecutorType: string(models.ExecutorTypeWorktree), AgentID: "auggie",
		RecoveryErrorObservation: &models.WorkspaceRecoveryErrorObservation{
			TaskID: taskID, SessionID: sessionID, TaskEnvironmentID: environmentID,
			EnvironmentOwnerTaskID: taskID, OwnershipGeneration: 1,
			SelectionSnapshot: models.WorkspaceRecoverySelectionSnapshot{
				TaskID: taskID, SessionID: sessionID, SessionPersisted: true,
				SessionTaskEnvironmentID: environmentID, TaskEnvironmentID: environmentID,
				EnvironmentOwnerTaskID: taskID, OwnershipGeneration: 1,
				ExecutorType: string(models.ExecutorTypeWorktree),
				Slots: []models.WorkspaceRecoveryInventorySlot{{
					EnvironmentRepoID: "environment-repository-main-checkout-restore",
					RepositoryID:      repositoryID, BranchSlug: "main", WorktreeID: worktreeID,
					WorktreePath: repositoryPath, WorktreeBranch: before.branch, Status: "active",
					RepositoryPresent: true, RepositoryLocalPath: repositoryPath,
				}},
			},
		},
		WorkspaceRepositories: []WorkspaceRepositorySpec{{
			RepositoryID: repositoryID, RepositoryPath: repositoryPath, WorktreePath: repositoryPath,
			WorktreeBranch: before.branch, BaseBranch: "main", RepoName: "repository",
			WorktreeID: worktreeID, BranchSlug: "main",
		}},
	}
	store.selection = info.RecoveryErrorObservation.SelectionSnapshot
	provider := &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{sessionID: info}}
	mgr, backend := newEnvironmentExecutionTestManager(t, provider)
	mgr.SetWorktreeManager(worktreeManager)
	mgr.SetExecutorProfileReader(&fakeExecutorProfileReader{
		task: &models.Task{ID: taskID},
		session: &models.TaskSession{
			ID: sessionID, TaskID: taskID, TaskEnvironmentID: environmentID,
			State: models.TaskSessionStateFailed,
		},
		env: &models.TaskEnvironment{
			ID: environmentID, TaskID: taskID, Status: models.TaskEnvironmentStatusStopped,
			OwnershipGeneration: 1, ExecutorType: string(models.ExecutorTypeWorktree),
		},
	})

	execution, err := mgr.GetOrEnsureExecution(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetOrEnsureExecution: %v", err)
	}
	if execution == nil {
		t.Fatal("GetOrEnsureExecution returned nil execution")
	}
	if execution.WorkspacePath != repositoryPath {
		t.Fatalf("execution workspace = %q, want canonical main checkout %q", execution.WorkspacePath, repositoryPath)
	}
	if execution.AgentCommand != "" {
		t.Fatalf("workspace-only restoration has agent command %q", execution.AgentCommand)
	}
	if got := backend.createCount.Load(); got != 1 {
		t.Fatalf("runtime creation count = %d, want 1", got)
	}
	if backend.lastRequest == nil || backend.lastRequest.WorkspacePath != repositoryPath {
		t.Fatalf("runtime workspace request = %+v, want canonical main checkout %q", backend.lastRequest, repositoryPath)
	}
	if len(backend.lastRequest.WorkspaceSourceRoots) != 1 || backend.lastRequest.WorkspaceSourceRoots[0] != repositoryPath {
		t.Fatalf("runtime source roots = %v, want the selected main checkout %q", backend.lastRequest.WorkspaceSourceRoots, repositoryPath)
	}
	readyDeadline := time.Now().Add(2 * time.Second)
	for !execution.IsAgentctlReady() && time.Now().Before(readyDeadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !execution.IsAgentctlReady() {
		t.Fatal("workspace-only runtime did not become ready before test cleanup")
	}
	if info.WorktreeRecoveryAdmitted != true {
		t.Fatal("workspace restoration did not pass through selected worktree recovery admission")
	}
	if len(info.WorkspaceRepositories) != 1 || info.WorkspaceRepositories[0].WorktreeID != worktreeID ||
		info.WorkspaceRepositories[0].WorktreePath != repositoryPath || info.WorkspaceRepositories[0].RepositoryPath != repositoryPath {
		t.Fatalf("restored repository inventory = %+v, want the original main-checkout slot", info.WorkspaceRepositories)
	}
	if store.worktree != worktreeRecord || store.worktree.Path != repositoryPath {
		t.Fatal("workspace restoration replaced the canonical worktree record")
	}
	assertLifecycleMainCheckoutState(t, repositoryPath, before)
	if _, err := os.Lstat(repositoryPath + ".kandev-recovery.json"); !os.IsNotExist(err) {
		t.Fatalf("main checkout received recovery record: %v", err)
	}
}

type lifecycleMainCheckoutStore struct {
	worktree.Store
	worktree  *worktree.Worktree
	selection models.WorkspaceRecoverySelectionSnapshot
}

func (s *lifecycleMainCheckoutStore) GetWorktreeByID(_ context.Context, id string) (*worktree.Worktree, error) {
	if s.worktree == nil || s.worktree.ID != id {
		return nil, nil
	}
	return s.worktree, nil
}

func (s *lifecycleMainCheckoutStore) ReadRecoverySelectionSnapshot(
	context.Context,
	models.WorkspaceRecoverySelectionSnapshot,
) (models.WorkspaceRecoverySelectionSnapshot, error) {
	return s.selection.Canonical(), nil
}

type lifecycleMainCheckoutSnapshot struct {
	branch string
	head   string
	index  []byte
	files  map[string][]byte
}

func newLifecycleMainCheckout(t *testing.T) (string, lifecycleMainCheckoutSnapshot) {
	t.Helper()
	path := initGitRepo(t)
	writeLifecycleMainCheckoutFile(t, path, ".gitignore", "ignored.txt\n")
	writeLifecycleMainCheckoutFile(t, path, "tracked.txt", "committed\n")
	runLifecycleMainCheckoutGit(t, path, "add", ".gitignore", "tracked.txt")
	runLifecycleMainCheckoutGit(t, path, "commit", "-m", "checkout state")
	writeLifecycleMainCheckoutFile(t, path, "tracked.txt", "unstaged edit\n")
	writeLifecycleMainCheckoutFile(t, path, "staged.txt", "staged edit\n")
	runLifecycleMainCheckoutGit(t, path, "add", "staged.txt")
	writeLifecycleMainCheckoutFile(t, path, "untracked.txt", "untracked data\n")
	writeLifecycleMainCheckoutFile(t, path, "ignored.txt", "ignored data\n")
	return path, captureLifecycleMainCheckoutState(t, path)
}

func writeLifecycleMainCheckoutFile(t *testing.T, root, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o600); err != nil {
		t.Fatalf("write main checkout file %q: %v", name, err)
	}
}

func captureLifecycleMainCheckoutState(t *testing.T, path string) lifecycleMainCheckoutSnapshot {
	t.Helper()
	snapshot := lifecycleMainCheckoutSnapshot{branch: currentBranch(t, path), files: make(map[string][]byte)}
	snapshot.head = runLifecycleMainCheckoutGitOutput(t, path, "rev-parse", "--verify", "HEAD")
	var err error
	snapshot.index, err = os.ReadFile(filepath.Join(path, ".git", "index"))
	if err != nil {
		t.Fatalf("read main checkout index: %v", err)
	}
	for _, name := range []string{"tracked.txt", "staged.txt", "untracked.txt", "ignored.txt"} {
		snapshot.files[name], err = os.ReadFile(filepath.Join(path, name))
		if err != nil {
			t.Fatalf("read main checkout file %q: %v", name, err)
		}
	}
	return snapshot
}

func assertLifecycleMainCheckoutState(t *testing.T, path string, want lifecycleMainCheckoutSnapshot) {
	t.Helper()
	got := captureLifecycleMainCheckoutState(t, path)
	if got.branch != want.branch || got.head != want.head || !bytes.Equal(got.index, want.index) {
		t.Fatalf("main checkout identity/index changed: got branch=%q head=%q index=%x, want branch=%q head=%q index=%x",
			got.branch, got.head, got.index, want.branch, want.head, want.index)
	}
	for name, contents := range want.files {
		if !bytes.Equal(got.files[name], contents) {
			t.Fatalf("main checkout file %q changed: got %q, want %q", name, got.files[name], contents)
		}
	}
}

func runLifecycleMainCheckoutGit(t *testing.T, directory string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = directory
	cmd.Env = newIsolatedGitEnv()
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}

func runLifecycleMainCheckoutGitOutput(t *testing.T, directory string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = directory
	cmd.Env = newIsolatedGitEnv()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}
