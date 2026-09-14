package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	mcphandlers "github.com/kandev/kandev/internal/mcp/handlers"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeGitLabChangeLinks struct {
	mrs       []*gitlab.TaskMR
	linkedURL string
	unlinkErr map[string]error
	unlinked  []string
}

type fakeGitHubChangeLinks struct {
	prs               []*github.TaskPR
	linkedURL         string
	linkedWorkspaceID string
	linkedUserID      string
	unlinked          []string
}

func (f *fakeGitHubChangeLinks) AssociateExistingPRByURLForWorkspace(_ context.Context, workspaceID, userID, taskID, repositoryID, prURL string) (*github.TaskPR, error) {
	f.linkedURL = prURL
	f.linkedWorkspaceID = workspaceID
	f.linkedUserID = userID
	pr := &github.TaskPR{ID: "linked-github", TaskID: taskID, RepositoryID: repositoryID, PRNumber: 42, PRURL: prURL}
	f.prs = append(f.prs, pr)
	return pr, nil
}

func (f *fakeGitHubChangeLinks) DetachTaskPR(_ context.Context, _ string, associationID string) (*github.TaskPR, error) {
	f.unlinked = append(f.unlinked, associationID)
	return nil, nil
}

func (f *fakeGitHubChangeLinks) ListTaskPRs(_ context.Context, taskIDs []string) (map[string][]*github.TaskPR, error) {
	out := make(map[string][]*github.TaskPR)
	for _, taskID := range taskIDs {
		for _, pr := range f.prs {
			if pr.TaskID == taskID {
				out[taskID] = append(out[taskID], pr)
			}
		}
	}
	return out, nil
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
	provider := "gitlab"
	if strings.Contains(host, "github.com") {
		provider = "github"
	}
	if err := repos.CreateRepository(ctx, &models.Repository{
		ID: repositoryID, WorkspaceID: workspaceID, Name: name, ProviderHost: host,
		Provider: provider, ProviderOwner: owner, ProviderName: name, CreatedAt: now, UpdatedAt: now,
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
	if _, err := githubChangeURL("", "acme", "api", 7); err == nil {
		t.Fatal("githubChangeURL accepted an unknown empty host")
	}
	url, err := githubChangeURL("https://enterprise.github.com", "acme", "api", 7)
	if err != nil || url != "https://enterprise.github.com/acme/api/pull/7" {
		t.Fatalf("githubChangeURL alternate origin = %q, %v", url, err)
	}
}

func TestTaskChangeCoordinatorDispatchesGitHubLinkWithWorkspaceIdentity(t *testing.T) {
	taskSvc, repos := newTaskChangeCoordinatorHarness(t)
	seedTaskChangeCoordinatorTask(t, repos, "ws-1", "task-1", "repo-1", "https://github.com", "acme", "api")
	githubLinks := &fakeGitHubChangeLinks{}
	coordinator := taskChangeLinkCoordinator{tasks: taskSvc, github: githubLinks}
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-1"})

	links, err := coordinator.LinkTaskChange(ctx, mcphandlers.TaskChangeLinkRequest{
		TaskID: "task-1",
		Link:   mcphandlers.TaskChangeLink{Provider: "github", RepositoryID: "repo-1", Number: 42},
	})
	if err != nil {
		t.Fatalf("LinkTaskChange: %v", err)
	}
	if githubLinks.linkedWorkspaceID != "ws-1" || githubLinks.linkedUserID != "user-1" {
		t.Fatalf("GitHub identity = workspace %q user %q", githubLinks.linkedWorkspaceID, githubLinks.linkedUserID)
	}
	if githubLinks.linkedURL != "https://github.com/acme/api/pull/42" {
		t.Fatalf("linked URL = %q", githubLinks.linkedURL)
	}
	if len(links) != 1 || links[0].Provider != "github" {
		t.Fatalf("links = %#v, want one GitHub link", links)
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

func TestTaskChangeCoordinatorReplaceSameIdentityIsANoOp(t *testing.T) {
	taskSvc, repos := newTaskChangeCoordinatorHarness(t)
	seedTaskChangeCoordinatorTask(t, repos, "ws-1", "task-1", "repo-1", "https://gitlab.example.test", "group", "project")
	links := &fakeGitLabChangeLinks{mrs: []*gitlab.TaskMR{{ID: "current", TaskID: "task-1", RepositoryID: "repo-1", MRIID: 42}}}
	coordinator := taskChangeLinkCoordinator{tasks: taskSvc, gitlab: links}
	identity := mcphandlers.TaskChangeLink{Provider: "gitlab", RepositoryID: "repo-1", Number: 42}

	got, err := coordinator.ReplaceTaskChange(context.Background(), mcphandlers.TaskChangeLinkRequest{TaskID: "task-1", Link: identity, Old: &identity})
	if err != nil || len(got) != 1 || got[0] != identity {
		t.Fatalf("ReplaceTaskChange() = %#v, %v", got, err)
	}
	if len(links.unlinked) != 0 || links.linkedURL != "" {
		t.Fatalf("same-identity replace mutated provider: unlinked=%#v linked=%q", links.unlinked, links.linkedURL)
	}
}

func TestTaskChangeCoordinatorRejectsCrossProviderReplaceWithoutMutation(t *testing.T) {
	taskSvc, repos := newTaskChangeCoordinatorHarness(t)
	seedTaskChangeCoordinatorTask(t, repos, "ws-1", "task-1", "repo-1", "https://gitlab.example.test", "group", "project")
	now := time.Now().UTC()
	if err := repos.CreateRepository(context.Background(), &models.Repository{
		ID: "repo-gh", WorkspaceID: "ws-1", Name: "api", ProviderHost: "https://github.com", ProviderOwner: "acme", ProviderName: "api", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create GitHub repository: %v", err)
	}
	if err := repos.CreateTaskRepository(context.Background(), &models.TaskRepository{
		ID: "tr-task-1-repo-gh", TaskID: "task-1", RepositoryID: "repo-gh", CreatedAt: now, UpdatedAt: now, Metadata: map[string]interface{}{},
	}); err != nil {
		t.Fatalf("attach GitHub repository: %v", err)
	}
	gitlabLinks := &fakeGitLabChangeLinks{mrs: []*gitlab.TaskMR{{
		ID: "old", TaskID: "task-1", RepositoryID: "repo-1", MRIID: 7,
		MRURL: "https://gitlab.example.test/group/project/-/merge_requests/7",
	}}}
	githubLinks := &fakeGitHubChangeLinks{}
	coordinator := taskChangeLinkCoordinator{tasks: taskSvc, github: githubLinks, gitlab: gitlabLinks}

	_, err := coordinator.ReplaceTaskChange(context.Background(), mcphandlers.TaskChangeLinkRequest{
		TaskID: "task-1",
		Link:   mcphandlers.TaskChangeLink{Provider: "github", RepositoryID: "repo-gh", Number: 42},
		Old:    &mcphandlers.TaskChangeLink{Provider: "gitlab", RepositoryID: "repo-1", Number: 7},
	})
	if err == nil {
		t.Fatal("ReplaceTaskChange accepted a cross-provider replacement")
	}
	if len(gitlabLinks.mrs) != 1 || gitlabLinks.mrs[0].ID != "old" || githubLinks.linkedURL != "" || len(gitlabLinks.unlinked) != 0 {
		t.Fatalf("cross-provider replace mutated links: mrs=%#v github=%q unlinked=%#v", gitlabLinks.mrs, githubLinks.linkedURL, gitlabLinks.unlinked)
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

func TestTaskChangeCoordinatorReportsRollbackFailure(t *testing.T) {
	taskSvc, repos := newTaskChangeCoordinatorHarness(t)
	seedTaskChangeCoordinatorTask(t, repos, "ws-1", "task-1", "repo-1", "https://gitlab.example.test", "group", "project")
	gitlabLinks := &fakeGitLabChangeLinks{
		mrs: []*gitlab.TaskMR{{
			ID: "old", TaskID: "task-1", RepositoryID: "repo-1", MRIID: 7,
			MRURL: "https://gitlab.example.test/group/project/-/merge_requests/7",
		}},
		unlinkErr: map[string]error{
			"old":        errors.New("delete failed"),
			"linked-new": errors.New("rollback failed"),
		},
	}
	coordinator := taskChangeLinkCoordinator{tasks: taskSvc, gitlab: gitlabLinks}

	_, err := coordinator.ReplaceTaskChange(context.Background(), mcphandlers.TaskChangeLinkRequest{
		TaskID: "task-1",
		Link:   mcphandlers.TaskChangeLink{Provider: "gitlab", RepositoryID: "repo-1", Number: 42},
		Old:    &mcphandlers.TaskChangeLink{Provider: "gitlab", RepositoryID: "repo-1", Number: 7},
	})
	if err == nil || !strings.Contains(err.Error(), "delete failed") || !strings.Contains(err.Error(), "rollback failed") ||
		!strings.Contains(err.Error(), "active task change links") || !strings.Contains(err.Error(), "42") {
		t.Fatalf("ReplaceTaskChange error = %v, want both failures and active links", err)
	}
}

func TestTaskChangeCoordinatorUnlinksStaleAssociationWithoutCurrentRepository(t *testing.T) {
	taskSvc, repos := newTaskChangeCoordinatorHarness(t)
	seedTaskChangeCoordinatorTask(t, repos, "ws-1", "task-1", "repo-1", "https://gitlab.example.test", "group", "project")
	requireErr := repos.DeleteTaskRepository(context.Background(), "tr-task-1-repo-1")
	if requireErr != nil {
		t.Fatalf("remove current task repository: %v", requireErr)
	}
	gitlabLinks := &fakeGitLabChangeLinks{mrs: []*gitlab.TaskMR{{
		ID: "stale", TaskID: "task-1", RepositoryID: "repo-1", MRIID: 42,
	}}}
	coordinator := taskChangeLinkCoordinator{tasks: taskSvc, gitlab: gitlabLinks}

	links, err := coordinator.UnlinkTaskChange(context.Background(), mcphandlers.TaskChangeLinkRequest{
		TaskID: "task-1",
		Link:   mcphandlers.TaskChangeLink{Provider: "gitlab", RepositoryID: "repo-1", Number: 42},
	})
	if err != nil {
		t.Fatalf("UnlinkTaskChange stale association: %v", err)
	}
	if len(links) != 0 || len(gitlabLinks.unlinked) != 1 || gitlabLinks.unlinked[0] != "stale" {
		t.Fatalf("stale unlink result links=%#v unlinked=%#v", links, gitlabLinks.unlinked)
	}
}

func TestTaskChangeCoordinatorUnlinksReadResolvedLegacyGitLabAssociation(t *testing.T) {
	taskSvc, repos := newTaskChangeCoordinatorHarness(t)
	seedTaskChangeCoordinatorTask(t, repos, "ws-1", "task-1", "repo-1", "https://gitlab.example.test", "group", "project")
	gitlabLinks := &fakeGitLabChangeLinks{mrs: []*gitlab.TaskMR{{
		ID: "legacy", TaskID: "task-1", RepositoryID: "", Host: "https://gitlab.example.test",
		ProjectPath: "group/project", MRIID: 7,
	}}}
	adapter := gitlabTaskChangeRequestAdapter{tasks: taskSvc}
	task, err := taskSvc.GetTask(context.Background(), "task-1")
	require.NoError(t, err)
	readChange := adapter.gitLabTaskChangeRequest(context.Background(), task, gitlabLinks.mrs[0])
	require.NotNil(t, readChange.RepositoryID)
	assert.Equal(t, "repo-1", *readChange.RepositoryID)

	coordinator := taskChangeLinkCoordinator{tasks: taskSvc, gitlab: gitlabLinks}
	links, err := coordinator.UnlinkTaskChange(context.Background(), mcphandlers.TaskChangeLinkRequest{
		TaskID: "task-1",
		Link:   mcphandlers.TaskChangeLink{Provider: "gitlab", RepositoryID: *readChange.RepositoryID, Number: readChange.Number},
	})
	require.NoError(t, err)
	assert.Empty(t, links)
	assert.Equal(t, []string{"legacy"}, gitlabLinks.unlinked)
	assert.Empty(t, gitlabLinks.mrs)
}

func TestTaskChangeCoordinatorReplacesReadResolvedLegacyGitLabAssociation(t *testing.T) {
	taskSvc, repos := newTaskChangeCoordinatorHarness(t)
	seedTaskChangeCoordinatorTask(t, repos, "ws-1", "task-1", "repo-1", "https://gitlab.example.test", "group", "project")
	gitlabLinks := &fakeGitLabChangeLinks{mrs: []*gitlab.TaskMR{{
		ID: "legacy", TaskID: "task-1", RepositoryID: "", Host: "https://gitlab.example.test",
		ProjectPath: "group/project", MRIID: 7,
	}}}
	adapter := gitlabTaskChangeRequestAdapter{tasks: taskSvc}
	task, err := taskSvc.GetTask(context.Background(), "task-1")
	require.NoError(t, err)
	readChange := adapter.gitLabTaskChangeRequest(context.Background(), task, gitlabLinks.mrs[0])
	require.NotNil(t, readChange.RepositoryID)

	coordinator := taskChangeLinkCoordinator{tasks: taskSvc, gitlab: gitlabLinks}
	result, err := coordinator.ManageTaskChangeRequest(context.Background(), mcphandlers.TaskChangeLinkRequest{
		Operation: "replace",
		TaskID:    "task-1",
		Link:      mcphandlers.TaskChangeLink{Provider: "gitlab", RepositoryID: *readChange.RepositoryID, Number: 42},
		Old:       &mcphandlers.TaskChangeLink{Provider: "gitlab", RepositoryID: *readChange.RepositoryID, Number: readChange.Number},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"legacy"}, gitlabLinks.unlinked)
	assert.True(t, result.StateKnown)
	assert.Equal(t, []mcphandlers.TaskChangeLink{{Provider: "gitlab", RepositoryID: "repo-1", Number: 42}}, result.Links)
}

func TestTaskChangeManagementDispatchesThroughHandlerAndCoordinator(t *testing.T) {
	taskSvc, repos := newTaskChangeCoordinatorHarness(t)
	seedTaskChangeCoordinatorTask(t, repos, "ws-1", "task-1", "repo-1", "https://gitlab.example.test", "group", "project")
	gitlabLinks := &fakeGitLabChangeLinks{mrs: []*gitlab.TaskMR{{
		ID: "legacy", TaskID: "task-1", RepositoryID: "", Host: "https://gitlab.example.test",
		ProjectPath: "group/project", MRIID: 7,
	}}}
	response := dispatchTaskChangeManagement(t, taskSvc, repos, taskChangeLinkCoordinator{tasks: taskSvc, gitlab: gitlabLinks}, map[string]interface{}{
		"operation": "replace", "task_id": "task-1", "caller_task_id": "task-caller",
		"provider": "gitlab", "repository_id": "repo-1", "number": 42,
		"old_provider": "gitlab", "old_repository_id": "repo-1", "old_number": 7,
	})

	require.Equal(t, ws.MessageTypeResponse, response.Type)
	assert.Equal(t, []string{"legacy"}, gitlabLinks.unlinked)
	assert.Len(t, gitlabLinks.mrs, 1)
	assert.Equal(t, "linked-new", gitlabLinks.mrs[0].ID)
	var result mcphandlers.TaskChangeLinkMutationResult
	require.NoError(t, json.Unmarshal(response.Payload, &result))
	assert.Equal(t, []mcphandlers.TaskChangeLink{{Provider: "gitlab", RepositoryID: "repo-1", Number: 42}}, result.Links)
	assert.True(t, result.StateKnown)
}

func TestTaskChangeManagementDispatchesFailureCompensationStateThroughHandler(t *testing.T) {
	taskSvc, repos := newTaskChangeCoordinatorHarness(t)
	seedTaskChangeCoordinatorTask(t, repos, "ws-1", "task-1", "repo-1", "https://gitlab.example.test", "group", "project")
	gitlabLinks := &fakeGitLabChangeLinks{
		mrs: []*gitlab.TaskMR{{
			ID: "legacy", TaskID: "task-1", RepositoryID: "", Host: "https://gitlab.example.test",
			ProjectPath: "group/project", MRIID: 7,
		}},
		unlinkErr: map[string]error{"legacy": errors.New("old unlink failed"), "linked-new": errors.New("rollback failed")},
	}
	response := dispatchTaskChangeManagement(t, taskSvc, repos, taskChangeLinkCoordinator{tasks: taskSvc, gitlab: gitlabLinks}, map[string]interface{}{
		"operation": "replace", "task_id": "task-1", "caller_task_id": "task-caller",
		"provider": "gitlab", "repository_id": "repo-1", "number": 42,
		"old_provider": "gitlab", "old_repository_id": "repo-1", "old_number": 7,
	})

	require.Equal(t, ws.MessageTypeError, response.Type)
	var errorPayload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(response.Payload, &errorPayload))
	assert.Equal(t, "old unlink failed", errorPayload.Details["operation_error"])
	assert.Equal(t, "rollback failed", errorPayload.Details["rollback_error"])
	assert.Equal(t, true, errorPayload.Details["state_known"])
	assert.Len(t, errorPayload.Details["links"], 2)
}

func dispatchTaskChangeManagement(
	t *testing.T,
	taskSvc *taskservice.Service,
	taskRepo *sqliterepo.Repository,
	coordinator mcphandlers.TaskChangeLinkService,
	payload map[string]interface{},
) *ws.Message {
	t.Helper()
	log := newTestLogger()
	h := mcphandlers.NewHandlers(taskSvc, nil, nil, nil, nil, taskRepo, taskRepo, nil, nil, nil, nil, nil, log)
	h.SetTaskChangeLinkService(coordinator)
	dispatcher := ws.NewDispatcher()
	h.RegisterHandlers(dispatcher)
	ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		WorkspaceID: "ws-1", CallerTaskID: "task-caller", CallerSessionID: "session-1",
		Surface: mcpprofile.SurfaceKanbanTask,
	})
	msg, err := ws.NewRequest("management-test", ws.ActionMCPManageTaskChangeRequest, payload)
	require.NoError(t, err)
	response, err := dispatcher.Dispatch(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, response)
	return response
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
