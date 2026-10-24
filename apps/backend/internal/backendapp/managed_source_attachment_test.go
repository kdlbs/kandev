package backendapp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"io/fs"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/events/bus"
	mcph "github.com/kandev/kandev/internal/mcp/handlers"
	mcpp "github.com/kandev/kandev/internal/mcp/profile"
	mcps "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/repoclone"
	taskh "github.com/kandev/kandev/internal/task/handlers"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.1, AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.2, AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.3
type managedAttachmentCloner struct {
	*repoclone.Cloner
	requests []repoclone.GitCredentialRequest
	after    func()
}

func (c *managedAttachmentCloner) EnsureWorkspaceClonedWithCredentialRequestAndState(ctx context.Context, r repoclone.GitCredentialRequest, host, token string) (string, repoclone.RemoteRefState, error) {
	c.requests = append(c.requests, r)
	path, state, err := c.Cloner.EnsureWorkspaceClonedWithCredentialRequestAndState(ctx, r, host, token)
	if c.after != nil {
		c.after()
	}
	return path, state, err
}

func TestManagedSourceAttachmentConsumers(t *testing.T) {
	for _, route := range []string{"http-source", "mcp-source", "http-local-source", "mcp-local-source"} {
		for _, scenario := range []string{"empty", "missing", "valid", "local-valid", "local-missing", "foreign", "clone-failure", "legacy-valid", "legacy-missing", "persistence", "late-failure", "pathless-legacy", "rebound-session", "changed-environment", "scoped-missing", "repository-changed", "repository-changed-missing"} {
			t.Run(route+"/"+scenario, func(t *testing.T) { managedAttachment(t, route, scenario) })
		}
	}
}

func managedAttachment(t *testing.T, route, scenario string) {
	ctx := context.Background()
	origin, root, primary := setupMaterializerScenario(t)
	repo := newMaterializerRepo(t)
	seedMaterializerTask(t, ctx, repo, origin, root, primary)
	w, err := repo.GetWorkspace(ctx, "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	w.OwnerID = "probe-owner"
	if err := repo.UpdateWorkspace(ctx, w); err != nil {
		t.Fatal(err)
	}
	ctx = authn.WithIdentity(ctx, authn.Identity{UserID: "probe-owner", Role: authn.RoleMember})
	ctx = mcps.WithPrincipal(ctx, mcps.Principal{WorkspaceID: "ws-1", CallerTaskID: "task-1", CallerSessionID: "session-1", Surface: mcpp.SurfaceKanbanTask})
	log := newTestLogger()
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(eventBus.Close)
	svc := taskservice.NewService(taskservice.Repos{Workspaces: repo, Tasks: repo, TaskRepos: repo, WorkspaceFolders: repo, Workflows: repo, Turns: repo, Sessions: repo, RepoEntities: repo, TaskEnvironments: repo, BackgroundWork: repo, Messages: repo, GitSnapshots: repo, Executors: repo, Environments: repo, Usage: repo}, eventBus, log, taskservice.RepositoryDiscoveryConfig{})
	cloner := repoclone.NewCloner(repoclone.Config{BasePath: managedAttachmentCloneBase(t, scenario)}, repoclone.ProtocolHTTPS, "", log)
	managed, err := cloner.WorkspaceProviderRepoPath("ws-1", "gitlab", "https://gitlab.example", "acme", managedAttachmentProviderName(scenario))
	if err != nil {
		t.Fatal(err)
	}
	saved := managed
	sourceType := "provider"
	switch scenario {
	case "empty", "alias-empty", "persistence", "late-failure", "pathless-legacy", "rebound-session", "changed-environment", "active", "inherited", "persistence-retry", "cloner-unavailable", "repository-changed":
		saved = ""
		if scenario == "pathless-legacy" {
			sourceType = ""
		}
	case "legacy-valid", "legacy-missing", "scoped-legacy":
		saved, err = cloner.ProviderRepoPath("gitlab", "https://gitlab.example", "acme", "added")
		if err != nil {
			t.Fatal(err)
		}
		if scenario == "legacy-valid" || scenario == "scoped-legacy" {
			if err := os.MkdirAll(filepath.Dir(saved), 0755); err != nil {
				t.Fatal(err)
			}
			runGit(t, origin, "clone", origin, saved)
		}
	case "valid", "alias-valid", "space-valid", "newline-valid":
		if err := os.MkdirAll(filepath.Dir(managed), 0755); err != nil {
			t.Fatal(err)
		}
		runGit(t, origin, "clone", origin, managed)
	case "local-valid":
		sourceType = "local"
		saved = origin
	case "local-missing":
		sourceType = "local"
		saved = filepath.Join(canonicalTempDir(t), "absent-local")
	case "foreign":
		saved = filepath.Join(canonicalTempDir(t), "foreign")
		runGit(t, origin, "clone", origin, saved)
		runGit(t, saved, "remote", "set-url", "origin", "https://foreign.example/acme/other.git")
		if err := os.WriteFile(filepath.Join(saved, "sentinel"), []byte("preserve"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if scenario == "clone-failure" {
		saved = ""
		origin = filepath.Join(canonicalTempDir(t), "absent-origin")
	}
	if strings.Contains(route, "local-source") {
		env, err := repo.GetTaskEnvironmentByTaskID(ctx, "task-1")
		if err != nil {
			t.Fatal(err)
		}
		env.ExecutorType = "local"
		env.WorkspacePath = canonicalTempDir(t)
		if err := repo.UpdateTaskEnvironment(ctx, env); err != nil {
			t.Fatal(err)
		}
	}
	var foreignSnapshot map[string]string
	if scenario == "foreign" || scenario == "scoped-legacy" {
		foreignSnapshot = managedAttachmentSnapshot(t, saved)
	}
	entity := &models.Repository{ID: "probe-added", WorkspaceID: "ws-1", Name: "added", SourceType: sourceType, Provider: "gitlab", ProviderHost: "https://gitlab.example", ProviderOwner: "acme", ProviderName: managedAttachmentProviderName(scenario), RemoteURL: origin, LocalPath: saved, DefaultBranch: "main"}
	if scenario == "scoped-legacy" {
		entity.ProviderScope = "connection-scope"
		entity.ProviderRepoID = "immutable-project"
		managed, err = cloner.WorkspaceProviderRepositoryPath("ws-1", entity.Provider, entity.ProviderHost, entity.ProviderScope, entity.ProviderRepoID, entity.ProviderOwner, entity.ProviderName)
		if err != nil {
			t.Fatal(err)
		}
	}
	if scenario == "scoped-missing" {
		entity.ProviderScope = "connection-scope"
		entity.ProviderRepoID = "immutable-project"
		entity.ProviderOwner = ""
		entity.ProviderName = ""
		managed, err = cloner.WorkspaceProviderRepositoryPath("ws-1", entity.Provider, entity.ProviderHost, entity.ProviderScope, entity.ProviderRepoID, "", "")
		if err != nil {
			t.Fatal(err)
		}
		saved = managed
		entity.LocalPath = saved
	}
	if err := repo.CreateRepository(ctx, entity); err != nil {
		t.Fatal(err)
	}
	exc := executor.NewExecutor(nil, nil, log, executor.ExecutorConfig{})
	observedCloner := &managedAttachmentCloner{Cloner: cloner}
	if scenario == "rebound-session" {
		observedCloner.after = func() {
			session, e := repo.GetTaskSession(ctx, "session-1")
			if e != nil {
				t.Fatal(e)
			}
			session.TaskEnvironmentID = "changed-environment"
			if e := repo.UpdateTaskSession(ctx, session); e != nil {
				t.Fatal(e)
			}
		}
	}
	if scenario == "changed-environment" {
		observedCloner.after = func() {
			environment, e := repo.GetTaskEnvironmentByTaskID(ctx, "task-1")
			if e != nil {
				t.Fatal(e)
			}
			environment.WorkspacePath = canonicalTempDir(t)
			if e := repo.UpdateTaskEnvironment(ctx, environment); e != nil {
				t.Fatal(e)
			}
		}
	}
	if scenario == "repository-changed" || scenario == "repository-changed-missing" {
		observedCloner.after = func() {
			scope, id := "changed-provider-scope", "changed-project"
			if _, e := svc.UpdateRepository(ctx, "probe-added", &taskservice.UpdateRepositoryRequest{ProviderScope: &scope, ProviderRepoID: &id}); e != nil {
				t.Fatal(e)
			}
		}
	}
	updater := &managedAttachmentUpdater{repoLocalPathUpdater: repoLocalPathUpdater{svc: svc}, fail: scenario == "persistence" || scenario == "persistence-retry"}
	exc.SetRepoCloner(observedCloner, updater)
	mat := newWorkspaceSourceMaterializer(repo, newPersistingMaterializerWorktreeMgr(t, root, repo), nil, log)
	rescan := &managedAttachmentRescan{fail: scenario == "late-failure"}
	mat.rescanner = rescan
	mat.SetHostRepositoryCloner(exc)
	svc.SetWorkspaceSourceMaterializer(mat)
	svc.SetBranchMaterializer(mat.branches)
	branchRescan := &stubRescanner{}
	if route == "mcp-branch" {
		mat.branches.rescanner = branchRescan
	}
	if scenario == "cloner-unavailable" {
		mat.SetHostRepositoryCloner(nil)
	}
	if scenario == "active" {
		if _, err := svc.StartTurn(ctx, "session-1"); err != nil {
			t.Fatal(err)
		}
		_, err := svc.AttachWorkspaceSources(ctx, taskservice.AttachWorkspaceSourcesRequest{TaskID: "task-1", Sources: []taskservice.WorkspaceSourceInput{{Kind: taskservice.WorkspaceSourceRepository, RepositoryID: "probe-added", BaseBranch: "main"}}})
		if !errors.Is(err, taskservice.ErrWorkspaceSourceActive) {
			t.Fatalf("active batch control=%v", err)
		}
	}
	if scenario == "inherited" {
		seedManagedAttachmentInheritance(t, ctx, repo)
	}
	sources := []any{map[string]any{"kind": "repository", "repository_id": "probe-added", "base_branch": "main"}}
	if scenario == "late-failure" {
		sources = append(sources, map[string]any{"kind": "folder", "local_path": canonicalTempDir(t), "display_name": "notes"})
	}
	payload := map[string]any{"task_id": "task-1", "repository_id": "probe-added", "base_branch": "main"}
	success := false
	response := ""
	attach := func() {
		if strings.HasPrefix(route, "http-") {
			router := gin.New()
			taskh.RegisterTaskRoutes(router, ws.NewDispatcher(), svc, nil, repo, nil, log)
			body, e := json.Marshal(map[string]any{"sources": sources})
			if e != nil {
				t.Fatal(e)
			}
			req := httptest.NewRequest("POST", "/api/v1/tasks/task-1/workspace-sources", strings.NewReader(string(body))).WithContext(ctx)
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)
			response = recorder.Body.String()
			success = recorder.Code == 200
			return
		}
		{
			dispatcher := ws.NewDispatcher()
			mcph.NewHandlers(svc, nil, nil, nil, nil, repo, repo, eventBus, nil, nil, nil, nil, log).RegisterHandlers(dispatcher)
			action := ws.ActionMCPAddBranchToTask
			if route == "mcp-source" || route == "mcp-local-source" {
				action = ws.ActionMCPAddWorkspaceSources
				payload = map[string]any{"task_id": "task-1", "caller_task_id": "task-1", "caller_session_id": "session-1", "sources": sources}
			}
			msg, err := ws.NewRequest("probe", action, payload)
			if err != nil {
				t.Fatal(err)
			}
			result, err := dispatcher.Dispatch(ctx, msg)
			if err != nil {
				t.Fatal(err)
			}
			response = string(result.Payload)
			success = result.Type == ws.MessageTypeResponse
		}
	}
	attach()
	if scenario == "persistence-retry" {
		if success || updater.calls != 1 {
			t.Fatalf("first persistence failure: success=%v calls=%d", success, updater.calls)
		}
		rows, e := repo.ListTaskRepositories(ctx, "task-1")
		if e != nil || len(rows) != 1 {
			t.Fatalf("failed branch retained attachment: %v %v", rows, e)
		}
		updater.fail = false
		attach()
		if updater.calls != 2 {
			t.Fatalf("retry path persistence calls=%d", updater.calls)
		}
	}
	verifyLateFailure := func() {
		if success {
			t.Fatal("late failure succeeded")
		}
		rows, e := repo.ListTaskRepositories(ctx, "task-1")
		if e != nil || len(rows) != 1 {
			t.Fatalf("late failure attachments: %v %v", rows, e)
		}
		stored, e := repo.GetRepository(ctx, "probe-added")
		if e != nil || stored.LocalPath != managed {
			t.Fatalf("verified shared clone not retained: %v %v", stored, e)
		}
		if _, e := os.ReadFile(filepath.Join(managed, "README.md")); e != nil {
			t.Fatal(e)
		}
		folders, e := repo.ListTaskWorkspaceFolders(ctx, "task-1")
		if e != nil || len(folders) != 0 {
			t.Fatalf("failed mixed batch folders=%v err=%v", folders, e)
		}
		inventory, e := repo.ListTaskEnvironmentRepos(ctx, "env-1")
		if e != nil {
			t.Fatal(e)
		}
		active := 0
		for _, row := range inventory {
			if row.DeletedAt == nil && row.Status != "deleted" {
				active++
			}
		}
		if active != 1 {
			t.Fatalf("failed mixed batch active inventory=%d", active)
		}
		worktrees, e := mat.worktreeMgr.GetAllByTaskID(ctx, "task-1")
		if e != nil {
			t.Fatal(e)
		}
		for _, wt := range worktrees {
			if wt.RepositoryID == "probe-added" {
				t.Fatal("failed mixed batch kept active worktree")
			}
		}

	}
	if scenario == "late-failure" {
		verifyLateFailure()
		rescan.fail = false
		attach()
	}
	rows, err := repo.ListTaskRepositories(ctx, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := repo.ListTaskEnvironmentRepos(ctx, "env-1")
	if err != nil {
		t.Fatal(err)
	}
	stored, err := repo.GetRepository(ctx, "probe-added")
	if err != nil {
		t.Fatal(err)
	}
	var contentPath string
	for _, r := range inventory {
		if r.RepositoryID == "probe-added" {
			contentPath = filepath.Join(r.WorktreePath, "README.md")
		}
	}
	if strings.Contains(route, "local-source") {
		e, err := repo.GetTaskEnvironmentByTaskID(ctx, "task-1")
		if err != nil {
			t.Fatal(err)
		}
		contentPath = filepath.Join(e.WorkspacePath, "added", "README.md")
	}
	readable := false
	if contentPath != "" {
		_, err = os.ReadFile(contentPath)
		readable = err == nil
	}
	for _, r := range observedCloner.requests {
		if r.TaskID != "task-1" || r.SessionID != "session-1" || r.RepositoryID != "probe-added" || r.WorkspaceID != "ws-1" {
			t.Fatalf("credential request scope=%+v", r)
		}
	}
	switch scenario {
	case "empty", "missing", "persistence", "late-failure", "pathless-legacy", "rebound-session", "changed-environment", "active", "inherited", "persistence-retry", "scoped-missing", "repository-changed", "repository-changed-missing", "alias-empty", "alias-missing", "space-missing", "newline-missing":
		if len(observedCloner.requests) != 1 {
			t.Fatalf("scoped clone requests=%d, want one", len(observedCloner.requests))
		}
	case "valid", "local-valid", "local-missing", "foreign", "legacy-valid", "legacy-missing", "cloner-unavailable", "scoped-legacy", "alias-valid", "space-valid", "newline-valid":
		if len(observedCloner.requests) != 0 {
			t.Fatalf("unexpected clone requests=%d", len(observedCloner.requests))
		}
	}
	if scenario == "scoped-missing" && (observedCloner.requests[0].ProviderScope != entity.ProviderScope || observedCloner.requests[0].ProviderRepositoryID != entity.ProviderRepoID) {
		t.Fatalf("scoped provider identity lost: %+v", observedCloner.requests[0])
	}
	t.Logf("clone_requests=%d", len(observedCloner.requests))
	t.Logf("success=%v attachments=%d inventory=%d saved_changed=%v readable=%v response=%s", success, len(rows), len(inventory), stored.LocalPath != saved, readable, response)
	verifyFailure := func() {
		if scenario == "local-missing" || scenario == "foreign" || scenario == "legacy-missing" || scenario == "scoped-legacy" {
			if _, err := os.Lstat(managed); !os.IsNotExist(err) {
				t.Fatalf("refused source created managed destination: %v", err)
			}
		}
		if success || len(rows) != 1 || len(inventory) != 1 || (scenario != "rebound-session" && scenario != "changed-environment" && stored.LocalPath != saved) {
			t.Error("failed attachment did not preserve prior state")
		}
		if scenario == "persistence" && updater.calls != 1 {
			t.Fatalf("verified persistence attempts=%d", updater.calls)
		}
		if scenario == "legacy-missing" && !strings.Contains(response, "POST /api/v1/workspaces/") {
			t.Fatalf("missing actionable re-import route: %s", response)
		}
		if (scenario == "foreign" || scenario == "scoped-legacy") && !reflect.DeepEqual(foreignSnapshot, managedAttachmentSnapshot(t, saved)) {
			t.Fatal("foreign repository tree mutated")
		}
		if scenario == "foreign" {
			b, e := os.ReadFile(filepath.Join(saved, "sentinel"))
			if e != nil || string(b) != "preserve" {
				t.Fatal("foreign sentinel altered")
			}
		}
	}
	if scenario == "scoped-legacy" || scenario == "local-missing" || scenario == "foreign" || scenario == "clone-failure" || scenario == "legacy-missing" || scenario == "persistence" || scenario == "rebound-session" || scenario == "changed-environment" || scenario == "cloner-unavailable" || scenario == "repository-changed" || scenario == "repository-changed-missing" {
		verifyFailure()
		return
	}
	if !success || !readable || len(rows) != 2 || len(inventory) != 2 {
		t.Errorf("eligible attachment must produce readable worktree, got %s", fmt.Sprint(success, readable, len(rows), len(inventory)))
	}
	if scenario == "legacy-valid" && (stored.LocalPath != saved || len(observedCloner.requests) != 0) {
		t.Fatal("existing legacy source redirected or cloned")
	}
	verifyManagedAttachmentCompatibility(t, scenario, stored.LocalPath, managed, origin, contentPath)
	if route == "mcp-branch" {
		var result struct {
			WorktreePath      string `json:"worktree_path"`
			TaskWorkspacePath string `json:"task_workspace_path"`
			AgentCWDChanged   bool   `json:"agent_cwd_changed"`
		}
		if err := json.Unmarshal([]byte(response), &result); err != nil {
			t.Fatal(err)
		}
		if result.WorktreePath != filepath.Dir(contentPath) || result.TaskWorkspacePath != root || result.AgentCWDChanged || len(branchRescan.calls) != 1 || len(branchRescan.notifyCalls) != 1 || len(rescan.calls) != 0 {
			t.Fatalf("branch result=%+v rescans=%d rebinds=%d", result, len(branchRescan.calls), len(rescan.calls))
		}
	}
	for _, row := range rows {
		if row.RepositoryID == "probe-added" && row.BaseBranch != "main" {
			t.Fatalf("requested base changed: %s", row.BaseBranch)
		}
	}
	if runGit(t, filepath.Dir(contentPath), "rev-parse", "HEAD") != runGit(t, origin, "rev-parse", "main") {
		t.Fatal("attached checkout used wrong commit/base")
	}
	if scenario == "local-valid" && stored.LocalPath != origin {
		t.Error("explicit local source redirected")
	}

}

type managedAttachmentUpdater struct {
	repoLocalPathUpdater
	fail  bool
	calls int
}

func (u *managedAttachmentUpdater) UpdateRepositoryLocalPath(ctx context.Context, id, path string) error {
	u.calls++
	if u.fail {
		return errors.New("verified path persistence refused")
	}
	return u.repoLocalPathUpdater.UpdateRepositoryLocalPath(ctx, id, path)
}

type managedAttachmentRescan struct {
	workspaceSourceRescanStub
	fail bool
}

func (r *managedAttachmentRescan) RescanWorkspaceForSession(ctx context.Context, id, dir string, roots ...[]string) error {
	if r.fail {
		return errors.New("rescan refused")
	}
	return r.workspaceSourceRescanStub.RescanWorkspaceForSession(ctx, id, dir, roots...)
}

func managedAttachmentSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		result[rel] = fmt.Sprintf("%x", sha256.Sum256(data))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func seedManagedAttachmentInheritance(t *testing.T, ctx context.Context, repo *sqliterepo.Repository) {
	t.Helper()
	if err := repo.CreateTask(ctx, &models.Task{ID: "parent-owner", WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Environment owner"}); err != nil {
		t.Fatal(err)
	}
	task, err := repo.GetTask(ctx, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	task.ParentID = "parent-owner"
	task.Metadata = map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent"}}
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	env, err := repo.GetTaskEnvironmentByTaskID(ctx, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	env.TaskID = "parent-owner"
	if err := repo.UpdateTaskEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}
}

func (u *managedAttachmentUpdater) UpdateVerifiedRepositoryLocalPath(ctx context.Context, repository *models.Repository, path string) error {
	u.calls++
	if u.fail {
		return errors.New("verified path persistence refused")
	}
	return u.repoLocalPathUpdater.UpdateVerifiedRepositoryLocalPath(ctx, repository, path)
}
