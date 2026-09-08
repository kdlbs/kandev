package backendapp

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/gitlab"
	mcphandlers "github.com/kandev/kandev/internal/mcp/handlers"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type fakeGitLabChangeLinks struct {
	mrs       []*gitlab.TaskMR
	linkedURL string
	unlinkErr map[string]error
	unlinked  []string
}

func (f *fakeGitLabChangeLinks) AssociateExistingMRByURL(
	_ context.Context, _ string, taskID string, repositoryID string, mrURL string,
) (*gitlab.TaskMR, error) {
	f.linkedURL = mrURL
	for _, mr := range f.mrs {
		if mr.TaskID == taskID && mr.RepositoryID == repositoryID && mr.MRURL == mrURL {
			return mr, nil
		}
	}
	mr := &gitlab.TaskMR{
		ID: "linked-new", TaskID: taskID, RepositoryID: repositoryID,
		Host: "https://gitlab.example.test", ProjectPath: "group/project", MRIID: 42,
		MRURL: mrURL, State: "open",
	}
	f.mrs = append(f.mrs, mr)
	return mr, nil
}

func (f *fakeGitLabChangeLinks) ListTaskMRsByTask(_ context.Context, taskID string) ([]*gitlab.TaskMR, error) {
	out := make([]*gitlab.TaskMR, 0, len(f.mrs))
	for _, mr := range f.mrs {
		if mr.TaskID == taskID {
			out = append(out, mr)
		}
	}
	return out, nil
}

func (f *fakeGitLabChangeLinks) UnlinkTaskMR(_ context.Context, _ string, associationID string) error {
	f.unlinked = append(f.unlinked, associationID)
	if err := f.unlinkErr[associationID]; err != nil {
		return err
	}
	filtered := f.mrs[:0]
	for _, mr := range f.mrs {
		if mr.ID != associationID {
			filtered = append(filtered, mr)
		}
	}
	f.mrs = filtered
	return nil
}

func newTaskChangeCoordinatorHarness(t *testing.T) (*taskservice.Service, *sqliterepo.Repository) {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "task-change-links.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	database := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	repos, cleanup, err := repository.Provide(database, database, nil)
	if err != nil {
		t.Fatalf("task repository: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	if _, err := worktree.NewSQLiteStore(database, database); err != nil {
		t.Fatalf("worktree store: %v", err)
	}
	log := newTestLogger()
	taskSvc := taskservice.NewService(taskservice.Repos{
		Workspaces:   repos,
		Tasks:        repos,
		TaskRepos:    repos,
		Workflows:    repos,
		Messages:     repos,
		Turns:        repos,
		Sessions:     repos,
		GitSnapshots: repos,
		RepoEntities: repos,
		Executors:    repos,
		Environments: repos,
		Reviews:      repos,
	}, bus.NewMemoryEventBus(log), log, taskservice.RepositoryDiscoveryConfig{})
	return taskSvc, repos
}

func seedTaskChangeCoordinatorTask(
	t *testing.T, repos *sqliterepo.Repository, workspaceID, taskID, repositoryID, host, owner, name string,
) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	if err := repos.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: workspaceID, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repos.CreateRepository(ctx, &models.Repository{
		ID: repositoryID, WorkspaceID: workspaceID, Name: name, ProviderHost: host,
		ProviderOwner: owner, ProviderName: name, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if err := repos.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: taskID, State: v1.TaskStateTODO, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repos.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "tr-" + taskID + "-" + repositoryID, TaskID: taskID, RepositoryID: repositoryID,
		CreatedAt: now, UpdatedAt: now, Metadata: map[string]interface{}{},
	}); err != nil {
		t.Fatalf("create task repository: %v", err)
	}
}

func TestTaskChangeURLsRequireCanonicalRepositoryIdentity(t *testing.T) {
	githubURL, err := githubChangeURL("https://github.com", "acme", "api", 7)
	if err != nil || githubURL != "https://github.com/acme/api/pull/7" {
		t.Fatalf("githubChangeURL() = %q, %v", githubURL, err)
	}
	gitlabURL, err := gitlabChangeURL("https://gitlab.example.test/", "group", "api", 9)
	if err != nil || gitlabURL != "https://gitlab.example.test/group/api/-/merge_requests/9" {
		t.Fatalf("gitlabChangeURL() = %q, %v", gitlabURL, err)
	}
	if _, err := githubChangeURL("https://gitlab.example.test", "acme", "api", 7); err == nil {
		t.Fatal("githubChangeURL accepted a non-GitHub host")
	}
	if _, err := gitlabChangeURL("", "group", "api", 9); err == nil {
		t.Fatal("gitlabChangeURL accepted an incomplete repository identity")
	}
}

func TestGitHubChangeURLRejectsSubstringHostSpoofing(t *testing.T) {
	for _, host := range []string{
		"https://github.com.evil.test",
		"https://mygithub.com",
		"github.com.evil.test",
	} {
		if _, err := githubChangeURL(host, "acme", "api", 7); err == nil {
			t.Fatalf("githubChangeURL accepted spoofed host %q", host)
		}
	}
}

func TestTaskChangeCoordinatorDispatchesGitLabLinkAndReturnsLinkedSet(t *testing.T) {
	taskSvc, repos := newTaskChangeCoordinatorHarness(t)
	seedTaskChangeCoordinatorTask(t, repos, "ws-1", "task-1", "repo-1", "https://gitlab.example.test", "group", "project")
	gitlabLinks := &fakeGitLabChangeLinks{}
	coordinator := taskChangeLinkCoordinator{tasks: taskSvc, gitlab: gitlabLinks}

	links, err := coordinator.LinkTaskChange(context.Background(), mcphandlers.TaskChangeLinkRequest{
		TaskID: "task-1",
		Link:   mcphandlers.TaskChangeLink{Provider: "gitlab", RepositoryID: "repo-1", Number: 42},
	})
	if err != nil {
		t.Fatalf("LinkTaskChange: %v", err)
	}
	if gitlabLinks.linkedURL != "https://gitlab.example.test/group/project/-/merge_requests/42" {
		t.Fatalf("linked URL = %q", gitlabLinks.linkedURL)
	}
	if len(links) != 1 || links[0].Provider != "gitlab" || links[0].RepositoryID != "repo-1" || links[0].Number != 42 {
		t.Fatalf("links = %#v, want gitlab repo-1 !42", links)
	}
}

func TestTaskChangeCoordinatorUnlinkAbsentIsIdempotent(t *testing.T) {
	taskSvc, repos := newTaskChangeCoordinatorHarness(t)
	seedTaskChangeCoordinatorTask(t, repos, "ws-1", "task-1", "repo-1", "https://gitlab.example.test", "group", "project")
	gitlabLinks := &fakeGitLabChangeLinks{}
	coordinator := taskChangeLinkCoordinator{tasks: taskSvc, gitlab: gitlabLinks}

	links, err := coordinator.UnlinkTaskChange(context.Background(), mcphandlers.TaskChangeLinkRequest{
		TaskID: "task-1",
		Link:   mcphandlers.TaskChangeLink{Provider: "gitlab", RepositoryID: "repo-1", Number: 42},
	})
	if err != nil {
		t.Fatalf("UnlinkTaskChange absent: %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("links after absent unlink = %#v, want none", links)
	}
}

func TestTaskChangeCoordinatorReplaceRollsBackNewLinkWhenOldUnlinkFails(t *testing.T) {
	taskSvc, repos := newTaskChangeCoordinatorHarness(t)
	seedTaskChangeCoordinatorTask(t, repos, "ws-1", "task-1", "repo-1", "https://gitlab.example.test", "group", "project")
	gitlabLinks := &fakeGitLabChangeLinks{
		mrs: []*gitlab.TaskMR{{
			ID: "old", TaskID: "task-1", RepositoryID: "repo-1", MRIID: 7,
			MRURL: "https://gitlab.example.test/group/project/-/merge_requests/7",
		}},
		unlinkErr: map[string]error{"old": errors.New("delete failed")},
	}
	coordinator := taskChangeLinkCoordinator{tasks: taskSvc, gitlab: gitlabLinks}

	_, err := coordinator.ReplaceTaskChange(context.Background(), mcphandlers.TaskChangeLinkRequest{
		TaskID: "task-1",
		Link:   mcphandlers.TaskChangeLink{Provider: "gitlab", RepositoryID: "repo-1", Number: 42},
		Old:    &mcphandlers.TaskChangeLink{Provider: "gitlab", RepositoryID: "repo-1", Number: 7},
	})
	if err == nil {
		t.Fatal("ReplaceTaskChange succeeded despite old unlink failure")
	}
	if len(gitlabLinks.mrs) != 1 || gitlabLinks.mrs[0].ID != "old" {
		t.Fatalf("MRs after failed replace = %#v, want only old association", gitlabLinks.mrs)
	}
	if len(gitlabLinks.unlinked) != 2 || gitlabLinks.unlinked[0] != "old" || gitlabLinks.unlinked[1] != "linked-new" {
		t.Fatalf("unlink order = %#v, want old then rollback of new", gitlabLinks.unlinked)
	}
}

func TestTaskChangeCoordinatorRejectsTaskRepositoryFromAnotherWorkspace(t *testing.T) {
	taskSvc, repos := newTaskChangeCoordinatorHarness(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if err := repos.CreateWorkspace(ctx, &models.Workspace{ID: "ws-task", Name: "Task", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create task workspace: %v", err)
	}
	if err := repos.CreateWorkspace(ctx, &models.Workspace{ID: "ws-repo", Name: "Repo", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create repo workspace: %v", err)
	}
	if err := repos.CreateRepository(ctx, &models.Repository{
		ID: "repo-cross", WorkspaceID: "ws-repo", Name: "project",
		ProviderHost: "https://gitlab.example.test", ProviderOwner: "group", ProviderName: "project",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if err := repos.CreateTask(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-task", Title: "Task", State: v1.TaskStateTODO, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repos.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "tr-cross", TaskID: "task-1", RepositoryID: "repo-cross", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task repository: %v", err)
	}
	coordinator := taskChangeLinkCoordinator{tasks: taskSvc, gitlab: &fakeGitLabChangeLinks{}}

	_, err := coordinator.LinkTaskChange(ctx, mcphandlers.TaskChangeLinkRequest{
		TaskID: "task-1",
		Link:   mcphandlers.TaskChangeLink{Provider: "gitlab", RepositoryID: "repo-cross", Number: 1},
	})
	if err == nil {
		t.Fatal("LinkTaskChange accepted a repository from another workspace")
	}
}
