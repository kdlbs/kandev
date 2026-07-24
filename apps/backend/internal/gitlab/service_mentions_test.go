package gitlab

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

type mentionRecordingClient struct {
	*MockClient
	issues func(context.Context, string, string, int, int) (*IssueSearchPage, error)
	mrs    func(context.Context, string, string, int, int) (*MRSearchPage, error)
}

func (c *mentionRecordingClient) ListIssuesPaged(
	ctx context.Context,
	filter, customQuery string,
	page, perPage int,
) (*IssueSearchPage, error) {
	return c.issues(ctx, filter, customQuery, page, perPage)
}

func (c *mentionRecordingClient) SearchMRsPaged(
	ctx context.Context,
	filter, customQuery string,
	page, perPage int,
) (*MRSearchPage, error) {
	return c.mrs(ctx, filter, customQuery, page, perPage)
}

func newMentionTestService(t *testing.T, host string, client Client) (*Service, *Store) {
	t.Helper()
	service := NewService(host, client, "mock", nil, logger.Default())
	store := newTestStore(t)
	service.SetStore(store)
	return service, store
}

func assertLiteralMentionQuery(t *testing.T, filter, customQuery, wantSearch string, page, perPage int) {
	t.Helper()
	if filter != "" {
		t.Fatalf("raw filter = %q, want empty", filter)
	}
	if page != 1 || perPage != 10 {
		t.Fatalf("pagination = page %d perPage %d, want 1/10", page, perPage)
	}
	values, err := url.ParseQuery(customQuery)
	if err != nil {
		t.Fatalf("parse generated query: %v", err)
	}
	if values.Get("search") != wantSearch || values.Get("in") != "title" ||
		values.Get("scope") != "all" || values.Get("state") != "opened" ||
		values.Get("order_by") != "updated_at" || values.Get("sort") != "desc" {
		t.Fatalf("generated query = %q (%v)", customQuery, values)
	}
}

func TestServiceMentionSearch_BuildsLiteralQueryAndFiltersWorkspaceProjects(t *testing.T) {
	const host = "https://gitlab.example.test/base"
	const query = `auth&scope=created_by_me#fragment`
	client := &mentionRecordingClient{MockClient: NewMockClient(host)}
	client.issues = func(ctx context.Context, filter, customQuery string, page, perPage int) (*IssueSearchPage, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		assertLiteralMentionQuery(t, filter, customQuery, query, page, perPage)
		return &IssueSearchPage{Issues: []*Issue{
			{ID: 1001, IID: 42, ProjectID: 101, ProjectPath: "group/api", Title: "Fix auth", WebURL: host + "/group/api/-/issues/42"},
			{ID: 9001, IID: 9, ProjectID: 999, ProjectPath: "other/secret", Title: "Leak", WebURL: host + "/other/secret/-/issues/9"},
			nil,
		}}, nil
	}
	client.mrs = func(ctx context.Context, filter, customQuery string, page, perPage int) (*MRSearchPage, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		assertLiteralMentionQuery(t, filter, customQuery, query, page, perPage)
		return &MRSearchPage{MRs: []*MR{
			{ID: 2001, IID: 7, ProjectID: 202, ProjectPath: "group/web", Title: "Auth MR", WebURL: host + "/group/web/-/merge_requests/7"},
			{ID: 9002, IID: 8, ProjectID: 999, ProjectPath: "other/secret", Title: "Leak", WebURL: host + "/other/secret/-/merge_requests/8"},
			nil,
		}}, nil
	}
	service, _ := newMentionTestService(t, host, client)
	if err := service.ConfigureMentionScopeForWorkspace(context.Background(), "workspace-1", host+"/", []MentionProjectScope{
		{ID: 101, Path: "group/api"},
		{ID: 202, Path: "group/web"},
	}); err != nil {
		t.Fatalf("configure mention scope: %v", err)
	}

	issues, err := service.SearchMentionIssuesForWorkspace(context.Background(), "workspace-1", query, 99)
	if err != nil {
		t.Fatalf("search issues: %v", err)
	}
	if len(issues) != 1 || issues[0].ID != 1001 || issues[0].IID != 42 ||
		issues[0].ProjectID != 101 || issues[0].ProjectPath != "group/api" ||
		issues[0].Host != host || issues[0].URL != host+"/group/api/-/issues/42" {
		t.Fatalf("issues = %#v", issues)
	}
	mrs, err := service.SearchMentionMRsForWorkspace(context.Background(), "workspace-1", query, 99)
	if err != nil {
		t.Fatalf("search MRs: %v", err)
	}
	if len(mrs) != 1 || mrs[0].ID != 2001 || mrs[0].IID != 7 ||
		mrs[0].ProjectID != 202 || mrs[0].ProjectPath != "group/web" ||
		mrs[0].Host != host || mrs[0].URL != host+"/group/web/-/merge_requests/7" {
		t.Fatalf("MRs = %#v", mrs)
	}
}

func TestServiceMentionIssueSearch_ScansPastForeignFirstPage(t *testing.T) {
	const host = "https://gitlab.example.test"
	issueCalls := 0
	client := &mentionRecordingClient{MockClient: NewMockClient(host)}
	client.issues = func(_ context.Context, _, _ string, page, perPage int) (*IssueSearchPage, error) {
		issueCalls++
		if perPage != 10 {
			t.Fatalf("perPage = %d, want 10", perPage)
		}
		switch page {
		case 1:
			foreign := make([]*Issue, 10)
			for index := range foreign {
				foreign[index] = &Issue{
					ID: int64(9001 + index), IID: 9 + index, ProjectID: 999, ProjectPath: "other/secret",
					Title: "Foreign auth", WebURL: host + "/other/secret/-/issues/" + strconv.Itoa(9+index),
				}
			}
			return &IssueSearchPage{
				Issues: foreign, TotalCount: 11, Page: 1, PerPage: 10,
			}, nil
		case 2:
			return &IssueSearchPage{
				Issues: []*Issue{{
					ID: 1001, IID: 42, ProjectID: 101, ProjectPath: "group/api",
					Title: "Allowed auth", WebURL: host + "/group/api/-/issues/42",
				}},
				TotalCount: 11, Page: 2, PerPage: 10,
			}, nil
		default:
			t.Fatalf("unexpected page %d", page)
			return nil, nil
		}
	}
	service, _ := newMentionTestService(t, host, client)
	if err := service.ConfigureMentionScopeForWorkspace(context.Background(), "workspace-1", host, []MentionProjectScope{
		{ID: 101, Path: "group/api"},
	}); err != nil {
		t.Fatalf("configure mention scope: %v", err)
	}

	issues, err := service.SearchMentionIssuesForWorkspace(context.Background(), "workspace-1", "auth", 1)
	if err != nil {
		t.Fatalf("search issues: %v", err)
	}
	if len(issues) != 1 || issues[0].ID != 1001 {
		t.Fatalf("issues = %#v, want allowed second-page issue", issues)
	}
	if issueCalls != 2 {
		t.Fatalf("issue calls = %d, want 2", issueCalls)
	}
}

func TestServiceMentionIssueSearch_StopsPaginationWhenContextCanceled(t *testing.T) {
	const host = "https://gitlab.example.test"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	issueCalls := 0
	client := &mentionRecordingClient{MockClient: NewMockClient(host)}
	client.issues = func(_ context.Context, _, _ string, page, perPage int) (*IssueSearchPage, error) {
		issueCalls++
		if page == 1 {
			cancel()
		}
		foreign := make([]*Issue, perPage)
		for index := range foreign {
			foreign[index] = &Issue{
				ID: int64(9001 + index), IID: 9 + index, ProjectID: 999, ProjectPath: "other/secret",
				Title: "Foreign auth", WebURL: host + "/other/secret/-/issues/" + strconv.Itoa(9+index),
			}
		}
		return &IssueSearchPage{
			Issues: foreign, TotalCount: 1000, Page: page, PerPage: perPage,
		}, nil
	}
	service, _ := newMentionTestService(t, host, client)
	if err := service.ConfigureMentionScopeForWorkspace(context.Background(), "workspace-1", host, []MentionProjectScope{
		{ID: 101, Path: "group/api"},
	}); err != nil {
		t.Fatalf("configure mention scope: %v", err)
	}

	_, err := service.SearchMentionIssuesForWorkspace(ctx, "workspace-1", "auth", 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
	if issueCalls != 1 {
		t.Fatalf("issue calls = %d, want 1", issueCalls)
	}
}

func TestServiceMentionMRSearch_ScansPastForeignFirstPage(t *testing.T) {
	const host = "https://gitlab.example.test"
	mrCalls := 0
	client := &mentionRecordingClient{MockClient: NewMockClient(host)}
	client.mrs = func(_ context.Context, _, _ string, page, perPage int) (*MRSearchPage, error) {
		mrCalls++
		if perPage != 10 {
			t.Fatalf("perPage = %d, want 10", perPage)
		}
		switch page {
		case 1:
			foreign := make([]*MR, 10)
			for index := range foreign {
				foreign[index] = &MR{
					ID: int64(9001 + index), IID: 9 + index, ProjectID: 999, ProjectPath: "other/secret",
					Title: "Foreign auth", WebURL: host + "/other/secret/-/merge_requests/" + strconv.Itoa(9+index),
				}
			}
			return &MRSearchPage{
				MRs: foreign, TotalCount: 11, Page: 1, PerPage: 10,
			}, nil
		case 2:
			return &MRSearchPage{
				MRs: []*MR{{
					ID: 2001, IID: 42, ProjectID: 101, ProjectPath: "group/api",
					Title: "Allowed auth", WebURL: host + "/group/api/-/merge_requests/42",
				}},
				TotalCount: 11, Page: 2, PerPage: 10,
			}, nil
		default:
			t.Fatalf("unexpected page %d", page)
			return nil, nil
		}
	}
	service, _ := newMentionTestService(t, host, client)
	if err := service.ConfigureMentionScopeForWorkspace(context.Background(), "workspace-1", host, []MentionProjectScope{
		{ID: 101, Path: "group/api"},
	}); err != nil {
		t.Fatalf("configure mention scope: %v", err)
	}

	mrs, err := service.SearchMentionMRsForWorkspace(context.Background(), "workspace-1", "auth", 1)
	if err != nil {
		t.Fatalf("search MRs: %v", err)
	}
	if len(mrs) != 1 || mrs[0].ID != 2001 {
		t.Fatalf("MRs = %#v, want allowed second-page MR", mrs)
	}
	if mrCalls != 2 {
		t.Fatalf("MR calls = %d, want 2", mrCalls)
	}
}

func TestServiceMentionMRSearch_StopsPaginationWhenContextCanceled(t *testing.T) {
	const host = "https://gitlab.example.test"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mrCalls := 0
	client := &mentionRecordingClient{MockClient: NewMockClient(host)}
	client.mrs = func(_ context.Context, _, _ string, page, perPage int) (*MRSearchPage, error) {
		mrCalls++
		if page == 1 {
			cancel()
		}
		foreign := make([]*MR, perPage)
		for index := range foreign {
			foreign[index] = &MR{
				ID: int64(9001 + index), IID: 9 + index, ProjectID: 999, ProjectPath: "other/secret",
				Title: "Foreign auth", WebURL: host + "/other/secret/-/merge_requests/" + strconv.Itoa(9+index),
			}
		}
		return &MRSearchPage{
			MRs: foreign, TotalCount: 1000, Page: page, PerPage: perPage,
		}, nil
	}
	service, _ := newMentionTestService(t, host, client)
	if err := service.ConfigureMentionScopeForWorkspace(context.Background(), "workspace-1", host, []MentionProjectScope{
		{ID: 101, Path: "group/api"},
	}); err != nil {
		t.Fatalf("configure mention scope: %v", err)
	}

	_, err := service.SearchMentionMRsForWorkspace(ctx, "workspace-1", "auth", 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
	if mrCalls != 1 {
		t.Fatalf("MR calls = %d, want 1", mrCalls)
	}
}

func TestServiceMentionSearch_RequiresExplicitMatchingWorkspaceBinding(t *testing.T) {
	called := false
	host := "https://gitlab.example.test"
	client := &mentionRecordingClient{MockClient: NewMockClient(host)}
	client.issues = func(context.Context, string, string, int, int) (*IssueSearchPage, error) {
		called = true
		return &IssueSearchPage{}, nil
	}
	client.mrs = func(context.Context, string, string, int, int) (*MRSearchPage, error) {
		called = true
		return &MRSearchPage{}, nil
	}
	service, store := newMentionTestService(t, host, client)

	if _, err := service.SearchMentionIssuesForWorkspace(context.Background(), "", "auth", 5); !errors.Is(err, ErrMentionWorkspaceRequired) {
		t.Fatalf("blank workspace error = %v", err)
	}
	if _, err := service.SearchMentionMRsForWorkspace(context.Background(), "workspace-1", "auth", 5); !errors.Is(err, ErrMentionUnsupportedScope) {
		t.Fatalf("unbound workspace error = %v", err)
	}
	if err := store.UpsertMentionScope(context.Background(), &MentionScope{
		WorkspaceID: "workspace-1",
		Host:        "https://other-gitlab.example.test",
		Projects:    []MentionProjectScope{{ID: 101, Path: "group/api"}},
	}); err != nil {
		t.Fatalf("seed mismatched scope: %v", err)
	}
	if _, err := service.SearchMentionIssuesForWorkspace(context.Background(), "workspace-1", "auth", 5); !errors.Is(err, ErrMentionUnsupportedScope) {
		t.Fatalf("host-mismatched workspace error = %v", err)
	}
	if called {
		t.Fatal("global GitLab client searched without exact workspace binding")
	}
}

func TestServiceConfigureMentionScope_RejectsUnsafeOrEmptyScope(t *testing.T) {
	host := "https://gitlab.example.test"
	client := &mentionRecordingClient{MockClient: NewMockClient(host)}
	client.issues = func(context.Context, string, string, int, int) (*IssueSearchPage, error) {
		return &IssueSearchPage{}, nil
	}
	client.mrs = func(context.Context, string, string, int, int) (*MRSearchPage, error) { return &MRSearchPage{}, nil }
	service, _ := newMentionTestService(t, host, client)

	tests := []struct {
		name       string
		workspace  string
		host       string
		projects   []MentionProjectScope
		wantTarget error
	}{
		{name: "blank workspace", host: host, projects: []MentionProjectScope{{ID: 1, Path: "group/api"}}, wantTarget: ErrMentionWorkspaceRequired},
		{name: "global default forbidden", workspace: "workspace-1", host: "", projects: []MentionProjectScope{{ID: 1, Path: "group/api"}}, wantTarget: ErrMentionUnsupportedScope},
		{name: "foreign host", workspace: "workspace-1", host: "https://other.example.test", projects: []MentionProjectScope{{ID: 1, Path: "group/api"}}, wantTarget: ErrMentionUnsupportedScope},
		{name: "userinfo", workspace: "workspace-1", host: "https://user:secret@gitlab.example.test", projects: []MentionProjectScope{{ID: 1, Path: "group/api"}}, wantTarget: ErrMentionInvalidScope},
		{name: "empty projects", workspace: "workspace-1", host: host, wantTarget: ErrMentionInvalidScope},
		{name: "invalid project", workspace: "workspace-1", host: host, projects: []MentionProjectScope{{ID: 0, Path: "../secret"}}, wantTarget: ErrMentionInvalidScope},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := service.ConfigureMentionScopeForWorkspace(
				context.Background(), test.workspace, test.host, test.projects,
			)
			if !errors.Is(err, test.wantTarget) {
				t.Fatalf("error = %v, want %v", err, test.wantTarget)
			}
		})
	}
}

func TestServiceMentionSearch_PropagatesCancellation(t *testing.T) {
	host := "https://gitlab.example.test"
	client := &mentionRecordingClient{MockClient: NewMockClient(host)}
	client.issues = func(ctx context.Context, _ string, _ string, _ int, _ int) (*IssueSearchPage, error) {
		return nil, ctx.Err()
	}
	client.mrs = func(ctx context.Context, _ string, _ string, _ int, _ int) (*MRSearchPage, error) {
		return nil, ctx.Err()
	}
	service, _ := newMentionTestService(t, host, client)
	if err := service.ConfigureMentionScopeForWorkspace(context.Background(), "workspace-1", host, []MentionProjectScope{{ID: 101, Path: "group/api"}}); err != nil {
		t.Fatalf("configure scope: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.SearchMentionIssuesForWorkspace(ctx, "workspace-1", "auth", 5); !errors.Is(err, context.Canceled) {
		t.Fatalf("issue cancellation = %v", err)
	}
	if _, err := service.SearchMentionMRsForWorkspace(ctx, "workspace-1", "auth", 5); !errors.Is(err, context.Canceled) {
		t.Fatalf("MR cancellation = %v", err)
	}
}
