package backendapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type remoteWorkspaceMaterializerStub struct {
	calls [][]lifecycle.WorkspaceRepositoryMaterialization
	err   error
	ids   []string
}

type hostRepositoryClonerStub struct {
	path  string
	calls []*models.Repository
	err   error
}

func (s *hostRepositoryClonerStub) EnsureRepositoryCloned(_ context.Context, repository *models.Repository) (string, error) {
	s.calls = append(s.calls, repository)
	return s.path, s.err
}

func (s *remoteWorkspaceMaterializerStub) MaterializeRepositoriesForEnvironment(_ context.Context, _ string, repositories []lifecycle.WorkspaceRepositoryMaterialization) ([]string, error) {
	s.calls = append(s.calls, append([]lifecycle.WorkspaceRepositoryMaterialization(nil), repositories...))
	return append([]string(nil), s.ids...), s.err
}

type failingSessionMaterializerRepo struct {
	workspaceSourceMaterializerRepo
	err error
}

func (r failingSessionMaterializerRepo) ListTaskSessions(context.Context, string) ([]*models.TaskSession, error) {
	return nil, r.err
}

func TestBuildRemoteWorkspaceRepositories_SkipsPrimaryAndUsesDurableRuntimeNames(t *testing.T) {
	repositories, err := buildRemoteWorkspaceRepositories([]*models.TaskRepository{
		{RepositoryID: "primary", BaseBranch: "main"},
		{RepositoryID: "repo-1", BaseBranch: "main", CheckoutBranch: "feature/add-source"},
	}, map[string]*models.Repository{
		"primary": {ID: "primary", Name: "primary", RemoteURL: "https://github.com/acme/primary.git"},
		"repo-1":  {ID: "repo-1", Name: "API", RemoteURL: "https://github.com/acme/api.git"},
	})
	if err != nil {
		t.Fatalf("buildRemoteWorkspaceRepositories: %v", err)
	}
	want := lifecycle.WorkspaceRepositoryMaterialization{RepositoryURL: "https://github.com/acme/api.git", Destination: "API-feature-add-source", BaseBranch: "main", CheckoutBranch: "feature/add-source"}
	if len(repositories) != 1 || repositories[0] != want {
		t.Fatalf("repositories=%+v, want [%+v]", repositories, want)
	}
	_, err = buildRemoteWorkspaceRepositories([]*models.TaskRepository{{RepositoryID: "primary", BaseBranch: "main"}, {RepositoryID: "local", BaseBranch: "main"}}, map[string]*models.Repository{
		"primary": {ID: "primary", Name: "primary", RemoteURL: "https://github.com/acme/primary.git"},
		"local":   {ID: "local", Name: "local", LocalPath: "/host/only/repository"},
	})
	if err == nil {
		t.Fatal("local-only repository was accepted for remote materialization")
	}
}

func TestLocalWorkspaceEntries_RejectsFolderRepositoryNameCollision(t *testing.T) {
	_, err := localWorkspaceEntries(
		[]*models.TaskRepository{{RepositoryID: "repo-1", BaseBranch: "main"}},
		[]*models.TaskWorkspaceFolder{{DisplayName: "docs", LocalPath: "/folders/docs"}},
		map[string]*models.Repository{"repo-1": {ID: "repo-1", Name: "docs", LocalPath: "/repositories/docs"}},
		nil,
	)
	if err == nil {
		t.Fatal("local workspace entries accepted colliding folder and repository names")
	}
}

func TestWorkspaceSourceMaterializer_RemoteLoadsSessionsBeforeFilesystemMutation(t *testing.T) {
	ctx := context.Background()
	repo := newMaterializerRepo(t)
	seedWorkspaceSourceTask(t, repo, t.TempDir())
	seedRemoteMaterializerRepositories(t, repo)
	env, err := repo.GetTaskEnvironmentByTaskID(ctx, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	env.ExecutorType = string(models.ExecutorTypeLocalDocker)
	if err := repo.UpdateTaskEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}
	remote := &remoteWorkspaceMaterializerStub{}
	materializer := &workspaceSourceMaterializer{
		repo:               failingSessionMaterializerRepo{workspaceSourceMaterializerRepo: repo, err: errors.New("sessions unavailable")},
		worktreeMgr:        newMaterializerWorktreeMgr(t, filepath.Join(t.TempDir(), "task-1")),
		remoteMaterializer: remote,
		logger:             newTestLogger(),
	}
	_, err = materializer.MaterializeWorkspaceSources(ctx, "task-1", &models.WorkspaceSourceBatch{TaskID: "task-1"})
	if err == nil {
		t.Fatal("MaterializeWorkspaceSources succeeded despite session lookup failure")
	}
	if len(remote.calls) != 0 {
		t.Fatalf("remote materialization ran before session lookup: %+v", remote.calls)
	}
}

func TestWorkspaceSourceMaterializer_PrelaunchReturnsExplicitDeferredResult(t *testing.T) {
	ctx := context.Background()
	repo := newMaterializerRepo(t)
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-prelaunch", Name: "Prelaunch"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-prelaunch", WorkspaceID: "ws-prelaunch", WorkflowID: "wf", WorkflowStepID: "step", Title: "Task"}); err != nil {
		t.Fatal(err)
	}
	materializer := &workspaceSourceMaterializer{repo: repo, worktreeMgr: newMaterializerWorktreeMgr(t, filepath.Join(t.TempDir(), "task-prelaunch")), logger: newTestLogger()}

	result, err := materializer.MaterializeWorkspaceSources(ctx, "task-prelaunch", &models.WorkspaceSourceBatch{TaskID: "task-prelaunch"})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.WorkspacePath != "" || len(result.SessionIDs) != 0 {
		t.Fatalf("prelaunch materialization result = %#v, want explicit empty deferred result", result)
	}
}

func TestWorkspaceSourceMaterializer_RemoteMaterializesAdditionalRepositories(t *testing.T) {
	for _, executorType := range []models.ExecutorType{models.ExecutorTypeLocalDocker, models.ExecutorTypeSSH, models.ExecutorTypeSprites} {
		t.Run(string(executorType), func(t *testing.T) {
			ctx := context.Background()
			repo := newMaterializerRepo(t)
			seedWorkspaceSourceTask(t, repo, t.TempDir())
			seedRemoteMaterializerRepositories(t, repo)
			env, err := repo.GetTaskEnvironmentByTaskID(ctx, "task-1")
			if err != nil {
				t.Fatal(err)
			}
			env.ExecutorType = string(executorType)
			if err := repo.UpdateTaskEnvironment(ctx, env); err != nil {
				t.Fatal(err)
			}
			remote := &remoteWorkspaceMaterializerStub{ids: []string{"session-1"}}
			materializer := &workspaceSourceMaterializer{repo: repo, worktreeMgr: newMaterializerWorktreeMgr(t, filepath.Join(t.TempDir(), "task-1")), remoteMaterializer: remote, logger: newTestLogger()}
			result, err := materializer.MaterializeWorkspaceSources(ctx, "task-1", &models.WorkspaceSourceBatch{TaskID: "task-1", Sources: []models.WorkspaceSource{{Repository: &models.TaskRepository{RepositoryID: "repo-added", BaseBranch: "main"}}}})
			if err != nil {
				t.Fatal(err)
			}
			if result.WorkspacePath != env.WorkspacePath || len(result.SessionIDs) != 1 || result.SessionIDs[0] != "session-1" {
				t.Fatalf("materialization result = %#v", result)
			}
			if len(remote.calls) != 1 || len(remote.calls[0]) != 1 || remote.calls[0][0].Destination != "added-main" {
				t.Fatalf("remote projection=%+v; want only additional repository", remote.calls)
			}
		})
	}
}

func TestWorkspaceSourceMaterializer_RemoteReturnsOnlyLifecycleAdoptedSessions(t *testing.T) {
	ctx := context.Background()
	repo := newMaterializerRepo(t)
	seedWorkspaceSourceTask(t, repo, t.TempDir())
	seedRemoteMaterializerRepositories(t, repo)
	env, err := repo.GetTaskEnvironmentByTaskID(ctx, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	env.ExecutorType = string(models.ExecutorTypeSSH)
	if err := repo.UpdateTaskEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}
	remote := &remoteWorkspaceMaterializerStub{ids: []string{"session-live"}}
	materializer := &workspaceSourceMaterializer{repo: repo, worktreeMgr: newMaterializerWorktreeMgr(t, filepath.Join(t.TempDir(), "task-1")), remoteMaterializer: remote, logger: newTestLogger()}

	result, err := materializer.MaterializeWorkspaceSources(ctx, "task-1", &models.WorkspaceSourceBatch{TaskID: "task-1", Sources: []models.WorkspaceSource{{Repository: &models.TaskRepository{RepositoryID: "repo-added", BaseBranch: "main"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.SessionIDs, []string{"session-live"}) {
		t.Fatalf("session ids = %v, want only lifecycle-adopted session", result.SessionIDs)
	}
}

// A live attachment batch must not revalidate durable siblings. An agent may
// have committed in the first sibling between attachments, making its HEAD
// intentionally differ from origin; only the newly submitted destination is
// eligible for materialization on this live execution.
func TestWorkspaceSourceMaterializer_RemoteMaterializesOnlySubmittedRepositoryBatch(t *testing.T) {
	ctx := context.Background()
	repo := newMaterializerRepo(t)
	seedWorkspaceSourceTask(t, repo, t.TempDir())
	seedRemoteMaterializerRepositories(t, repo)
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-later", WorkspaceID: "ws-1", Name: "later", RemoteURL: "https://github.com/acme/later.git"}); err != nil {
		t.Fatal(err)
	}
	later := &models.TaskRepository{ID: "task-repo-later", TaskID: "task-1", RepositoryID: "repo-later", BaseBranch: "main", Position: 2, Metadata: map[string]interface{}{}}
	if err := repo.CreateTaskRepository(ctx, later); err != nil {
		t.Fatal(err)
	}
	env, err := repo.GetTaskEnvironmentByTaskID(ctx, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	env.ExecutorType = string(models.ExecutorTypeSSH)
	if err := repo.UpdateTaskEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}
	remote := &remoteWorkspaceMaterializerStub{}
	materializer := &workspaceSourceMaterializer{repo: repo, worktreeMgr: newMaterializerWorktreeMgr(t, filepath.Join(t.TempDir(), "task-1")), remoteMaterializer: remote, logger: newTestLogger()}
	_, err = materializer.MaterializeWorkspaceSources(ctx, "task-1", &models.WorkspaceSourceBatch{TaskID: "task-1", Sources: []models.WorkspaceSource{{Repository: later}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(remote.calls) != 1 || len(remote.calls[0]) != 1 || remote.calls[0][0].Destination != "later-main" {
		t.Fatalf("live remote projection=%+v; want only later-main", remote.calls)
	}
}

func TestWorkspaceSourceMaterializer_RemoteFailure(t *testing.T) {
	ctx := context.Background()
	repo := newMaterializerRepo(t)
	seedWorkspaceSourceTask(t, repo, t.TempDir())
	seedRemoteMaterializerRepositories(t, repo)
	env, err := repo.GetTaskEnvironmentByTaskID(ctx, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	env.ExecutorType = string(models.ExecutorTypeSprites)
	if err := repo.UpdateTaskEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}
	materializer := &workspaceSourceMaterializer{repo: repo, worktreeMgr: newMaterializerWorktreeMgr(t, filepath.Join(t.TempDir(), "task-1")), remoteMaterializer: &remoteWorkspaceMaterializerStub{err: errors.New("agentctl unavailable")}, logger: newTestLogger()}
	if _, err := materializer.MaterializeWorkspaceSources(ctx, "task-1", &models.WorkspaceSourceBatch{TaskID: "task-1"}); err == nil {
		t.Fatal("MaterializeWorkspaceSources succeeded despite remote materialization failure")
	}
}

func TestWorkspaceSourceMaterializer_RemoteRejectsLocalOnlyAdditionalRepositoryBeforeCall(t *testing.T) {
	ctx := context.Background()
	repo := newMaterializerRepo(t)
	seedWorkspaceSourceTask(t, repo, t.TempDir())
	seedRemoteMaterializerRepositories(t, repo)
	if err := repo.UpdateRepository(ctx, &models.Repository{ID: "repo-added", WorkspaceID: "ws-1", Name: "added", LocalPath: "/host/only/repository"}); err != nil {
		t.Fatal(err)
	}
	env, err := repo.GetTaskEnvironmentByTaskID(ctx, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	env.ExecutorType = string(models.ExecutorTypeSSH)
	if err := repo.UpdateTaskEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}
	remote := &remoteWorkspaceMaterializerStub{}
	materializer := &workspaceSourceMaterializer{repo: repo, worktreeMgr: newMaterializerWorktreeMgr(t, filepath.Join(t.TempDir(), "task-1")), remoteMaterializer: remote, logger: newTestLogger()}
	if _, err := materializer.MaterializeWorkspaceSources(ctx, "task-1", &models.WorkspaceSourceBatch{TaskID: "task-1", Sources: []models.WorkspaceSource{{Repository: &models.TaskRepository{RepositoryID: "repo-added", BaseBranch: "main"}}}}); !errors.Is(err, ErrRemoteRepositoryLocatorUnavailable) {
		t.Fatalf("error=%v; want local-only remote locator error", err)
	}
	if len(remote.calls) != 0 {
		t.Fatalf("remote materialization called for local-only repository: %+v", remote.calls)
	}
}

func TestWorkspaceSourceMaterializer_LocalFolderCreatesLiveTaskEntryAndRebindsSessions(t *testing.T) {
	ctx := context.Background()
	repo := newMaterializerRepo(t)
	tasksBase := filepath.Join(t.TempDir(), "tasks")
	mgr := newMaterializerWorktreeMgr(t, filepath.Join(tasksBase, "task-1"))
	source := filepath.Join(t.TempDir(), "notes")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "note.txt"), []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	seedWorkspaceSourceTask(t, repo, source)
	env, err := repo.GetTaskEnvironmentByTaskID(ctx, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	env.ExecutorType = "local_pc"
	if err := repo.UpdateTaskEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}
	rescan := &workspaceSourceRescanStub{}
	materializer := &workspaceSourceMaterializer{repo: repo, worktreeMgr: mgr, rescanner: rescan, logger: newTestLogger()}

	result, err := materializer.MaterializeWorkspaceSources(ctx, "task-1", &models.WorkspaceSourceBatch{TaskID: "task-1", Sources: []models.WorkspaceSource{{Folder: &models.TaskWorkspaceFolder{DisplayName: "notes", LocalPath: source}}}})
	if err != nil {
		t.Fatalf("MaterializeWorkspaceSources: %v", err)
	}
	root := filepath.Join(tasksBase, "task-1")
	if got, err := os.ReadFile(filepath.Join(root, "notes", "note.txt")); err != nil || string(got) != "before" {
		t.Fatalf("live entry = %q, %v", got, err)
	}
	if err := os.WriteFile(filepath.Join(source, "note.txt"), []byte("after"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "notes", "note.txt")); string(got) != "after" {
		t.Fatalf("entry is not live: %q", got)
	}
	env, err = repo.GetTaskEnvironmentByTaskID(ctx, "task-1")
	if err != nil || env.WorkspacePath != root {
		t.Fatalf("workspace path = %q, %v; want %q", env.WorkspacePath, err, root)
	}
	if len(rescan.calls) != 1 || rescan.calls[0].workDir != root {
		t.Fatalf("rescan calls = %+v", rescan.calls)
	}
	if result.WorkspacePath != root || len(result.SessionIDs) != 1 || result.SessionIDs[0] != "session-1" {
		t.Fatalf("materialization result = %#v", result)
	}
}

func TestWorkspaceSourceMaterializer_LocalFolderUsesPersistedBatchSourceOnlyOnce(t *testing.T) {
	ctx := context.Background()
	repo := newMaterializerRepo(t)
	tasksBase := filepath.Join(t.TempDir(), "tasks")
	mgr := newMaterializerWorktreeMgr(t, filepath.Join(tasksBase, "task-1"))
	source := filepath.Join(t.TempDir(), "notes")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "note.txt"), []byte("persisted"), 0o644); err != nil {
		t.Fatal(err)
	}
	seedWorkspaceSourceTask(t, repo, source)
	env, err := repo.GetTaskEnvironmentByTaskID(ctx, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	env.ExecutorType = "local_pc"
	if err := repo.UpdateTaskEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}
	batch := &models.WorkspaceSourceBatch{TaskID: "task-1", Sources: []models.WorkspaceSource{{
		Folder: &models.TaskWorkspaceFolder{DisplayName: "notes", LocalPath: source},
	}}}
	if err := repo.CreateWorkspaceSourceBatch(ctx, batch); err != nil {
		t.Fatal(err)
	}

	materializer := &workspaceSourceMaterializer{repo: repo, worktreeMgr: mgr, logger: newTestLogger()}
	if _, err := materializer.MaterializeWorkspaceSources(ctx, "task-1", batch); err != nil {
		t.Fatalf("MaterializeWorkspaceSources after persistence: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(tasksBase, "task-1", "notes", "note.txt")); err != nil || string(got) != "persisted" {
		t.Fatalf("persisted folder entry = %q, %v", got, err)
	}
}

func TestWorkspaceSourceMaterializer_LocalPromotionPreservesPrimaryRepository(t *testing.T) {
	ctx := context.Background()
	repo := newMaterializerRepo(t)
	tasksBase := filepath.Join(t.TempDir(), "tasks")
	mgr := newMaterializerWorktreeMgr(t, filepath.Join(tasksBase, "task-1"))
	primary := filepath.Join(t.TempDir(), "primary")
	folder := filepath.Join(t.TempDir(), "notes")
	for _, path := range []string{primary, folder} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(primary, "repo.txt"), []byte("repo"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "note.txt"), []byte("note"), 0o644); err != nil {
		t.Fatal(err)
	}
	seedWorkspaceSourceTask(t, repo, primary)
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "primary", LocalPath: primary, DefaultBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{ID: "tr-primary", TaskID: "task-1", RepositoryID: "repo-1", BaseBranch: "main", Metadata: map[string]interface{}{}}); err != nil {
		t.Fatal(err)
	}
	rescan := &workspaceSourceRescanStub{}
	m := &workspaceSourceMaterializer{repo: repo, worktreeMgr: mgr, rescanner: rescan, logger: newTestLogger()}
	batch := &models.WorkspaceSourceBatch{TaskID: "task-1", Sources: []models.WorkspaceSource{{Folder: &models.TaskWorkspaceFolder{DisplayName: "notes", LocalPath: folder}}}}
	if _, err := m.MaterializeWorkspaceSources(ctx, "task-1", batch); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(tasksBase, "task-1")
	for _, file := range []string{"primary/repo.txt", "notes/note.txt"} {
		if _, err := os.ReadFile(filepath.Join(root, file)); err != nil {
			t.Fatalf("missing promoted source %s: %v", file, err)
		}
	}
	env, err := repo.GetTaskEnvironmentByTaskID(ctx, "task-1")
	if err != nil || env.WorkspacePath != root {
		t.Fatalf("workspace path = %q, %v", env.WorkspacePath, err)
	}
	if len(rescan.calls) != 1 || rescan.calls[0].workDir != root {
		t.Fatalf("rescan calls = %+v", rescan.calls)
	}
}

func TestWorkspaceSourceMaterializer_LocalClonesProviderRepositoryBeforeLinking(t *testing.T) {
	ctx := context.Background()
	repo := newMaterializerRepo(t)
	tasksBase := filepath.Join(t.TempDir(), "tasks")
	mgr := newMaterializerWorktreeMgr(t, filepath.Join(tasksBase, "task-1"))
	seedWorkspaceSourceTask(t, repo, t.TempDir())
	clonePath := filepath.Join(t.TempDir(), "cloned")
	if err := os.MkdirAll(clonePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-remote", WorkspaceID: "ws-1", Name: "remote", Provider: "github", ProviderOwner: "acme", ProviderName: "remote"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{ID: "task-repo-remote", TaskID: "task-1", RepositoryID: "repo-remote", BaseBranch: "main", Position: 0, Metadata: map[string]interface{}{}}); err != nil {
		t.Fatal(err)
	}
	cloner := &hostRepositoryClonerStub{path: clonePath}
	materializer := &workspaceSourceMaterializer{repo: repo, worktreeMgr: mgr, hostCloner: cloner, logger: newTestLogger()}
	if _, err := materializer.MaterializeWorkspaceSources(ctx, "task-1", &models.WorkspaceSourceBatch{TaskID: "task-1"}); err != nil {
		t.Fatalf("MaterializeWorkspaceSources: %v", err)
	}
	if len(cloner.calls) != 1 || cloner.calls[0].ID != "repo-remote" {
		t.Fatalf("clone calls = %+v", cloner.calls)
	}
	if got, err := os.Readlink(filepath.Join(tasksBase, "task-1", "remote")); err != nil || got != clonePath {
		t.Fatalf("repository link = %q, %v; want %q", got, err, clonePath)
	}
}

func TestWorkspaceSourceMaterializer_WorktreeAddsLiveFolderAtTaskRoot(t *testing.T) {
	ctx := context.Background()
	repoPath, taskRoot, primary := setupMaterializerScenario(t)
	repo := newMaterializerRepo(t)
	seedMaterializerTask(t, ctx, repo, repoPath, taskRoot, primary)
	folder := filepath.Join(t.TempDir(), "notes")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "note.txt"), []byte("live"), 0o644); err != nil {
		t.Fatal(err)
	}
	rescan := &workspaceSourceRescanStub{}
	m := &workspaceSourceMaterializer{repo: repo, worktreeMgr: newMaterializerWorktreeMgr(t, taskRoot), rescanner: rescan, logger: newTestLogger()}
	batch := &models.WorkspaceSourceBatch{TaskID: "task-1", Sources: []models.WorkspaceSource{{Folder: &models.TaskWorkspaceFolder{DisplayName: "notes", LocalPath: folder}}}}
	if _, err := m.MaterializeWorkspaceSources(ctx, "task-1", batch); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(taskRoot, "notes", "note.txt")); err != nil || string(got) != "live" {
		t.Fatalf("worktree folder link = %q, %v", got, err)
	}
	if len(rescan.calls) != 1 || rescan.calls[0].workDir != taskRoot {
		t.Fatalf("rescan calls = %+v", rescan.calls)
	}
}

func TestWorkspaceSourceMaterializer_RollsBackLinkAndPathWhenAdoptionFails(t *testing.T) {
	ctx := context.Background()
	repo := newMaterializerRepo(t)
	tasksBase := filepath.Join(t.TempDir(), "tasks")
	mgr := newMaterializerWorktreeMgr(t, filepath.Join(tasksBase, "task-1"))
	source := filepath.Join(t.TempDir(), "notes")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	seedWorkspaceSourceTask(t, repo, source)
	materializer := &workspaceSourceMaterializer{repo: repo, worktreeMgr: mgr, rescanner: &workspaceSourceRescanStub{err: os.ErrPermission}, logger: newTestLogger()}
	batch := &models.WorkspaceSourceBatch{TaskID: "task-1", Sources: []models.WorkspaceSource{{Folder: &models.TaskWorkspaceFolder{DisplayName: "notes", LocalPath: source}}}}
	if _, err := materializer.MaterializeWorkspaceSources(ctx, "task-1", batch); err == nil {
		t.Fatal("MaterializeWorkspaceSources succeeded despite failed adoption")
	}
	if _, err := os.Lstat(filepath.Join(tasksBase, "task-1", "notes")); !os.IsNotExist(err) {
		t.Fatalf("created link remains after rollback: %v", err)
	}
	env, err := repo.GetTaskEnvironmentByTaskID(ctx, "task-1")
	if err != nil || env.WorkspacePath != source {
		t.Fatalf("workspace path = %q, %v; want original %q", env.WorkspacePath, err, source)
	}
}

func TestWorkspaceSourceMaterializer_RebindFailureRestoresEarlierSessionsInReverseOrder(t *testing.T) {
	ctx := context.Background()
	repo := newMaterializerRepo(t)
	tasksBase := filepath.Join(t.TempDir(), "tasks")
	mgr := newMaterializerWorktreeMgr(t, filepath.Join(tasksBase, "task-1"))
	source := filepath.Join(t.TempDir(), "notes")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	seedWorkspaceSourceTask(t, repo, source)
	now := time.Now().UTC()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-2", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput, StartedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	rescan := &orderedWorkspaceRebindStub{failOnCall: 2}
	materializer := &workspaceSourceMaterializer{repo: repo, worktreeMgr: mgr, rescanner: rescan, logger: newTestLogger()}

	batch := &models.WorkspaceSourceBatch{TaskID: "task-1", Sources: []models.WorkspaceSource{{Folder: &models.TaskWorkspaceFolder{DisplayName: "notes", LocalPath: source}}}}
	if _, err := materializer.MaterializeWorkspaceSources(ctx, "task-1", batch); err == nil {
		t.Fatal("MaterializeWorkspaceSources succeeded despite second rebind failure")
	}
	root := filepath.Join(tasksBase, "task-1")
	if len(rescan.calls) != 3 {
		t.Fatalf("rebind calls = %+v, want two adoptions and one restore", rescan.calls)
	}
	if rescan.calls[0].workDir != root || rescan.calls[1].workDir != root || rescan.calls[0].sessionID == rescan.calls[1].sessionID {
		t.Fatalf("adoption calls = %+v, want distinct sessions at %q", rescan.calls[:2], root)
	}
	if !reflect.DeepEqual(rescan.calls[2], stubRescanCall{sessionID: rescan.calls[0].sessionID, workDir: source}) {
		t.Fatalf("restore call = %+v, want reverse restoration to %q", rescan.calls[2], source)
	}
	if len(rescan.roots) != 3 || !reflect.DeepEqual(rescan.roots[0], []string{source}) || !reflect.DeepEqual(rescan.roots[1], []string{source}) || len(rescan.roots[2]) != 0 {
		t.Fatalf("authoritative roots = %+v, want post-state roots followed by prior roots", rescan.roots)
	}
}

func TestWorkspaceSourceMaterializer_WorktreeLateFolderFailureEmitsNoMaterializedEvent(t *testing.T) {
	ctx := context.Background()
	repoPath, taskRoot, primaryPath := setupMaterializerScenario(t)
	repo := newMaterializerRepo(t)
	seedMaterializerTask(t, ctx, repo, repoPath, taskRoot, primaryPath)
	branch := &models.TaskRepository{ID: "tr-branch-2", TaskID: "task-1", RepositoryID: "repo-1", BaseBranch: "main", CheckoutBranch: "branch-2", Position: 1, Metadata: map[string]interface{}{}}
	if err := repo.CreateTaskRepository(ctx, branch); err != nil {
		t.Fatal(err)
	}
	rescanner := &stubRescanner{}
	materializer := &workspaceSourceMaterializer{
		repo:        repo,
		worktreeMgr: newMaterializerWorktreeMgr(t, taskRoot),
		branches:    &branchMaterializer{repo: repo, worktreeMgr: newMaterializerWorktreeMgr(t, taskRoot), rescanner: rescanner, logger: newTestLogger()},
		logger:      newTestLogger(),
	}
	batch := &models.WorkspaceSourceBatch{TaskID: "task-1", Sources: []models.WorkspaceSource{
		{Repository: branch},
		{Folder: &models.TaskWorkspaceFolder{DisplayName: "duplicate", LocalPath: t.TempDir()}},
		{Folder: &models.TaskWorkspaceFolder{DisplayName: "duplicate", LocalPath: t.TempDir()}},
	}}
	if _, err := materializer.MaterializeWorkspaceSources(ctx, "task-1", batch); err == nil {
		t.Fatal("MaterializeWorkspaceSources succeeded despite late folder failure")
	}
	if len(rescanner.notifyCalls) != 0 {
		t.Fatalf("materialized events = %+v, want none", rescanner.notifyCalls)
	}
}

type workspaceSourceRescanStub struct {
	calls []stubRescanCall
	err   error
}

type orderedWorkspaceRebindStub struct {
	calls      []stubRescanCall
	roots      [][]string
	failOnCall int
}

func (s *orderedWorkspaceRebindStub) RebindWorkspaceForSession(_ context.Context, id, dir string, sourceRoots ...[]string) error {
	s.calls = append(s.calls, stubRescanCall{sessionID: id, workDir: dir})
	if len(sourceRoots) == 1 {
		s.roots = append(s.roots, append([]string(nil), sourceRoots[0]...))
	}
	if len(s.calls) == s.failOnCall {
		return errors.New("rebind failed")
	}
	return nil
}

func (s *workspaceSourceRescanStub) RebindWorkspaceForSession(_ context.Context, id, dir string, _ ...[]string) error {
	s.calls = append(s.calls, stubRescanCall{sessionID: id, workDir: dir})
	return s.err
}

func seedWorkspaceSourceTask(t *testing.T, repo *sqliterepo.Repository, source string) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Sources", Priority: "medium"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput, StartedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: "env-1", TaskID: "task-1", ExecutorType: string(models.ExecutorTypeLocal), Status: models.TaskEnvironmentStatusReady, TaskDirName: "task-1", WorkspacePath: source, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
}

func seedRemoteMaterializerRepositories(t *testing.T, repo *sqliterepo.Repository) {
	t.Helper()
	ctx := context.Background()
	for _, repository := range []*models.Repository{
		{ID: "repo-primary", WorkspaceID: "ws-1", Name: "primary", RemoteURL: "https://github.com/acme/primary.git"},
		{ID: "repo-added", WorkspaceID: "ws-1", Name: "added", RemoteURL: "https://github.com/acme/added.git"},
	} {
		if err := repo.CreateRepository(ctx, repository); err != nil {
			t.Fatal(err)
		}
	}
	for _, taskRepository := range []*models.TaskRepository{
		{ID: "task-repo-primary", TaskID: "task-1", RepositoryID: "repo-primary", BaseBranch: "main", Position: 0, Metadata: map[string]interface{}{}},
		{ID: "task-repo-added", TaskID: "task-1", RepositoryID: "repo-added", BaseBranch: "main", Position: 1, Metadata: map[string]interface{}{}},
	} {
		if err := repo.CreateTaskRepository(ctx, taskRepository); err != nil {
			t.Fatal(err)
		}
	}
}
