package lifecycle

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	"github.com/kandev/kandev/pkg/api/v1"
)

func TestWorkspaceRestoreTerminalAdmission(t *testing.T) {
	for _, state := range []models.TaskSessionState{
		models.TaskSessionStateFailed,
		models.TaskSessionStateCompleted,
		models.TaskSessionStateCancelled,
	} {
		t.Run(string(state), func(t *testing.T) {
			const sessionID = "session-workspace-restore"
			mgr, backend := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{
				infos: map[string]*WorkspaceInfo{
					sessionID: {
						TaskID:        "task-workspace-restore",
						SessionID:     sessionID,
						WorkspacePath: "/workspace/task",
						AgentID:       "auggie",
					},
				},
			})
			mgr.SetExecutorProfileReader(&fakeExecutorProfileReader{session: &models.TaskSession{
				ID: sessionID, TaskID: "task-workspace-restore", State: state,
			}})

			execution, err := mgr.GetOrEnsureExecution(context.Background(), sessionID)
			if err != nil {
				t.Fatalf("GetOrEnsureExecution returned error: %v", err)
			}
			if execution == nil {
				t.Fatal("GetOrEnsureExecution returned nil execution")
			}
			if got := backend.createCount.Load(); got != 1 {
				t.Fatalf("runtime creation count = %d, want 1", got)
			}
			if execution.AgentCommand != "" {
				t.Fatalf("workspace-only execution has agent command %q", execution.AgentCommand)
			}
		})
	}
}

func TestWorkspaceRestoreAdmissionRejectsInvalidOwners(t *testing.T) {
	archivedAt := time.Now()
	tests := []struct {
		name       string
		reader     *fakeExecutorProfileReader
		configure  func(*Manager)
		want       error
		wantCreate int32
	}{
		{
			name:       "archived task",
			reader:     &fakeExecutorProfileReader{task: &models.Task{ID: "task-workspace-restore", ArchivedAt: &archivedAt}},
			want:       ErrSessionTerminal,
			wantCreate: 0,
		},
		{
			name:       "missing task",
			reader:     &fakeExecutorProfileReader{task: &models.Task{ID: "different-task"}},
			wantCreate: 0,
		},
		{
			name:       "cleanup active",
			reader:     &fakeExecutorProfileReader{cleanupActive: true},
			want:       errTaskCleanupActive,
			wantCreate: 0,
		},
		{
			name:   "foreign session",
			reader: &fakeExecutorProfileReader{},
			configure: func(mgr *Manager) {
				mgr.SetSessionExecAccessChecker(func(context.Context, string) error {
					return errors.New("session access denied")
				})
			},
			wantCreate: 0,
		},
		{
			name: "ambiguous task ownership",
			reader: &fakeExecutorProfileReader{session: &models.TaskSession{
				ID: "session-workspace-restore", State: models.TaskSessionStateFailed,
			}},
			wantCreate: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr, backend := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{
				infos: map[string]*WorkspaceInfo{
					"session-workspace-restore": {
						TaskID:        "task-workspace-restore",
						SessionID:     "session-workspace-restore",
						WorkspacePath: "/workspace/task",
						AgentID:       "auggie",
					},
				},
			})
			if tt.reader.session == nil {
				tt.reader.session = &models.TaskSession{
					ID: "session-workspace-restore", TaskID: "task-workspace-restore", State: models.TaskSessionStateFailed,
				}
			}
			mgr.SetExecutorProfileReader(tt.reader)
			if tt.configure != nil {
				tt.configure(mgr)
			}

			_, err := mgr.EnsureWorkspaceExecutionForSession(
				context.Background(), "task-workspace-restore", "session-workspace-restore",
			)
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
			if err == nil {
				t.Fatal("workspace restore unexpectedly succeeded")
			}
			if got := backend.createCount.Load(); got != tt.wantCreate {
				t.Fatalf("runtime creation count = %d, want %d", got, tt.wantCreate)
			}
			if _, exists := mgr.executionStore.GetBySessionID("session-workspace-restore"); exists {
				t.Fatal("rejected workspace restore left an execution registered")
			}
		})
	}
}

func TestWorkspaceRestoreReusesRetainedTerminalRuntime(t *testing.T) {
	mgr := newTestManager(t)
	mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
		"session-workspace-restore": {
			TaskID: "task-workspace-restore", SessionID: "session-workspace-restore",
		},
	}}
	mgr.SetExecutorProfileReader(&fakeExecutorProfileReader{session: &models.TaskSession{
		ID: "session-workspace-restore", TaskID: "task-workspace-restore", State: models.TaskSessionStateFailed,
	}})
	retained := &AgentExecution{
		ID: "retained-workspace", TaskID: "task-workspace-restore", SessionID: "session-workspace-restore",
		Status: v1.AgentStatusStopped,
	}
	if err := mgr.executionStore.Add(retained); err != nil {
		t.Fatalf("add retained execution: %v", err)
	}

	got, err := mgr.GetOrEnsureExecution(context.Background(), "session-workspace-restore")
	if err != nil {
		t.Fatalf("GetOrEnsureExecution returned error: %v", err)
	}
	if got != retained {
		t.Fatalf("execution = %p, want retained %p", got, retained)
	}
}

func TestManualRecoveryPreflightDoesNotDeadlockWorkspaceSingleflight(t *testing.T) {
	const (
		taskID        = "task-recovery-singleflight"
		sessionID     = "session-recovery-singleflight"
		environmentID = "environment-recovery-singleflight"
		repositoryID  = "repository-recovery-singleflight"
		worktreeID    = "worktree-recovery-singleflight"
		taskDirName   = "recovery-singleflight"
	)
	root := t.TempDir()
	repositoryPath := filepath.Join(root, "repository")
	recoveryTestGit(t, repositoryPath, "init", "-b", "main")
	recoveryTestGit(t, repositoryPath, "config", "user.email", "test@example.com")
	recoveryTestGit(t, repositoryPath, "config", "user.name", "Test User")
	recoveryTestGit(t, repositoryPath, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("initial\n"), 0o644); err != nil {
		t.Fatalf("write repository fixture: %v", err)
	}
	recoveryTestGit(t, repositoryPath, "add", "README.md")
	recoveryTestGit(t, repositoryPath, "commit", "-m", "initial")
	recoveryTestGit(t, repositoryPath, "branch", "feature/recovery")

	tasksBase := filepath.Join(root, "tasks")
	taskRoot := filepath.Join(tasksBase, taskDirName)
	worktreePath := filepath.Join(taskRoot, "repository")
	if err := os.MkdirAll(taskRoot, 0o755); err != nil {
		t.Fatalf("create task root: %v", err)
	}
	if err := storageworkspaces.WriteOwnershipMarker(taskRoot, storageworkspaces.OwnershipMarker{
		TaskID: taskID, TaskDirName: taskDirName, LayoutVersion: storageworkspaces.LayoutVersionSemantic,
	}); err != nil {
		t.Fatalf("write task root ownership marker: %v", err)
	}
	recoveryTestGit(t, repositoryPath, "worktree", "add", worktreePath, "feature/recovery")
	recoveryTestGit(t, repositoryPath, "worktree", "remove", "--force", worktreePath)
	recoveryTestGit(t, repositoryPath, "worktree", "prune")

	selected := &worktree.Worktree{
		ID: worktreeID, SessionID: sessionID, TaskID: taskID, TaskDirName: taskDirName,
		TaskEnvironmentID: environmentID, RepositoryID: repositoryID, BranchSlug: "main",
		RepositoryPath: repositoryPath, Path: worktreePath, Branch: "feature/recovery", Status: worktree.StatusActive,
	}
	claimEntered := make(chan struct{})
	releaseClaim := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseClaim) }) }
	defer release()
	store := &recoveryAdmissionBarrierStore{
		worktree: selected, claimEntered: claimEntered, releaseClaim: releaseClaim,
	}
	worktreeManager, err := worktree.NewManager(worktree.Config{
		Enabled: true, TasksBasePath: tasksBase, BranchPrefix: "feature/",
	}, store, newTestLogger())
	if err != nil {
		t.Fatalf("create worktree manager: %v", err)
	}
	info := &WorkspaceInfo{
		TaskID: taskID, SessionID: sessionID, TaskEnvironmentID: environmentID,
		EnvironmentOwnerTaskID: taskID, OwnershipGeneration: 1,
		ExecutorType: string(models.ExecutorTypeWorktree),
		WorkspaceRepositories: []WorkspaceRepositorySpec{{
			RepositoryID: repositoryID, RepositoryPath: repositoryPath,
			WorktreeID: worktreeID, WorktreePath: worktreePath, WorktreeBranch: "feature/recovery", BranchSlug: "main",
		}},
	}
	manager, backend := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{})
	manager.SetWorktreeManager(worktreeManager)
	var singleflightBodyCalls atomic.Int32
	runLifecycleAdmission := func(ctx context.Context) error {
		_, err := manager.doCoalescedExecution(ctx, sessionID, func(sharedCtx context.Context) (interface{}, error) {
			singleflightBodyCalls.Add(1)
			admission, admissionErr := manager.admitWorkspaceRecovery(sharedCtx, info)
			if admission != nil {
				_ = admission.Release(context.Background())
			}
			return nil, admissionErr
		})
		return err
	}
	firstResult := make(chan error, 1)
	go func() { firstResult <- runLifecycleAdmission(context.Background()) }()
	select {
	case <-claimEntered:
	case err := <-firstResult:
		t.Fatalf("workspace reconstruction ended before the recovery claim: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("workspace reconstruction did not reach the held recovery claim")
	}

	secondCtx := &lifecycleObservedDoneContext{Context: context.Background(), observed: make(chan struct{})}
	secondResult := make(chan error, 1)
	go func() { secondResult <- runLifecycleAdmission(secondCtx) }()
	select {
	case <-secondCtx.observed:
	case <-time.After(2 * time.Second):
		release()
		t.Fatal("second lifecycle caller did not join the session singleflight")
	}

	manualRequest := worktree.RecoveryAdmissionRequest{
		TaskID: taskID, SessionID: sessionID, TaskEnvironmentID: environmentID,
		OwnerTaskID: taskID, OwnershipGeneration: 1, ExecutorType: string(models.ExecutorTypeWorktree),
		InspectionWait: worktree.ManualRecoveryInspectionWait,
		Slots: []worktree.RecoverySlot{{
			WorktreeID: worktreeID, RepositoryID: repositoryID, BranchSlug: "main", RepositoryPath: repositoryPath,
		}},
	}
	manualCtx := &lifecycleObservedDoneContext{Context: context.Background(), observed: make(chan struct{})}
	manualResult := make(chan error, 1)
	go func() {
		admission, admissionErr := worktreeManager.AdmitRecovery(manualCtx, manualRequest)
		if admission != nil {
			_ = admission.Release(context.Background())
		}
		manualResult <- admissionErr
	}()
	select {
	case <-manualCtx.observed:
	case <-time.After(2 * time.Second):
		release()
		t.Fatal("manual recovery did not wait outside the lifecycle singleflight")
	}

	release()
	for name, result := range map[string]<-chan error{
		"lifecycle owner":  firstResult,
		"lifecycle waiter": secondResult,
		"manual preflight": manualResult,
	} {
		select {
		case err := <-result:
			if err == nil {
				t.Errorf("%s unexpectedly acquired recovery authority", name)
			}
		case <-time.After(5 * time.Second):
			t.Errorf("%s remained blocked after inspection release", name)
		}
	}
	if got := singleflightBodyCalls.Load(); got != 1 {
		t.Fatalf("lifecycle singleflight body calls = %d, want 1", got)
	}
	if got := store.claimCalls.Load(); got != 2 {
		t.Fatalf("recovery claim attempts = %d, want lifecycle admission and manual retry", got)
	}
	if got := backend.createCount.Load(); got != 0 {
		t.Fatalf("runtime instances created after refused recovery = %d, want 0", got)
	}
}

type recoveryAdmissionBarrierStore struct {
	worktree.Store
	worktree     *worktree.Worktree
	claimEntered chan struct{}
	releaseClaim <-chan struct{}
	claimCalls   atomic.Int32
}

func (s *recoveryAdmissionBarrierStore) GetWorktreeByID(_ context.Context, id string) (*worktree.Worktree, error) {
	if s.worktree == nil || s.worktree.ID != id {
		return nil, nil
	}
	copy := *s.worktree
	return &copy, nil
}

func (s *recoveryAdmissionBarrierStore) AcquireTaskEnvironmentRecoveryClaim(
	ctx context.Context,
	_ models.TaskEnvironmentRecoveryClaimRequest,
) (*models.TaskEnvironmentRecoveryClaim, error) {
	if s.claimCalls.Add(1) == 1 {
		close(s.claimEntered)
		select {
		case <-s.releaseClaim:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, errors.New("injected recovery claim refusal")
}

func (*recoveryAdmissionBarrierStore) ReleaseTaskEnvironmentRecoveryClaim(
	context.Context,
	*models.TaskEnvironmentRecoveryClaim,
) error {
	return nil
}

type lifecycleObservedDoneContext struct {
	context.Context
	observed chan struct{}
	once     sync.Once
}

func (c *lifecycleObservedDoneContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.observed) })
	return c.Context.Done()
}

func recoveryTestGit(t *testing.T, directory string, args ...string) {
	t.Helper()
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatalf("create git fixture directory: %v", err)
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = directory
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}
