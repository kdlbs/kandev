package backendapp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/worktree"
)

func TestWorkspaceSourceMaterializer_LocalFolderCreatesLiveTaskEntryAtEstablishedRoot(t *testing.T) {
	ctx := context.Background()
	repo := newMaterializerRepo(t)
	tasksBase := filepath.Join(canonicalTempDir(t), "tasks")
	mgr := newMaterializerWorktreeMgr(t, filepath.Join(tasksBase, "task-1"))
	workspaceRoot := t.TempDir()
	source := filepath.Join(t.TempDir(), "notes")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "note.txt"), []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	seedWorkspaceSourceTask(t, repo, workspaceRoot)
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
	root := workspaceRoot
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
	tasksBase := filepath.Join(canonicalTempDir(t), "tasks")
	mgr := newMaterializerWorktreeMgr(t, filepath.Join(tasksBase, "task-1"))
	workspaceRoot := t.TempDir()
	source := filepath.Join(t.TempDir(), "notes")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "note.txt"), []byte("persisted"), 0o644); err != nil {
		t.Fatal(err)
	}
	seedWorkspaceSourceTask(t, repo, workspaceRoot)
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

	rescan := &workspaceSourceRescanStub{}
	materializer := &workspaceSourceMaterializer{repo: repo, worktreeMgr: mgr, rescanner: rescan, logger: newTestLogger()}
	if _, err := materializer.MaterializeWorkspaceSources(ctx, "task-1", batch); err != nil {
		t.Fatalf("MaterializeWorkspaceSources after persistence: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(workspaceRoot, "notes", "note.txt")); err != nil || string(got) != "persisted" {
		t.Fatalf("persisted folder entry = %q, %v", got, err)
	}
}

func TestWorkspaceSourceMaterializer_LocalEstablishedRootPreservesPrimaryRepository(t *testing.T) {
	ctx := context.Background()
	repo := newMaterializerRepo(t)
	tasksBase := filepath.Join(canonicalTempDir(t), "tasks")
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
	root := primary
	for _, file := range []string{"repo.txt", "notes/note.txt"} {
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
	tasksBase := filepath.Join(canonicalTempDir(t), "tasks")
	mgr := newMaterializerWorktreeMgr(t, filepath.Join(tasksBase, "task-1"))
	workspaceRoot := t.TempDir()
	seedWorkspaceSourceTask(t, repo, workspaceRoot)
	clonePath := filepath.Join(canonicalTempDir(t), "cloned")
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
	rescan := &workspaceSourceRescanStub{}
	materializer := &workspaceSourceMaterializer{repo: repo, worktreeMgr: mgr, hostCloner: cloner, rescanner: rescan, logger: newTestLogger()}
	if _, err := materializer.MaterializeWorkspaceSources(ctx, "task-1", &models.WorkspaceSourceBatch{TaskID: "task-1"}); err != nil {
		t.Fatalf("MaterializeWorkspaceSources: %v", err)
	}
	if len(cloner.calls) != 1 || cloner.calls[0].ID != "repo-remote" {
		t.Fatalf("clone calls = %+v", cloner.calls)
	}
	if cloner.taskID != "task-1" || cloner.sessionID != "session-1" {
		t.Fatalf("clone scope = task %q session %q", cloner.taskID, cloner.sessionID)
	}
	if got, err := os.Readlink(filepath.Join(workspaceRoot, "remote")); err != nil || got != clonePath {
		t.Fatalf("repository link = %q, %v; want %q", got, err, clonePath)
	}
}

func TestWorkspaceSourceMaterializer_WorktreeFolderPreservesEstablishedRoot(t *testing.T) {
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
	rescan := &nestedWorkspaceRescanStub{}
	m := &workspaceSourceMaterializer{repo: repo, worktreeMgr: newMaterializerWorktreeMgr(t, taskRoot), rescanner: rescan, logger: newTestLogger()}
	batch := &models.WorkspaceSourceBatch{TaskID: "task-1", Sources: []models.WorkspaceSource{{Folder: &models.TaskWorkspaceFolder{DisplayName: "notes", LocalPath: folder}}}}
	if _, err := m.MaterializeWorkspaceSources(ctx, "task-1", batch); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(primary, "notes", "note.txt")); err != nil || string(got) != "live" {
		t.Fatalf("worktree folder link = %q, %v", got, err)
	}
	if len(rescan.rescanCalls) != 1 || rescan.rescanCalls[0].workDir != primary {
		t.Fatalf("rescan calls = %+v", rescan.rescanCalls)
	}
	if len(rescan.rebindCalls) != 0 {
		t.Fatal("folder attachment rebound the session")
	}
	env, err := repo.GetTaskEnvironmentByTaskID(ctx, "task-1")
	if err != nil || env.WorkspacePath != primary {
		t.Fatalf("workspace changed: %+v, %v", env, err)
	}
}

func TestWorkspaceSourceMaterializer_NestedRepositoryKeepsAgentRootAndRescans(t *testing.T) {
	ctx := context.Background()
	repoPath, taskRoot, primaryPath := setupMaterializerScenario(t)
	repo := newMaterializerRepo(t)
	seedMaterializerTask(t, ctx, repo, repoPath, taskRoot, primaryPath)
	branch := &models.TaskRepository{
		ID:                    "tr-nested",
		TaskID:                "task-1",
		RepositoryID:          "repo-1",
		WorkspaceRelativePath: "kandev/added",
		BaseBranch:            "main",
		CheckoutBranch:        "branch-2",
		Position:              1,
		Metadata:              map[string]interface{}{},
	}
	if err := repo.CreateTaskRepository(ctx, branch); err != nil {
		t.Fatal(err)
	}
	rescan := &nestedWorkspaceRescanStub{}
	mgr := newMaterializerWorktreeMgr(t, taskRoot)
	m := &workspaceSourceMaterializer{
		repo:        repo,
		worktreeMgr: mgr,
		branches:    &branchMaterializer{repo: repo, worktreeMgr: mgr, rescanner: rescan, logger: newTestLogger()},
		rescanner:   rescan,
		logger:      newTestLogger(),
	}

	batch := &models.WorkspaceSourceBatch{
		TaskID:              "task-1",
		RepositoryPlacement: string(taskservice.WorkspacePlacementCurrentRoot),
		Sources:             []models.WorkspaceSource{{Repository: branch}},
	}
	result, err := m.MaterializeWorkspaceSources(ctx, "task-1", batch)
	if err != nil {
		t.Fatalf("MaterializeWorkspaceSources: %v", err)
	}

	wantPath := filepath.Join(primaryPath, "added")
	if result == nil || result.WorkspacePath != primaryPath || !reflect.DeepEqual(result.SessionIDs, []string{"session-1"}) {
		t.Fatalf("materialization result = %#v, want primary workspace and session-1", result)
	}
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("nested worktree = %s: %v", wantPath, err)
	}
	env, err := repo.GetTaskEnvironmentByTaskID(ctx, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if env.WorkspacePath != primaryPath {
		t.Fatalf("workspace path = %q, want unchanged primary path %q", env.WorkspacePath, primaryPath)
	}
	if len(rescan.rebindCalls) != 0 {
		t.Fatalf("nested attachment rebound the running session: %+v", rescan.rebindCalls)
	}
	if len(rescan.rescanCalls) != 1 || rescan.rescanCalls[0].workDir != primaryPath {
		t.Fatalf("nested attachment rescan calls = %+v, want one rescan at %q", rescan.rescanCalls, primaryPath)
	}
	if len(rescan.notifyCalls) != 1 || rescan.notifyCalls[0].WorktreePath != wantPath || rescan.notifyCalls[0].TaskWorkspacePath != primaryPath {
		t.Fatalf("nested materialized events = %+v, want %q under %q", rescan.notifyCalls, wantPath, primaryPath)
	}
	envRepos, err := repo.ListTaskEnvironmentRepos(ctx, env.ID)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, row := range envRepos {
		if row.RepositoryID == "repo-1" && row.WorktreePath == wantPath && row.WorkspaceRelativePath == "kandev/added" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("environment repository inventory = %+v, want nested path", envRepos)
	}
}

func TestWorkspaceSourceMaterializer_ParentRootAdditionRescansWithoutRebind(t *testing.T) {
	ctx := context.Background()
	repoPath, taskRoot, primaryPath := setupMaterializerScenario(t)
	repo := newMaterializerRepo(t)
	seedMaterializerTask(t, ctx, repo, repoPath, taskRoot, primaryPath)
	env, err := repo.GetTaskEnvironmentByTaskID(ctx, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	env.WorkspacePath = taskRoot
	env.WorkspaceLayout = taskservice.WorkspaceLayoutTaskRoot
	if err := repo.UpdateTaskEnvironment(ctx, env); err != nil {
		t.Fatalf("UpdateTaskEnvironment: %v", err)
	}
	branch := &models.TaskRepository{
		ID: "tr-parent-root", TaskID: "task-1", RepositoryID: "repo-1",
		WorkspaceRelativePath: "added", BaseBranch: "main", CheckoutBranch: "branch-parent-root",
		Position: 1, Metadata: map[string]interface{}{},
	}
	if err := repo.CreateTaskRepository(ctx, branch); err != nil {
		t.Fatalf("CreateTaskRepository: %v", err)
	}
	rescan := &nestedWorkspaceRescanStub{}
	mgr := newMaterializerWorktreeMgr(t, taskRoot)
	m := &workspaceSourceMaterializer{
		repo: repo, worktreeMgr: mgr,
		branches:  &branchMaterializer{repo: repo, worktreeMgr: mgr, rescanner: rescan, logger: newTestLogger()},
		rescanner: rescan, logger: newTestLogger(),
	}

	result, err := m.MaterializeWorkspaceSources(ctx, "task-1", &models.WorkspaceSourceBatch{
		TaskID: "task-1", Sources: []models.WorkspaceSource{{Repository: branch}},
		RepositoryPlacement: string(taskservice.WorkspacePlacementCurrentRoot),
	})
	if err != nil {
		t.Fatalf("MaterializeWorkspaceSources: %v", err)
	}
	wantPath := filepath.Join(taskRoot, "added")
	if result == nil || result.WorkspacePath != taskRoot || !reflect.DeepEqual(result.SessionIDs, []string{"session-1"}) {
		t.Fatalf("materialization result = %#v, want parent workspace and session-1", result)
	}
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("parent-root worktree = %s: %v", wantPath, err)
	}
	if len(rescan.rebindCalls) != 0 {
		t.Fatalf("parent-root addition rebound the session: %+v", rescan.rebindCalls)
	}
	if len(rescan.rescanCalls) != 1 || rescan.rescanCalls[0].workDir != taskRoot {
		t.Fatalf("parent-root rescan calls = %+v, want one rescan at %q", rescan.rescanCalls, taskRoot)
	}
}

func TestWorkspaceSourceInventoryRowsKeepSameRepositoryBranchesDistinct(t *testing.T) {
	first := &models.TaskRepository{
		ID:                    "task-repo-first",
		RepositoryID:          "repo-shared",
		BaseBranch:            "main",
		CheckoutBranch:        "feature/first",
		WorkspaceRelativePath: "repo/first",
		Position:              0,
	}
	second := &models.TaskRepository{
		ID:                    "task-repo-second",
		RepositoryID:          "repo-shared",
		BaseBranch:            "main",
		CheckoutBranch:        "feature/second",
		WorkspaceRelativePath: "repo/second",
		Position:              1,
	}
	rows := workspaceSourceInventoryRows(
		"env-1",
		string(models.ExecutorTypeWorktree),
		&models.WorkspaceSourceBatch{Sources: []models.WorkspaceSource{{Repository: first}, {Repository: second}}},
		[]*branchMaterialization{
			{taskRepositoryID: first.ID, repositoryID: first.RepositoryID, slug: "feature-first", worktree: &worktree.Worktree{ID: "wt-first", Path: "/tasks/task-1/repo/first", Branch: "feature/first"}},
			{taskRepositoryID: second.ID, repositoryID: second.RepositoryID, slug: "feature-second", worktree: &worktree.Worktree{ID: "wt-second", Path: "/tasks/task-1/repo/second", Branch: "feature/second"}},
		},
		nil,
	)
	if len(rows) != 2 {
		t.Fatalf("inventory rows = %+v, want two rows", rows)
	}
	if rows[0].WorktreeID != "wt-first" || rows[1].WorktreeID != "wt-second" {
		t.Fatalf("inventory rows aliased same-repository branches: %+v", rows)
	}
}

func TestWorkspaceSourceMaterializer_NestedRescanFailureRestoresCompletedSessionsAndLeavesNoInventory(t *testing.T) {
	ctx := context.Background()
	repoPath, taskRoot, primaryPath := setupMaterializerScenario(t)
	repo := newMaterializerRepo(t)
	seedMaterializerTask(t, ctx, repo, repoPath, taskRoot, primaryPath)
	now := time.Now().UTC()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-2", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput, StartedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	branch := &models.TaskRepository{
		ID:                    "tr-nested-rollback",
		TaskID:                "task-1",
		RepositoryID:          "repo-1",
		WorkspaceRelativePath: "kandev/rollback",
		BaseBranch:            "main",
		CheckoutBranch:        "branch-rollback",
		Position:              1,
		Metadata:              map[string]interface{}{},
	}
	if err := repo.CreateTaskRepository(ctx, branch); err != nil {
		t.Fatalf("CreateTaskRepository: %v", err)
	}
	rescan := &failingWorkspaceRescanStub{failOnRescan: 2}
	mgr := newMaterializerWorktreeMgr(t, taskRoot)
	m := &workspaceSourceMaterializer{
		repo:        repo,
		worktreeMgr: mgr,
		branches:    &branchMaterializer{repo: repo, worktreeMgr: mgr, rescanner: rescan, logger: newTestLogger()},
		rescanner:   rescan,
		logger:      newTestLogger(),
	}

	_, err := m.MaterializeWorkspaceSources(ctx, "task-1", &models.WorkspaceSourceBatch{
		TaskID:              "task-1",
		RepositoryPlacement: string(taskservice.WorkspacePlacementCurrentRoot),
		Sources:             []models.WorkspaceSource{{Repository: branch}},
	})
	if err == nil {
		t.Fatal("MaterializeWorkspaceSources succeeded despite second session rescan failure")
	}
	if len(rescan.rescanCalls) != 3 || len(rescan.rebindCalls) != 0 {
		t.Fatalf("rescan calls = %+v, rebind calls = %+v; want two forward rescans, one rescan rollback, and no rebind", rescan.rescanCalls, rescan.rebindCalls)
	}
	if rescan.rescanCalls[2].sessionID != rescan.rescanCalls[0].sessionID || rescan.rescanCalls[2].workDir != primaryPath {
		t.Fatalf("rollback rescan = %+v, want first session restored to %q", rescan.rescanCalls[2], primaryPath)
	}
	envRepos, err := repo.ListTaskEnvironmentRepos(ctx, "env-1")
	if err != nil {
		t.Fatalf("ListTaskEnvironmentRepos: %v", err)
	}
	for _, row := range envRepos {
		if row != nil && row.WorkspaceRelativePath == "kandev/rollback" {
			t.Fatalf("nested inventory row survived failed rescan: %+v", row)
		}
	}
	if _, err := os.Stat(filepath.Join(taskRoot, "kandev", "rollback")); !os.IsNotExist(err) {
		t.Fatalf("new worktree survived rollback: %v", err)
	}
}

func TestWorkspaceSourceMaterializer_NestedInventoryFailureRestoresWithRescan(t *testing.T) {
	ctx := context.Background()
	repoPath, taskRoot, primaryPath := setupMaterializerScenario(t)
	baseRepo := newMaterializerRepo(t)
	seedMaterializerTask(t, ctx, baseRepo, repoPath, taskRoot, primaryPath)
	branch := &models.TaskRepository{
		ID: "tr-nested-persist-failure", TaskID: "task-1", RepositoryID: "repo-1",
		WorkspaceRelativePath: "kandev/persist-failure", BaseBranch: "main", CheckoutBranch: "branch-persist-failure",
		Position: 1, Metadata: map[string]interface{}{},
	}
	if err := baseRepo.CreateTaskRepository(ctx, branch); err != nil {
		t.Fatalf("CreateTaskRepository: %v", err)
	}
	rescan := &nestedWorkspaceRescanStub{}
	mgr := newMaterializerWorktreeMgr(t, taskRoot)
	materializerRepo := &failingEnvironmentInventoryRepo{
		workspaceSourceMaterializerRepo: baseRepo,
		err:                             errors.New("inventory persistence failed"),
	}
	m := &workspaceSourceMaterializer{
		repo: materializerRepo, worktreeMgr: mgr,
		branches:  &branchMaterializer{repo: baseRepo, worktreeMgr: mgr, rescanner: rescan, logger: newTestLogger()},
		rescanner: rescan, logger: newTestLogger(),
	}

	_, err := m.MaterializeWorkspaceSources(ctx, "task-1", &models.WorkspaceSourceBatch{
		TaskID: "task-1", RepositoryPlacement: string(taskservice.WorkspacePlacementCurrentRoot),
		Sources: []models.WorkspaceSource{{Repository: branch}},
	})
	if err == nil {
		t.Fatal("MaterializeWorkspaceSources succeeded despite inventory persistence failure")
	}
	if len(rescan.rebindCalls) != 0 {
		t.Fatalf("inventory failure used workspace rebind for rollback: %+v", rescan.rebindCalls)
	}
	if len(rescan.rescanCalls) != 2 || rescan.rescanCalls[0].workDir != primaryPath || rescan.rescanCalls[1].workDir != primaryPath {
		t.Fatalf("rescan calls = %+v, want forward and rollback rescans at %q", rescan.rescanCalls, primaryPath)
	}
	if _, err := os.Stat(filepath.Join(taskRoot, "kandev", "persist-failure")); !os.IsNotExist(err) {
		t.Fatalf("new worktree survived inventory rollback: %v", err)
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

func TestRollbackOwnedDirectoryLinkPreservesReplacementFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes")
	const contents = "user replacement"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	err := rollbackOwnedDirectoryLink(ownedDirectoryLinkUndo{Path: path})
	if err == nil {
		t.Fatal("rollbackOwnedDirectoryLink removed a replacement file")
	}
	if got, readErr := os.ReadFile(path); readErr != nil || string(got) != contents {
		t.Fatalf("replacement file = %q, %v; want it preserved", got, readErr)
	}
}

func TestWorkspaceSourceMaterializer_RestoresRepointedLinkWhenAdoptionFails(t *testing.T) {
	ctx := context.Background()
	repo := newMaterializerRepo(t)
	tasksBase := filepath.Join(canonicalTempDir(t), "tasks")
	root := filepath.Join(tasksBase, "task-1")
	mgr := newMaterializerWorktreeMgr(t, root)
	original := filepath.Join(canonicalTempDir(t), "original-notes")
	replacement := filepath.Join(canonicalTempDir(t), "replacement-notes")
	for path, content := range map[string]string{original: "before", replacement: "after"} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "note.txt"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	seedWorkspaceSourceTask(t, repo, root)
	if _, err := worktree.CreateOwnedDirectoryLink(root, "notes", original); err != nil {
		t.Fatalf("seed owned directory link: %v", err)
	}

	materializer := &workspaceSourceMaterializer{repo: repo, worktreeMgr: mgr, rescanner: &workspaceSourceRescanStub{err: os.ErrPermission}, logger: newTestLogger()}
	batch := &models.WorkspaceSourceBatch{TaskID: "task-1", Sources: []models.WorkspaceSource{{Folder: &models.TaskWorkspaceFolder{DisplayName: "notes", LocalPath: replacement}}}}
	if _, err := materializer.MaterializeWorkspaceSources(ctx, "task-1", batch); err == nil {
		t.Fatal("MaterializeWorkspaceSources succeeded despite failed adoption")
	}
	if got, err := os.ReadFile(filepath.Join(root, "notes", "note.txt")); err != nil || string(got) != "before" {
		t.Fatalf("repointed link after rollback = %q, %v; want original target restored", got, err)
	}
}

func TestWorkspaceSourceMaterializer_RescanFailureRestoresEarlierSessionsInReverseOrder(t *testing.T) {
	ctx := context.Background()
	repo := newMaterializerRepo(t)
	tasksBase := filepath.Join(canonicalTempDir(t), "tasks")
	mgr := newMaterializerWorktreeMgr(t, filepath.Join(tasksBase, "task-1"))
	workspaceRoot := filepath.Join(tasksBase, "task-1")
	source := filepath.Join(canonicalTempDir(t), "notes")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	seedWorkspaceSourceTask(t, repo, workspaceRoot)
	now := time.Now().UTC()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-2", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput, StartedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	rescan := &orderedWorkspaceRebindStub{failOnCall: 2}
	materializer := &workspaceSourceMaterializer{repo: repo, worktreeMgr: mgr, rescanner: rescan, logger: newTestLogger()}

	batch := &models.WorkspaceSourceBatch{TaskID: "task-1", Sources: []models.WorkspaceSource{{Folder: &models.TaskWorkspaceFolder{DisplayName: "notes", LocalPath: source}}}}
	if _, err := materializer.MaterializeWorkspaceSources(ctx, "task-1", batch); err == nil {
		t.Fatal("MaterializeWorkspaceSources succeeded despite second rescan failure")
	}
	root := workspaceRoot
	if len(rescan.calls) != 3 {
		t.Fatalf("rebind calls = %+v, want two adoptions and one restore", rescan.calls)
	}
	if rescan.calls[0].workDir != root || rescan.calls[1].workDir != root || rescan.calls[0].sessionID == rescan.calls[1].sessionID {
		t.Fatalf("adoption calls = %+v, want distinct sessions at %q", rescan.calls[:2], root)
	}
	if !reflect.DeepEqual(rescan.calls[2], stubRescanCall{sessionID: rescan.calls[0].sessionID, workDir: root}) {
		t.Fatalf("restore call = %+v, want reverse restoration to %q", rescan.calls[2], root)
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

func TestWorkspaceSourceMaterializer_FolderGitProtectionAndRollback(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			runGit(t, root, "init")
			repo := newMaterializerRepo(t)
			seedWorkspaceSourceTask(t, repo, root)
			rescan := &workspaceSourceRescanStub{}
			if fail {
				rescan.err = os.ErrPermission
			}
			m := &workspaceSourceMaterializer{repo: repo, worktreeMgr: newMaterializerWorktreeMgr(t, filepath.Join(t.TempDir(), "task-1")), rescanner: rescan, logger: newTestLogger()}
			_, err := m.MaterializeWorkspaceSources(ctx, "task-1", &models.WorkspaceSourceBatch{TaskID: "task-1", Sources: []models.WorkspaceSource{{Folder: &models.TaskWorkspaceFolder{DisplayName: "notes", LocalPath: t.TempDir()}}}})
			if fail {
				if err == nil {
					t.Fatal("rescan failure was ignored")
				}
				if _, err := os.Lstat(filepath.Join(root, "notes")); !os.IsNotExist(err) {
					t.Fatalf("link survived rollback: %v", err)
				}
				if err := os.WriteFile(filepath.Join(root, "notes"), []byte("user file"), 0o644); err != nil {
					t.Fatal(err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			ignored := exec.Command("git", "-C", root, "check-ignore", "--quiet", "--", "notes").Run() == nil
			if ignored == fail {
				t.Fatalf("ignored=%v after failure=%v", ignored, fail)
			}
		})
	}
}
