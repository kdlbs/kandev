package github

import (
	"context"
	"errors"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type fakeTaskIssueStore struct {
	task     *taskmodels.Task
	repos    []*taskmodels.TaskRepository
	entities map[string]*taskmodels.Repository
	updated  map[string]interface{}
}

func (f *fakeTaskIssueStore) GetTask(_ context.Context, taskID string) (*taskmodels.Task, error) {
	if f.task == nil || f.task.ID != taskID {
		return nil, errors.New("task not found")
	}
	return f.task, nil
}

func (f *fakeTaskIssueStore) ListTaskRepositories(_ context.Context, taskID string) ([]*taskmodels.TaskRepository, error) {
	if f.task == nil || f.task.ID != taskID {
		return nil, errors.New("task not found")
	}
	return f.repos, nil
}

func (f *fakeTaskIssueStore) GetRepository(_ context.Context, repositoryID string) (*taskmodels.Repository, error) {
	if repo := f.entities[repositoryID]; repo != nil {
		return repo, nil
	}
	return nil, errors.New("repository not found")
}

func (f *fakeTaskIssueStore) UpdateTaskMetadata(_ context.Context, taskID string, metadata map[string]interface{}) (*taskmodels.Task, error) {
	if f.task == nil || f.task.ID != taskID {
		return nil, errors.New("task not found")
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

	resp, err := svc.LinkTaskIssue(context.Background(), "task-1", LinkTaskIssueRequest{
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
		task: &taskmodels.Task{ID: "task-1", Metadata: map[string]interface{}{
			taskMetaIssueURL:     "https://github.com/kdlbs/kandev/issues/1470",
			taskMetaIssueNumber:  1470,
			taskMetaIssueOwner:   "kdlbs",
			taskMetaIssueRepo:    "kandev",
			taskMetaIssueLinked:  true,
			taskMetaIssueWatchID: "watch-1",
			"keep":               "me",
		}},
	}
	svc := NewService(NewMockClient(), AuthMethodPAT, nil, nil, nil, testLogger(t))
	svc.SetTaskIssueStore(store)

	if err := svc.UnlinkTaskIssue(context.Background(), "task-1"); err != nil {
		t.Fatalf("UnlinkTaskIssue: %v", err)
	}
	if _, ok := store.updated[taskMetaIssueURL]; ok {
		t.Fatalf("issue url should be removed: %+v", store.updated)
	}
	if store.updated[taskMetaIssueWatchID] != "watch-1" {
		t.Fatalf("watch metadata should be preserved: %+v", store.updated)
	}
	if store.updated["keep"] != "me" {
		t.Fatalf("unrelated metadata should be preserved: %+v", store.updated)
	}
}
