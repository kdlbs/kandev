package backendapp

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/events/bus"
	mcph "github.com/kandev/kandev/internal/mcp/handlers"
	mcpp "github.com/kandev/kandev/internal/mcp/profile"
	mcps "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/repoclone"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type branchGitCompatibilityFixture struct {
	ctx          context.Context
	repo         *sqliterepo.Repository
	dispatcher   *ws.Dispatcher
	cloner       *managedAttachmentCloner
	origin, root string
}

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.1, AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.3
// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.4, AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.6
func TestManagedBranchGitHubURLGitCompatibility(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture Git transport wrapper requires a POSIX shell")
	}
	for _, scenario := range []string{"current-url", "legacy-url", "legacy-local"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := setupBranchGitCompatibility(t)
			installBranchGitCompatibility(t, fixture.origin, scenario != "current-url")
			payload := map[string]any{"task_id": "task-1", "base_branch": "main", "checkout_branch": "compatibility"}
			if scenario == "legacy-local" {
				payload["local_path"] = fixture.origin
			} else {
				payload["github_url"] = "https://github.com/acme/added"
			}
			msg, err := ws.NewRequest("git-compatibility", ws.ActionMCPAddBranchToTask, payload)
			if err != nil {
				t.Fatal(err)
			}
			response, err := fixture.dispatcher.Dispatch(fixture.ctx, msg)
			if err != nil {
				t.Fatal(err)
			}
			if response.Type != ws.MessageTypeResponse {
				t.Fatalf("valid branch attachment rejected: %s", response.Payload)
			}
			var result struct {
				ID                string `json:"id"`
				RepositoryID      string `json:"repository_id"`
				WorktreePath      string `json:"worktree_path"`
				TaskWorkspacePath string `json:"task_workspace_path"`
				AgentCWDChanged   bool   `json:"agent_cwd_changed"`
			}
			if err := json.Unmarshal(response.Payload, &result); err != nil {
				t.Fatal(err)
			}
			if result.TaskWorkspacePath != fixture.root || result.WorktreePath == "" || result.AgentCWDChanged {
				t.Fatalf("unexpected branch response: %+v", result)
			}
			content, err := os.ReadFile(filepath.Join(result.WorktreePath, "README.md"))
			if err != nil || string(content) != "initial\n" {
				t.Fatalf("attached content=%q err=%v", content, err)
			}
			fixture.verifyInventory(t, result.ID, result.RepositoryID, result.WorktreePath)
			fixture.verifyRepository(t, scenario == "legacy-local", result.RepositoryID)
		})
	}
}

func setupBranchGitCompatibility(t *testing.T) *branchGitCompatibilityFixture {
	t.Helper()
	ctx := context.Background()
	origin, root, primary := setupMaterializerScenario(t)
	repo := newMaterializerRepo(t)
	seedMaterializerTask(t, ctx, repo, origin, root, primary)
	workspace, err := repo.GetWorkspace(ctx, "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	workspace.OwnerID = "probe-owner"
	if err := repo.UpdateWorkspace(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	ctx = authn.WithIdentity(ctx, authn.Identity{UserID: "probe-owner", Role: authn.RoleMember})
	ctx = mcps.WithPrincipal(ctx, mcps.Principal{WorkspaceID: "ws-1", CallerTaskID: "task-1", CallerSessionID: "session-1", Surface: mcpp.SurfaceKanbanTask})
	log := newTestLogger()
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(eventBus.Close)
	svc := taskservice.NewService(taskservice.Repos{Workspaces: repo, Tasks: repo, TaskRepos: repo, WorkspaceFolders: repo, Workflows: repo, Turns: repo, Sessions: repo, RepoEntities: repo, TaskEnvironments: repo, BackgroundWork: repo, Messages: repo, GitSnapshots: repo, Executors: repo, Environments: repo, Usage: repo}, eventBus, log, taskservice.RepositoryDiscoveryConfig{})
	cloner := &managedAttachmentCloner{Cloner: repoclone.NewCloner(repoclone.Config{BasePath: canonicalTempDir(t)}, repoclone.ProtocolHTTPS, "", log)}
	cloner.SetGitCredentialProvider(staticRepoCloneCredential{})
	exc := executor.NewExecutor(nil, nil, log, executor.ExecutorConfig{})
	exc.SetRepoCloner(cloner, &repoLocalPathUpdater{svc: svc})
	mat := newWorkspaceSourceMaterializer(repo, newPersistingMaterializerWorktreeMgr(t, root, repo), nil, log)
	mat.branches.rescanner = &stubRescanner{}
	mat.SetHostRepositoryCloner(exc)
	svc.SetBranchMaterializer(mat.branches)
	dispatcher := ws.NewDispatcher()
	mcph.NewHandlers(svc, nil, nil, nil, nil, repo, repo, eventBus, nil, nil, nil, nil, log).RegisterHandlers(dispatcher)
	return &branchGitCompatibilityFixture{ctx: ctx, repo: repo, dispatcher: dispatcher, cloner: cloner, origin: origin, root: root}
}

func (f *branchGitCompatibilityFixture) verifyInventory(t *testing.T, attachmentID, repositoryID, path string) {
	t.Helper()
	rows, err := f.repo.ListTaskRepositories(f.ctx, "task-1")
	if err != nil || len(rows) != 2 {
		t.Fatalf("attachments=%v err=%v", rows, err)
	}
	if rows[0].ID != "tr-primary" || rows[1].ID != attachmentID || rows[1].RepositoryID != repositoryID || rows[1].BaseBranch != "main" || rows[1].CheckoutBranch != "compatibility" {
		t.Fatalf("requested attachment not persisted: %+v", rows[1])
	}
	inventory, err := f.repo.ListTaskEnvironmentRepos(f.ctx, "env-1")
	if err != nil || len(inventory) != 2 {
		t.Fatalf("inventory=%v err=%v", inventory, err)
	}
	matched := false
	for _, entry := range inventory {
		if entry.RepositoryID == repositoryID && entry.WorktreePath == path {
			matched = true
		}
	}
	if !matched {
		t.Fatal("returned checkout absent from canonical inventory")
	}
	environment, err := f.repo.GetTaskEnvironment(f.ctx, "env-1")
	if err != nil || environment.WorkspacePath != f.root {
		t.Fatalf("canonical workspace root differs from response: environment=%+v err=%v", environment, err)
	}
}

func (f *branchGitCompatibilityFixture) verifyRepository(t *testing.T, local bool, repositoryID string) {
	t.Helper()
	repo, err := f.repo.GetRepository(f.ctx, repositoryID)
	if err != nil {
		t.Fatal(err)
	}
	if local {
		if repo.LocalPath != f.origin || len(f.cloner.requests) != 0 {
			t.Fatal("explicit local source cloned or redirected")
		}
		return
	}
	want, err := f.cloner.WorkspaceRepoPath("ws-1", "github", "acme", "added")
	if err != nil {
		t.Fatal(err)
	}
	if repo.LocalPath != want || repo.RemoteURL != "https://github.com/acme/added.git" || repo.SourceType != "provider" {
		t.Fatalf("managed repository identity/path changed: %+v", repo)
	}
	if len(f.cloner.requests) != 1 {
		t.Fatalf("clone requests=%d, want one", len(f.cloner.requests))
	}
	request := f.cloner.requests[0]
	if request.WorkspaceID != "ws-1" || request.TaskID != "task-1" || request.SessionID != "session-1" || request.RepositoryID != repositoryID {
		t.Fatalf("clone request lost exact scope: %+v", request)
	}
}

func installBranchGitCompatibility(t *testing.T, origin string, legacy bool) {
	t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
	script := "#!/bin/sh\n"
	if legacy {
		// Emulate the unsupported option, retaining actual Git path inspection.
		script += "if [ \"$3\" = rev-parse ] && [ \"$4\" = --path-format=absolute ]; then\n" +
			"  printf '%s\\n' --path-format=absolute\n" +
			"  exec " + quote(realGit) + " -C \"$2\" rev-parse \"$5\"\n" +
			"fi\n"
	}
	// Keep the public URL through resolution/auth; only transport uses local content.
	script += "exec " + quote(realGit) + " -c protocol.file.allow=always -c " +
		quote("url.file://"+origin+".insteadOf=https://github.com/acme/added.git") + " \"$@\"\n"
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}
