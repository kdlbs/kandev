package lifecycle

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/task/models"
)

func TestPrepareExecutionGitMetadataAddsLazyWorktreeProjection(t *testing.T) {
	repo := t.TempDir()
	runLazyGit(t, "", "init", "-b", "main", repo)
	runLazyGit(t, repo, "config", "user.email", "test@example.com")
	runLazyGit(t, repo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "file"), []byte("initial"), 0o600); err != nil {
		t.Fatal(err)
	}
	runLazyGit(t, repo, "add", "file")
	runLazyGit(t, repo, "commit", "-m", "initial")
	workspaceRoot := filepath.Join(t.TempDir(), "task")
	workspace := filepath.Join(workspaceRoot, "repo")
	runLazyGit(t, repo, "worktree", "add", "-b", "task", workspace)

	info := &WorkspaceInfo{
		// Lazy recovery stores the sole checkout itself as WorkspacePath. Only
		// multi-repository recovery uses the parent task root.
		WorkspacePath: workspace,
		ExecutorType:  string(models.ExecutorTypeWorktree),
		WorkspaceRepositories: []WorkspaceRepositorySpec{{
			RepositoryPath: repo,
			RepoName:       "repo",
			BranchSlug:     "",
		}},
	}
	req := &ExecutorCreateRequest{}
	if err := (&Manager{}).prepareExecutionGitMetadata(info, &mockStopTracker{}, req); err != nil {
		t.Fatalf("prepareExecutionGitMetadata() error = %v", err)
	}
	if len(req.GitMetadataProjections) != 1 {
		t.Fatalf("projections = %d, want 1", len(req.GitMetadataProjections))
	}
	if req.GitMetadataProjections[0].CheckoutPath != workspace {
		t.Fatalf("checkout path = %q, want %q", req.GitMetadataProjections[0].CheckoutPath, workspace)
	}
}

func TestGetOrEnsureExecutionRecreatesLocalRepositoryInPlace(t *testing.T) {
	repository := initGitRepo(t)
	provider := &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
		"session-local": {
			TaskID:                     "task-1",
			SessionID:                  "session-local",
			TaskEnvironmentID:          "environment-local",
			ValidatedTaskEnvironmentID: "environment-local",
			ValidatedExecutorType:      string(models.ExecutorTypeLocal),
			ExecutorType:               string(models.ExecutorTypeLocal),
			WorkspacePath:              repository,
			AgentID:                    "auggie",
			WorkspaceRepositories: []WorkspaceRepositorySpec{{
				RepositoryID:   "repository-1",
				RepositoryPath: repository,
				RepoName:       "repository",
			}},
		},
	}}
	mgr, backend := newEnvironmentExecutionTestManager(t, provider)

	execution, err := mgr.GetOrEnsureExecution(context.Background(), "session-local")
	if err != nil {
		t.Fatalf("GetOrEnsureExecution() error = %v", err)
	}
	if execution.WorkspacePath != repository {
		t.Fatalf("execution workspace = %q, want in-place repository %q", execution.WorkspacePath, repository)
	}
	if len(backend.lastRequest.GitMetadataProjections) != 0 {
		t.Fatalf("local execution Git metadata projections = %#v, want none", backend.lastRequest.GitMetadataProjections)
	}
}

func TestGetOrEnsureExecutionReconstructsRemoteMultiRepositoryWorkspace(t *testing.T) {
	for _, testCase := range []struct {
		name                string
		previousExecutionID string
	}{
		{name: "fresh"},
		{name: "reconnect", previousExecutionID: "previous-execution"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			server := newWorkspaceRebindAgentctlServer(t, false)
			defer server.Close()
			client := workspaceMaterializationAgentctlClient(t, server.URL)
			provider := &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
				"session-remote": {
					TaskID:            "task-1",
					SessionID:         "session-remote",
					TaskEnvironmentID: "environment-remote",
					ExecutorType:      string(models.ExecutorTypeLocalDocker),
					WorkspacePath:     "/executor/workspace",
					AgentID:           "codex-acp",
					AgentExecutionID:  testCase.previousExecutionID,
					WorkspaceRepositories: []WorkspaceRepositorySpec{
						{RepositoryID: "repository-main", RepositoryURL: "https://github.com/acme/main.git", RepoName: "main", BaseBranch: "main"},
						{RepositoryID: "repository-added", RepositoryURL: "https://github.com/acme/added.git", RepoName: "added", BaseBranch: "main"},
					},
				},
			}}
			log := newTestLogger()
			registry := NewExecutorRegistry(log)
			backend := &lazyMultiRepositoryExecutor{
				createInstanceExecutor: createInstanceExecutor{
					MockExecutor: MockExecutor{name: executor.NameDocker},
					client:       client,
				},
			}
			registry.Register(backend)
			mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, &MockCredentialsManager{}, &MockProfileResolver{}, nil, ExecutorFallbackWarn, "", log)
			mgr.workspaceInfoProvider = provider
			cleanupManagerStopCh(t, mgr)

			execution, err := mgr.GetOrEnsureExecution(context.Background(), "session-remote")
			if err != nil {
				t.Fatalf("GetOrEnsureExecution() error = %v", err)
			}
			if !server.hasMaterialized("added-main") {
				t.Fatal("lazy remote recovery did not materialize the durable sibling repository")
			}
			wantRoots := []string{"/executor/workspace", "/executor/workspace/added-main"}
			if !reflect.DeepEqual(execution.WorkspaceSourceRoots, wantRoots) {
				t.Fatalf("workspace source roots = %v, want %v", execution.WorkspaceSourceRoots, wantRoots)
			}
			if got, want := server.operationLog(), []string{"materialize", "rescan", "attest"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("remote reconstruction operations = %v, want %v", got, want)
			}
		})
	}
}

func TestGetOrEnsureExecutionForceCleansRemoteRuntimeWhenReconstructionFails(t *testing.T) {
	server := newWorkspaceRebindAgentctlServer(t, false)
	defer server.Close()
	server.failMaterialize = true
	provider := &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
		"session-remote": {
			TaskID: "task-1", SessionID: "session-remote", TaskEnvironmentID: "environment-remote",
			ExecutorType: string(models.ExecutorTypeLocalDocker), WorkspacePath: "/executor/workspace", AgentID: "codex-acp",
			WorkspaceRepositories: []WorkspaceRepositorySpec{
				{RepositoryID: "repository-main", RepositoryURL: "https://github.com/acme/main.git", RepoName: "main", BaseBranch: "main"},
				{RepositoryID: "repository-added", RepositoryURL: "https://github.com/acme/added.git", RepoName: "added", BaseBranch: "main"},
			},
		},
	}}
	log := newTestLogger()
	registry := NewExecutorRegistry(log)
	backend := &lazyMultiRepositoryExecutor{createInstanceExecutor: createInstanceExecutor{
		MockExecutor: MockExecutor{name: executor.NameDocker}, client: workspaceMaterializationAgentctlClient(t, server.URL),
	}}
	registry.Register(backend)
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, &MockCredentialsManager{}, &MockProfileResolver{}, nil, ExecutorFallbackWarn, "", log)
	mgr.workspaceInfoProvider = provider
	cleanupManagerStopCh(t, mgr)

	_, err := mgr.GetOrEnsureExecution(context.Background(), "session-remote")
	if err == nil || !strings.Contains(err.Error(), "reconstruct remote workspace repositories") {
		t.Fatalf("GetOrEnsureExecution() error = %v, want reconstruction failure", err)
	}
	if got := backend.stopCount.Load(); got != 1 || !backend.forceStopped.Load() {
		t.Fatalf("remote reconstruction cleanup = (count=%d, force=%v), want one forced teardown", got, backend.forceStopped.Load())
	}
}

func TestGetOrEnsureExecutionReleasesRuntimeInventoryWhenGitMetadataPolicyFails(t *testing.T) {
	server := newWorkspaceRebindAgentctlServer(t, false)
	server.attestationErr = true
	defer server.Close()

	provider := &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
		"session-kubernetes": {
			TaskID: "task-1", SessionID: "session-kubernetes", TaskEnvironmentID: "environment-kubernetes",
			ExecutorType: string(models.ExecutorTypeKubernetes), WorkspacePath: "/executor/workspace", AgentID: "codex-acp",
			WorkspaceRepositories: []WorkspaceRepositorySpec{
				{RepositoryID: "repository-main", RepositoryURL: "https://github.com/acme/main.git", RepoName: "main", BaseBranch: "main"},
				{RepositoryID: "repository-added", RepositoryURL: "https://github.com/acme/added.git", RepoName: "added", BaseBranch: "main"},
			},
		},
	}}
	backend := &releaseTrackingLazyMultiRepositoryExecutor{lazyMultiRepositoryExecutor: lazyMultiRepositoryExecutor{
		createInstanceExecutor: createInstanceExecutor{
			MockExecutor: MockExecutor{name: executor.NameKubernetes},
			client:       workspaceMaterializationAgentctlClient(t, server.URL),
		},
	}}
	registry := NewExecutorRegistry(newTestLogger())
	registry.Register(backend)
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, &MockCredentialsManager{}, &MockProfileResolver{}, nil, ExecutorFallbackWarn, "", newTestLogger())
	mgr.workspaceInfoProvider = provider
	cleanupManagerStopCh(t, mgr)

	_, err := mgr.GetOrEnsureExecution(context.Background(), "session-kubernetes")
	if err == nil || !strings.Contains(err.Error(), "install clone Git metadata policy") {
		t.Fatalf("GetOrEnsureExecution() error = %v, want policy installation failure", err)
	}
	if got := backend.stopCount.Load(); got != 1 || !backend.forceStopped.Load() {
		t.Fatalf("policy failure cleanup = (count=%d, force=%v), want one forced teardown", got, backend.forceStopped.Load())
	}
	if backend.releaseCount != 1 {
		t.Fatalf("released runtime inventory = %d, want one release", backend.releaseCount)
	}
}

type lazyMultiRepositoryExecutor struct{ createInstanceExecutor }

func (*lazyMultiRepositoryExecutor) RequiresCloneURL() bool { return true }

func (*lazyMultiRepositoryExecutor) PrepareGitMetadataProjection(context.Context, *ExecutorCreateRequest) error {
	return nil
}

var _ ExecutorBackend = (*lazyMultiRepositoryExecutor)(nil)

type releaseTrackingLazyMultiRepositoryExecutor struct {
	lazyMultiRepositoryExecutor
	releaseCount int
}

func (e *releaseTrackingLazyMultiRepositoryExecutor) CreateInstance(ctx context.Context, req *ExecutorCreateRequest) (*ExecutorInstance, error) {
	instance, err := e.lazyMultiRepositoryExecutor.CreateInstance(ctx, req)
	if err != nil {
		return nil, err
	}
	instance.ReleaseRuntimeInventory = func(context.Context) error {
		e.releaseCount++
		return nil
	}
	return instance, nil
}

func TestPrepareExecutionGitMetadataMarksLazyCloneRequirement(t *testing.T) {
	info := &WorkspaceInfo{WorkspaceRepositories: []WorkspaceRepositorySpec{{RepositoryPath: "/source", RepoName: "repo"}}}
	req := &ExecutorCreateRequest{}
	runtime := &lazyCloneExecutor{}
	if err := (&Manager{}).prepareExecutionGitMetadata(info, runtime, req); err != nil {
		t.Fatalf("prepareExecutionGitMetadata() error = %v", err)
	}
	if !req.GitMetadataRequirement.RequiresMutableCloneCheckout() {
		t.Fatal("lazy clone request did not require Git metadata attestation")
	}
}

type lazyCloneExecutor struct{ mockStopTracker }

func (*lazyCloneExecutor) RequiresCloneURL() bool { return true }

func runLazyGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	if output, err := exec.CommandContext(context.Background(), "git", args...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}
