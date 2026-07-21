package mentions

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

type fakeTaskMentionSearcher struct {
	workspaceID   string
	query         string
	excludeTaskID string
	limit         int
	tasks         []*models.Task
	err           error
	getTask       *models.Task
	getErr        error
	getCalled     bool
}

func (s *fakeTaskMentionSearcher) GetTask(_ context.Context, _ string) (*models.Task, error) {
	s.getCalled = true
	return s.getTask, s.getErr
}

func (s *fakeTaskMentionSearcher) SearchMentionTasks(
	_ context.Context,
	workspaceID, query, excludeTaskID string,
	limit int,
) ([]*models.Task, error) {
	s.workspaceID = workspaceID
	s.query = query
	s.excludeTaskID = excludeTaskID
	s.limit = limit
	return s.tasks, s.err
}

func TestTaskProviderSearchMapsWorkspaceTasks(t *testing.T) {
	searcher := &fakeTaskMentionSearcher{tasks: []*models.Task{
		{ID: "task/one", WorkspaceID: "workspace-1", Identifier: "KAN-7", Title: "Fix authentication"},
		{ID: "other", WorkspaceID: "workspace-2", Identifier: "KAN-8", Title: "Wrong workspace"},
		nil,
	}}
	provider := NewTaskProvider(searcher)

	descriptor := provider.Descriptor()
	if descriptor.Source != "kandev_tasks" || descriptor.Provider != "kandev" || descriptor.Kind != "task" {
		t.Fatalf("descriptor = %#v", descriptor)
	}
	candidates, err := provider.Search(context.Background(), SearchRequest{
		WorkspaceID:   "workspace-1",
		Query:         "auth",
		ExcludeTaskID: "current-task",
		Limit:         4,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if searcher.workspaceID != "workspace-1" || searcher.query != "auth" ||
		searcher.excludeTaskID != "current-task" || searcher.limit != 4 {
		t.Fatalf("delegated request = workspace %q query %q exclude %q limit %d",
			searcher.workspaceID, searcher.query, searcher.excludeTaskID, searcher.limit)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidates = %#v, want one workspace-owned task", candidates)
	}
	want := Candidate{
		ID:    "task/one",
		Key:   "KAN-7",
		Title: "Fix authentication",
		URL:   "/t/task%2Fone",
		Scope: "workspace-1",
	}
	if candidates[0] != want {
		t.Fatalf("candidate = %#v, want %#v", candidates[0], want)
	}
}

func TestTaskProviderSearchPreservesRepositoryFailure(t *testing.T) {
	wantErr := errors.New("database unavailable")
	provider := NewTaskProvider(&fakeTaskMentionSearcher{err: wantErr})

	_, err := provider.Search(context.Background(), SearchRequest{
		WorkspaceID: "workspace-1",
		Query:       "auth",
		Limit:       DefaultLimit,
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want wrapped %v", err, wantErr)
	}
}

func TestTaskProviderWithoutServiceIsNotConfigured(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(NewTaskProvider(nil)); err != nil {
		t.Fatalf("register: %v", err)
	}

	response, err := NewService(registry).Search(context.Background(), SearchRequest{
		WorkspaceID: "workspace-1",
		Query:       "auth",
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(response.Groups) != 1 || response.Groups[0].Status != StatusNotConfigured {
		t.Fatalf("groups = %#v, want safe not-configured status", response.Groups)
	}
}

func TestTaskProviderAuthorizesSearchAndSubmissionScope(t *testing.T) {
	searcher := &fakeTaskMentionSearcher{getTask: &models.Task{
		ID: "task-1", WorkspaceID: "workspace-1", Title: "Task",
	}}
	provider := NewTaskProvider(searcher)
	authorizer, ok := provider.(ReferenceAuthorizer)
	if !ok {
		t.Fatal("task provider must authorize its references")
	}
	ref := apiv1.EntityReference{
		Version:  apiv1.EntityReferenceVersion,
		Ref:      "mention:v1:kandev:task:workspace-1:task-1",
		Provider: "kandev", Kind: "task", ID: "task-1",
		Title: "Task", URL: "/t/task-1", Scope: "workspace-1",
	}
	if err := authorizer.AuthorizeReference(context.Background(), ReferenceAuthorizationRequest{
		WorkspaceID: "workspace-1", Purpose: ReferencePurposeSearch, Reference: ref,
	}); err != nil {
		t.Fatalf("authorize search result: %v", err)
	}
	if searcher.getCalled {
		t.Fatal("search-result authorization should not repeat the task lookup")
	}
	if err := authorizer.AuthorizeReference(context.Background(), ReferenceAuthorizationRequest{
		WorkspaceID: "workspace-1", Purpose: ReferencePurposeSubmission, Reference: ref,
	}); err != nil {
		t.Fatalf("authorize submission: %v", err)
	}
	if !searcher.getCalled {
		t.Fatal("submission authorization must reload the referenced task")
	}

	wrongScope := ref
	wrongScope.Scope = "workspace-2"
	if err := authorizer.AuthorizeReference(context.Background(), ReferenceAuthorizationRequest{
		WorkspaceID: "workspace-1", Purpose: ReferencePurposeSubmission, Reference: wrongScope,
	}); err == nil {
		t.Fatal("cross-workspace reference was authorized")
	}
	wrongURL := ref
	wrongURL.URL = "/t/other"
	if err := authorizer.AuthorizeReference(context.Background(), ReferenceAuthorizationRequest{
		WorkspaceID: "workspace-1", Purpose: ReferencePurposeSubmission, Reference: wrongURL,
	}); err == nil {
		t.Fatal("mismatched task URL was authorized")
	}
}
