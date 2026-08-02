package azuredevops

import (
	"context"
	"errors"
	"testing"
)

type fakeDetailClient struct {
	invalidClient
	detail   *WorkItemDetail
	comments *WorkItemCommentPage
	identity *Identity
}

func (f *fakeDetailClient) TestAuth(context.Context) (*TestConnectionResult, error) {
	return &TestConnectionResult{OK: true, ID: "me", DisplayName: "Ada"}, nil
}

func (f *fakeDetailClient) GetWorkItemDetail(context.Context, string, int) (*WorkItemDetail, error) {
	return f.detail, nil
}

func (f *fakeDetailClient) ListWorkItemComments(context.Context, string, int, string) (*WorkItemCommentPage, error) {
	return f.comments, nil
}

func (f *fakeDetailClient) GetCurrentIdentity(context.Context) (*Identity, error) {
	return f.identity, nil
}

func TestServiceGetWorkItemDetail(t *testing.T) {
	client := &fakeDetailClient{
		detail:   &WorkItemDetail{WorkItem: WorkItem{ID: 101, Title: "Ship it"}},
		comments: &WorkItemCommentPage{Comments: []WorkItemComment{{ID: 9, Content: "Looks good"}}},
		identity: &Identity{ID: "me", DisplayName: "Ada"},
	}
	service, _, _ := newTestService(t, func(*Config, string) Client { return client })
	if _, err := service.SetConfigForWorkspace(t.Context(), "ws-1", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", PAT: "pat",
	}); err != nil {
		t.Fatalf("set config: %v", err)
	}

	detail, err := service.GetWorkItemDetailForWorkspace(t.Context(), "ws-1", "project-1", 101)
	if err != nil || detail.ID != 101 {
		t.Fatalf("GetWorkItemDetailForWorkspace = %+v, %v", detail, err)
	}
	comments, err := service.ListWorkItemCommentsForWorkspace(t.Context(), "ws-1", "project-1", 101, "opaque")
	if err != nil || len(comments.Comments) != 1 {
		t.Fatalf("ListWorkItemCommentsForWorkspace = %+v, %v", comments, err)
	}
	identity, err := service.GetCurrentIdentityForWorkspace(t.Context(), "ws-1")
	if err != nil || identity.ID != "me" {
		t.Fatalf("GetCurrentIdentityForWorkspace = %+v, %v", identity, err)
	}
}

func TestServiceDetailCapabilitiesReportNotConfigured(t *testing.T) {
	service, _, _ := newTestService(t, func(*Config, string) Client { return &invalidClient{} })
	if _, err := service.SetConfigForWorkspace(t.Context(), "ws-1", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", PAT: "pat",
	}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	if _, err := service.GetWorkItemDetailForWorkspace(t.Context(), "ws-1", "project-1", 101); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("GetWorkItemDetailForWorkspace error = %v, want ErrNotConfigured", err)
	}
	if _, err := service.ListWorkItemCommentsForWorkspace(t.Context(), "ws-1", "project-1", 101, ""); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("ListWorkItemCommentsForWorkspace error = %v, want ErrNotConfigured", err)
	}
	if _, err := service.GetCurrentIdentityForWorkspace(t.Context(), "ws-1"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("GetCurrentIdentityForWorkspace error = %v, want ErrNotConfigured", err)
	}
}
