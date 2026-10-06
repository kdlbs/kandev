package backendapp

import (
	"context"
	"errors"
	"fmt"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

type issueMutationBus struct {
	bus.EventBus
	mu        sync.Mutex
	published []*bus.Event
}

func (b *issueMutationBus) Publish(ctx context.Context, subject string, event *bus.Event) error {
	b.mu.Lock()
	b.published = append(b.published, event)
	b.mu.Unlock()
	return b.EventBus.Publish(ctx, subject, event)
}
func (b *issueMutationBus) snapshot() []*bus.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]*bus.Event(nil), b.published...)
}

type issueMutationHarness struct {
	services  [2]*taskservice.Service
	repos     [2]*sqliterepo.Repository
	buses     [2]*issueMutationBus
	repoLists [2]*issueMutationRepoList
}

func newIssueMutationHarness(t *testing.T, wrap func(int, *sqliterepo.Repository) repository.TaskRepository) *issueMutationHarness {
	t.Helper()
	h := &issueMutationHarness{}
	path := filepath.Join(t.TempDir(), "issue-mutations.db")
	for i := range 2 {
		raw, err := db.OpenSQLite(path)
		require.NoError(t, err)
		database := sqlx.NewDb(raw, "sqlite3")
		t.Cleanup(func() { require.NoError(t, database.Close()) })
		var repo *sqliterepo.Repository
		if i == 0 {
			repo, err = sqliterepo.NewWithDB(database, database, nil)
			require.NoError(t, err)
		} else {
			repo = sqliterepo.NewWithInitializedDB(database, database, nil)
		}
		h.repos[i] = repo
		log := newTestLogger()
		eb := bus.NewMemoryEventBus(log)
		t.Cleanup(eb.Close)
		h.buses[i] = &issueMutationBus{EventBus: eb}
		var tasks repository.TaskRepository = repo
		if wrap != nil {
			tasks = wrap(i, repo)
		}
		h.repoLists[i] = &issueMutationRepoList{TaskRepoRepository: repo}
		h.services[i] = taskservice.NewService(taskservice.Repos{Workspaces: repo, Tasks: tasks, TaskRepos: h.repoLists[i], Workflows: repo, Messages: repo, Turns: repo, Sessions: repo, GitSnapshots: repo, RepoEntities: repo, Executors: repo, Environments: repo, Reviews: repo}, h.buses[i], log, taskservice.RepositoryDiscoveryConfig{})
	}
	seedTaskChangeCoordinatorTask(t, h.repos[0], "issue-ws", "issue-task", "issue-repo", "https://github.com", "acme", "api")
	return h
}

type issueMutationReadGate struct {
	githubTaskIssueStoreAdapter
	armed   atomic.Bool
	arrived chan struct{}
	release chan struct{}
}

func (g *issueMutationReadGate) GetTask(ctx context.Context, id string) (*models.Task, error) {
	task, err := g.githubTaskIssueStoreAdapter.GetTask(ctx, id)
	if err != nil || !g.armed.Swap(false) {
		return task, err
	}
	close(g.arrived)
	select {
	case <-g.release:
		return task, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func issueMutationGitHub(t *testing.T, svc *taskservice.Service) (*github.Service, *issueMutationReadGate) {
	t.Helper()
	client := github.NewMockClient()
	for _, number := range []int{7, 42, 99} {
		owner, repo := issueMutationDomain(number)
		client.AddIssue(&github.Issue{Number: number, Title: "Issue", HTMLURL: issueMutationURL(number), RepoOwner: owner, RepoName: repo, State: "open"})
	}
	gh := github.NewService(client, github.AuthMethodPAT, nil, nil, nil, newTestLogger())
	t.Cleanup(gh.Stop)
	gate := &issueMutationReadGate{githubTaskIssueStoreAdapter: githubTaskIssueStoreAdapter{svc: svc}, arrived: make(chan struct{}), release: make(chan struct{})}
	gh.SetTaskIssueStore(gate)
	return gh, gate
}
func issueMutationURL(number int) string {
	owner, repo := issueMutationDomain(number)
	return fmt.Sprintf("https://github.com/%s/%s/issues/%d", owner, repo, number)
}
func issueMutationMetadata(number int) map[string]interface{} {
	return map[string]interface{}{"keep": "untouched", "alpha": "before", models.MetaKeyPortForwardingEnabled: false, "issue_url": issueMutationURL(number), "issue_number": number, "issue_owner": "acme", "issue_repo": "api", "github_issue_linked": true}
}
func assertIssueMutationIdentity(t *testing.T, metadata map[string]interface{}, number int) {
	t.Helper()
	keys := []string{"issue_url", "issue_number", "issue_owner", "issue_repo", "github_issue_linked"}
	if number == 0 {
		for _, key := range keys {
			require.NotContains(t, metadata, key)
		}
		return
	}
	require.Equal(t, issueMutationURL(number), metadata[keys[0]])
	require.Equal(t, float64(number), metadata[keys[1]])
	owner, repo := issueMutationDomain(number)
	require.Equal(t, owner, metadata[keys[2]])
	require.Equal(t, repo, metadata[keys[3]])
	require.Equal(t, true, metadata[keys[4]])
}
func runIssueMutation(ctx context.Context, gh *github.Service, number int) error {
	if number == 0 {
		return gh.UnlinkTaskIssue(ctx, "issue-task")
	}
	owner, repo := issueMutationDomain(number)
	_, err := gh.LinkTaskIssue(ctx, "issue-task", github.LinkTaskIssueRequest{Owner: owner, Repo: repo, Number: number})
	return err
}

func issueMutationDomain(number int) (string, string) {
	if number == 99 {
		return "other", "tools"
	}
	return "acme", "api"
}

type issueMutationRepoList struct {
	repository.TaskRepoRepository
	fail atomic.Bool
}

func (r *issueMutationRepoList) ListTaskRepositories(ctx context.Context, id string) ([]*models.TaskRepository, error) {
	if r.fail.Load() {
		return nil, errors.New("repository observation unavailable")
	}
	return r.TaskRepoRepository.ListTaskRepositories(ctx, id)
}
