package github

import (
	"context"
	"errors"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type fakeTaskIssueStore struct {
	task                 *taskmodels.Task
	repos                []*taskmodels.TaskRepository
	entities             map[string]*taskmodels.Repository
	repositoryErrs       map[string]error
	failOnCanceledUpdate bool
	// taskErr takes priority over the task-not-set fallback. Set it to an
	// error wrapping ErrTaskNotFound when a test needs the controller's 404 path.
	taskErr   error
	updateErr error
	updated   map[string]interface{}
}

type countingIssueClient struct {
	*MockClient
	getIssueCalls int
}

func (c *countingIssueClient) GetIssue(ctx context.Context, owner, repo string, number int) (*Issue, error) {
	c.getIssueCalls++
	return c.MockClient.GetIssue(ctx, owner, repo, number)
}

func (f *fakeTaskIssueStore) GetTask(_ context.Context, taskID string) (*taskmodels.Task, error) {
	if f.taskErr != nil {
		return nil, f.taskErr
	}
	if f.task == nil || f.task.ID != taskID {
		return nil, errors.New("task not found")
	}
	return f.task, nil
}

func (f *fakeTaskIssueStore) ListTaskRepositories(_ context.Context, taskID string) ([]*taskmodels.TaskRepository, error) {
	if f.taskErr != nil {
		return nil, f.taskErr
	}
	if f.task == nil || f.task.ID != taskID {
		return nil, errors.New("task not found")
	}
	return f.repos, nil
}

func (f *fakeTaskIssueStore) GetRepository(_ context.Context, repositoryID string) (*taskmodels.Repository, error) {
	if err := f.repositoryErrs[repositoryID]; err != nil {
		return nil, err
	}
	if repo := f.entities[repositoryID]; repo != nil {
		return repo, nil
	}
	return nil, errors.New("repository not found")
}

func (f *fakeTaskIssueStore) UpdateTaskMetadata(ctx context.Context, taskID string, metadata map[string]interface{}) (*taskmodels.Task, error) {
	if f.taskErr != nil {
		return nil, f.taskErr
	}
	if f.task == nil || f.task.ID != taskID {
		return nil, errors.New("task not found")
	}
	if f.failOnCanceledUpdate && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	f.updated = metadata
	f.task.Metadata = metadata
	return f.task, nil
}

func TestLinkTaskIssue_MergesIssueMetadataAndPreservesState(t *testing.T) {
	client := NewMockClient()
	client.AddIssue(&Issue{
		Number:    1470,
		Title:     "Link existing task",
		HTMLURL:   "https://github.com/kdlbs/kandev/issues/1470",
		RepoOwner: "kdlbs",
		RepoName:  "kandev",
		State:     "open",
	})
	store := &fakeTaskIssueStore{
		failOnCanceledUpdate: true,
		task: &taskmodels.Task{
			ID:       "task-1",
			State:    v1.TaskStateInProgress,
			Metadata: map[string]interface{}{"keep": "me"},
		},
		repos: []*taskmodels.TaskRepository{{RepositoryID: "repo-1"}},
		entities: map[string]*taskmodels.Repository{
			"repo-1": {ID: "repo-1", Provider: "github", ProviderOwner: "kdlbs", ProviderName: "kandev"},
		},
	}
	svc := NewService(client, AuthMethodPAT, nil, nil, nil, testLogger(t))
	svc.SetTaskIssueStore(store)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resp, err := svc.LinkTaskIssue(ctx, "task-1", LinkTaskIssueRequest{
		Issue: "https://github.com/kdlbs/kandev/issues/1470",
	})
	if err != nil {
		t.Fatalf("LinkTaskIssue: %v", err)
	}

	if resp.IssueNumber != 1470 || resp.IssueURL == "" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if store.task.State != v1.TaskStateInProgress {
		t.Fatalf("state changed: %s", store.task.State)
	}
	if store.updated["keep"] != "me" {
		t.Fatalf("existing metadata was not preserved: %+v", store.updated)
	}
	if store.updated[taskMetaIssueURL] != "https://github.com/kdlbs/kandev/issues/1470" {
		t.Fatalf("issue url not written: %+v", store.updated)
	}
	if store.updated[taskMetaIssueNumber] != 1470 {
		t.Fatalf("issue number not written: %+v", store.updated)
	}
}

func TestLinkTaskIssue_PropagatesRepositoryLookupError(t *testing.T) {
	client := NewMockClient()
	client.AddIssue(&Issue{
		Number:    1470,
		Title:     "Link existing task",
		HTMLURL:   "https://github.com/kdlbs/kandev/issues/1470",
		RepoOwner: "kdlbs",
		RepoName:  "kandev",
	})
	repoErr := errors.New("repository store unavailable")
	store := &fakeTaskIssueStore{
		task:           &taskmodels.Task{ID: "task-1", Metadata: map[string]interface{}{}},
		repos:          []*taskmodels.TaskRepository{{RepositoryID: "repo-1"}},
		repositoryErrs: map[string]error{"repo-1": repoErr},
	}
	svc := NewService(client, AuthMethodPAT, nil, nil, nil, testLogger(t))
	svc.SetTaskIssueStore(store)

	_, err := svc.LinkTaskIssue(context.Background(), "task-1", LinkTaskIssueRequest{
		Issue: "https://github.com/kdlbs/kandev/issues/1470",
	})
	if !errors.Is(err, repoErr) {
		t.Fatalf("err = %v, want repository lookup error", err)
	}
	if errors.Is(err, ErrIssueRepositoryMismatch) {
		t.Fatalf("repository lookup error should not be masked as mismatch: %v", err)
	}
	if store.updated != nil {
		t.Fatalf("metadata should not be updated: %+v", store.updated)
	}
}

func TestLinkTaskIssue_ChecksTaskBeforeFetchingIssue(t *testing.T) {
	client := &countingIssueClient{MockClient: NewMockClient()}
	taskErr := errors.New("task lookup failed")
	store := &fakeTaskIssueStore{taskErr: taskErr}
	svc := NewService(client, AuthMethodPAT, nil, nil, nil, testLogger(t))
	svc.SetTaskIssueStore(store)

	_, err := svc.LinkTaskIssue(context.Background(), "missing-task", LinkTaskIssueRequest{
		Issue: "https://github.com/kdlbs/kandev/issues/1470",
	})
	if !errors.Is(err, taskErr) {
		t.Fatalf("expected task lookup error before GitHub fetch, got %v", err)
	}
	if client.getIssueCalls != 0 {
		t.Fatalf("GetIssue called %d times before task lookup succeeded", client.getIssueCalls)
	}
}

func TestLinkTaskIssue_RejectsIssueFromDifferentTaskRepository(t *testing.T) {
	client := NewMockClient()
	client.AddIssue(&Issue{
		Number:    1,
		Title:     "Wrong repo",
		HTMLURL:   "https://github.com/other/repo/issues/1",
		RepoOwner: "other",
		RepoName:  "repo",
	})
	store := &fakeTaskIssueStore{
		task:  &taskmodels.Task{ID: "task-1", Metadata: map[string]interface{}{}},
		repos: []*taskmodels.TaskRepository{{RepositoryID: "repo-1"}},
		entities: map[string]*taskmodels.Repository{
			"repo-1": {ID: "repo-1", Provider: "github", ProviderOwner: "kdlbs", ProviderName: "kandev"},
		},
	}
	svc := NewService(client, AuthMethodPAT, nil, nil, nil, testLogger(t))
	svc.SetTaskIssueStore(store)

	_, err := svc.LinkTaskIssue(context.Background(), "task-1", LinkTaskIssueRequest{
		Issue: "https://github.com/other/repo/issues/1",
	})
	if !errors.Is(err, ErrIssueRepositoryMismatch) {
		t.Fatalf("err = %v, want ErrIssueRepositoryMismatch", err)
	}
	if store.updated != nil {
		t.Fatalf("metadata should not be updated: %+v", store.updated)
	}
}

func TestUnlinkTaskIssue_RemovesIssueMetadataOnly(t *testing.T) {
	store := &fakeTaskIssueStore{
		failOnCanceledUpdate: true,
		task: &taskmodels.Task{ID: "task-1", Metadata: map[string]interface{}{
			taskMetaIssueURL:    "https://github.com/kdlbs/kandev/issues/1470",
			taskMetaIssueNumber: 1470,
			taskMetaIssueOwner:  "kdlbs",
			taskMetaIssueRepo:   "kandev",
			taskMetaIssueLinked: true,
			"issue_watch_id":    "watch-1",
			"keep":              "me",
		}},
	}
	svc := NewService(NewMockClient(), AuthMethodPAT, nil, nil, nil, testLogger(t))
	svc.SetTaskIssueStore(store)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := svc.UnlinkTaskIssue(ctx, "task-1"); err != nil {
		t.Fatalf("UnlinkTaskIssue: %v", err)
	}
	if _, ok := store.updated[taskMetaIssueURL]; ok {
		t.Fatalf("issue url should be removed: %+v", store.updated)
	}
	if store.updated["issue_watch_id"] != "watch-1" {
		t.Fatalf("watch metadata should be preserved: %+v", store.updated)
	}
	if store.updated["keep"] != "me" {
		t.Fatalf("unrelated metadata should be preserved: %+v", store.updated)
	}
}

func TestParseIssueReference(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		input        string
		defaultOwner string
		defaultRepo  string
		wantOwner    string
		wantRepo     string
		wantNumber   int
		wantErr      bool
	}{
		{name: "empty", input: "  ", wantErr: true},
		{name: "hash number with default repo", input: "#1470", defaultOwner: "kdlbs", defaultRepo: "kandev", wantOwner: "kdlbs", wantRepo: "kandev", wantNumber: 1470},
		{name: "bare number with default repo", input: "1470", defaultOwner: "kdlbs", defaultRepo: "kandev", wantOwner: "kdlbs", wantRepo: "kandev", wantNumber: 1470},
		{name: "number without default repo", input: "#1470", defaultOwner: "kdlbs", wantErr: true},
		{name: "valid github url", input: "https://github.com/kdlbs/kandev/issues/1470", wantOwner: "kdlbs", wantRepo: "kandev", wantNumber: 1470},
		{name: "www github url", input: "https://www.github.com/kdlbs/kandev/issues/1470?foo=bar", wantOwner: "kdlbs", wantRepo: "kandev", wantNumber: 1470},
		{name: "non github host", input: "https://gitlab.com/kdlbs/kandev/issues/1470", wantErr: true},
		{name: "pull request url", input: "https://github.com/kdlbs/kandev/pull/1470", wantErr: true},
		{name: "missing issue number", input: "https://github.com/kdlbs/kandev/issues", wantErr: true},
		{name: "invalid issue number", input: "https://github.com/kdlbs/kandev/issues/nope", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, number, err := parseIssueReference(tt.input, tt.defaultOwner, tt.defaultRepo)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidIssueReference) {
					t.Fatalf("err = %v, want ErrInvalidIssueReference", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseIssueReference: %v", err)
			}
			if owner != tt.wantOwner || repo != tt.wantRepo || number != tt.wantNumber {
				t.Fatalf("got %s/%s#%d, want %s/%s#%d", owner, repo, number, tt.wantOwner, tt.wantRepo, tt.wantNumber)
			}
		})
	}
}
