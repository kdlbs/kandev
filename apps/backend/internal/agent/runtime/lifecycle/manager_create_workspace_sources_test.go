package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestLaunchResolveWorkspacePathUsesSingleFolderAsWorkspace(t *testing.T) {
	mgr := newTestManager(t)
	mgr.dataDir = t.TempDir()
	folder := t.TempDir()
	req := &LaunchRequest{
		TaskID:           "task-folder",
		WorkspaceID:      "workspace-folder",
		SessionID:        "session-folder",
		ExecutorType:     string(models.ExecutorTypeLocal),
		WorkspaceFolders: []WorkspaceFolderSpec{{Name: "assets", LocalPath: folder}},
	}

	got, _, _, _ := mgr.launchResolveWorkspacePath(context.Background(), req)
	if got != folder {
		t.Fatalf("workspace path = %q, want selected folder %q", got, folder)
	}
}

func TestLaunchResolveWorkspacePathCreatesManagedRootForMultipleFolders(t *testing.T) {
	mgr := newTestManager(t)
	mgr.dataDir = t.TempDir()
	first, second := t.TempDir(), t.TempDir()
	req := &LaunchRequest{
		TaskID:       "task-folders",
		WorkspaceID:  "workspace-folders",
		SessionID:    "session-folders",
		ExecutorType: string(models.ExecutorTypeLocal),
		WorkspaceFolders: []WorkspaceFolderSpec{
			{Name: "first", LocalPath: first},
			{Name: "second", LocalPath: second},
		},
	}

	got, _, _, _ := mgr.launchResolveWorkspacePath(context.Background(), req)
	want := filepath.Join(mgr.dataDir, "tasks", req.WorkspaceID, req.TaskID)
	if got != want {
		t.Fatalf("workspace path = %q, want managed root %q", got, want)
	}
	if got == first || got == second {
		t.Fatal("multiple folders must not use a source folder as the workspace root")
	}
}

func TestReconcileWorkspaceSourcesSkipsFolderThatIsWorkspaceRoot(t *testing.T) {
	root := t.TempDir()
	err := reconcileWorkspaceSources(context.Background(), root, []WorkspaceFolderSpec{{Name: "root", LocalPath: root}}, testWorkspaceLinkOwner())
	if err != nil {
		t.Fatalf("reconcileWorkspaceSources: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "root")); !os.IsNotExist(err) {
		t.Fatalf("self-referential folder entry = %v, want no entry", err)
	}
}

func TestReconcileWorkspaceSourcesRejectsDuplicateFolderTarget(t *testing.T) {
	root, source := t.TempDir(), t.TempDir()
	err := reconcileWorkspaceSources(context.Background(), root, []WorkspaceFolderSpec{
		{Name: "one", LocalPath: source},
		{Name: "two", LocalPath: source},
	}, testWorkspaceLinkOwner())
	if err == nil {
		t.Fatal("duplicate folder target was accepted")
	}
}

func TestValidateWorkspaceInfoForExecutionAcceptsFolderOnlyWorkspace(t *testing.T) {
	folder := t.TempDir()
	err := validateWorkspaceInfoForExecution(context.Background(), &WorkspaceInfo{
		ExecutorType:  string(models.ExecutorTypeLocal),
		WorkspacePath: folder,
		WorkspaceFolders: []WorkspaceFolderSpec{{
			Name: "folder", LocalPath: folder,
		}},
	})
	if err != nil {
		t.Fatalf("validateWorkspaceInfoForExecution: %v", err)
	}
}

func TestValidateWorkspaceInfoForExecutionAcceptsMixedWorkspaceBeforeLinksExist(t *testing.T) {
	repository := initGitRepo(t)
	folder := t.TempDir()
	root := t.TempDir()
	err := validateWorkspaceInfoForExecution(context.Background(), &WorkspaceInfo{
		ExecutorType:  string(models.ExecutorTypeLocal),
		WorkspacePath: root,
		WorkspaceFolders: []WorkspaceFolderSpec{{
			Name: "assets", LocalPath: folder,
		}},
		WorkspaceRepositories: []WorkspaceRepositorySpec{{
			RepositoryID: "repository", RepositoryPath: repository, RepoName: "repository",
		}},
	})
	if err != nil {
		t.Fatalf("validateWorkspaceInfoForExecution: %v", err)
	}
}

func TestValidateWorkspaceFolderExecutorRejectsRemoteExecutor(t *testing.T) {
	err := validateWorkspaceFolderExecutor(string(models.ExecutorTypeLocalDocker), []WorkspaceFolderSpec{{
		Name: "assets", LocalPath: t.TempDir(),
	}})
	if err == nil {
		t.Fatal("remote executor accepted host folder")
	}
}

func TestResolveManagedWorkspaceRootUsesParentOfPreparedWorktree(t *testing.T) {
	mgr := newTestManager(t)
	root := t.TempDir()
	repository := filepath.Join(t.TempDir(), "repository")
	got := mgr.resolveManagedWorkspaceRoot(context.Background(), &LaunchRequest{
		UseWorktree:    true,
		RepositoryPath: repository,
		Repositories: []RepoLaunchSpec{{
			RepositoryID: "repository", RepositoryPath: repository, RepoName: "repository",
		}},
	}, &EnvPrepareResult{WorkspacePath: filepath.Join(root, "repository")}, "")
	if got != root {
		t.Fatalf("managed root = %q, want %q", got, root)
	}
}
