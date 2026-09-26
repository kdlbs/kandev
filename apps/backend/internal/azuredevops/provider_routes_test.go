package azuredevops

import (
	"context"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
)

func TestProvideBuildsAServiceAndOnlyMocksWhenAsked(t *testing.T) {
	db := newTestDB(t)
	service, cleanup, err := Provide(db, db, newFakeSecretStore(), nil, logger.Default())
	if err != nil || service == nil || cleanup == nil {
		t.Fatalf("Provide returned service %v, cleanup present %t, err %v", service, cleanup != nil, err)
	}
	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	if service.Store() == nil {
		t.Fatal("Provide returned a service without a store")
	}
	if service.MockClient() != nil {
		t.Fatalf("Provide wired a mock client without %s", mockEnvVar)
	}

	t.Setenv(mockEnvVar, "true")
	mocked, mockedCleanup, err := Provide(newTestDB(t), db, newFakeSecretStore(), nil, logger.Default())
	if err != nil {
		t.Fatalf("Provide with mock: %v", err)
	}
	t.Cleanup(func() { _ = mockedCleanup() })
	if mocked.MockClient() == nil {
		t.Fatalf("Provide did not wire a mock client with %s=true", mockEnvVar)
	}
	// The mock client is what the factory hands back for any workspace.
	if client := mocked.clientFn(&Config{OrganizationURL: "https://dev.azure.com/acme"}, "pat"); client != mocked.MockClient() {
		t.Fatalf("client factory = %T, want the mock client", client)
	}
}

func TestRegisterMockRoutesOnlyMountsWhenAMockIsWired(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newTestDB(t)
	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	plain := NewService(store, newFakeSecretStore(), func(*Config, string) Client { return &invalidClient{} }, logger.Default())
	plainRouter := gin.New()
	RegisterMockRoutes(plainRouter, plain, logger.Default())
	if response := performRequest(t, plainRouter, http.MethodPost, "/api/v1/azure-devops/mock/state", MockState{}); response.Code != http.StatusNotFound {
		t.Fatalf("mock route mounted without a mock client: %d", response.Code)
	}
	RegisterMockRoutes(plainRouter, nil, logger.Default())

	mock := NewMockClient()
	service := NewService(store, newFakeSecretStore(), func(*Config, string) Client { return mock }, logger.Default())
	service.mock = mock
	router := gin.New()
	RegisterMockRoutes(router, service, logger.Default())

	seeded := performRequest(t, router, http.MethodPost, "/api/v1/azure-devops/mock/state", MockState{
		Authenticated: true,
		User:          TestConnectionResult{OK: true, ID: "seeded-user", DisplayName: "Seeded"},
		Projects:      []Project{{ID: "project-1", Name: "Seeded Platform"}},
	})
	if seeded.Code != http.StatusOK || !decodeJSON[map[string]bool](t, seeded)["seeded"] {
		t.Fatalf("POST mock state = %d: %s", seeded.Code, seeded.Body.String())
	}
	projects, err := mock.ListProjects(context.Background())
	if err != nil || len(projects) != 1 || projects[0].Name != "Seeded Platform" {
		t.Fatalf("seeded projects = %+v, %v", projects, err)
	}

	invalid := performRequest(t, router, http.MethodPost, "/api/v1/azure-devops/mock/state", "nope")
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("malformed mock state = %d: %s", invalid.Code, invalid.Body.String())
	}

	reset := performRequest(t, router, http.MethodDelete, "/api/v1/azure-devops/mock/state", nil)
	if reset.Code != http.StatusOK || !decodeJSON[map[string]bool](t, reset)["reset"] {
		t.Fatalf("DELETE mock state = %d: %s", reset.Code, reset.Body.String())
	}
	if projects, err := mock.ListProjects(context.Background()); err != nil || len(projects) != 0 {
		t.Fatalf("projects after reset = %+v, %v", projects, err)
	}
	auth, err := mock.TestAuth(context.Background())
	if err != nil || !auth.OK || auth.ID != "mock-user" {
		t.Fatalf("auth after reset = %+v, %v", auth, err)
	}
}

// TestDefaultClientFactoryFailsClosedWithoutConfig proves the production
// factory never returns a client that silently succeeds when the workspace
// configuration is missing.
func TestDefaultClientFactoryFailsClosedWithoutConfig(t *testing.T) {
	client := DefaultClientFactory(nil, "pat")
	ctx := context.Background()
	calls := map[string]func() error{
		"TestAuth":              func() error { _, err := client.TestAuth(ctx); return err },
		"ListProjects":          func() error { _, err := client.ListProjects(ctx); return err },
		"ListRepositories":      func() error { _, err := client.ListRepositories(ctx, "p"); return err },
		"QueryWIQL":             func() error { _, err := client.QueryWIQL(ctx, "p", "SELECT", 1); return err },
		"GetWorkItem":           func() error { _, err := client.GetWorkItem(ctx, "p", 1); return err },
		"ListPullRequests":      func() error { _, err := client.ListPullRequests(ctx, PullRequestFilter{}); return err },
		"GetPullRequest":        func() error { _, err := client.GetPullRequest(ctx, "p", "r", 1); return err },
		"ListReviewers":         func() error { _, err := client.ListReviewers(ctx, "p", "r", 1); return err },
		"ListThreads":           func() error { _, err := client.ListThreads(ctx, "p", "r", 1); return err },
		"ListLinkedWorkItems":   func() error { _, err := client.ListLinkedWorkItems(ctx, "p", "r", 1); return err },
		"ListPolicyEvaluations": func() error { _, err := client.ListPolicyEvaluations(ctx, "p", 1); return err },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			err := call()
			if err == nil || err.Error() != "azure devops: config is required" {
				t.Fatalf("error = %v, want the missing-config failure", err)
			}
		})
	}

	rest, ok := DefaultClientFactory(&Config{OrganizationURL: "https://dev.azure.com/acme"}, "pat").(*RESTClient)
	if !ok || rest.organization != "acme" || rest.pat != "pat" || rest.initErr != nil {
		t.Fatalf("configured factory = %+v, ok = %t", rest, ok)
	}
}

// TestControllerReadRoutesSurfaceUpstreamFailures walks every workspace-scoped
// read route with a failing Azure client and proves none of them answers 200
// with empty data.
func TestControllerReadRoutesSurfaceUpstreamFailures(t *testing.T) {
	upstream := &APIError{StatusCode: http.StatusInternalServerError, Endpoint: "/_apis", Body: "boom"}
	routes := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"projects", http.MethodGet, "/api/v1/azure-devops/projects?workspace_id=ws-1", nil},
		{"teams", http.MethodGet, "/api/v1/azure-devops/teams?workspace_id=ws-1&project=project-1", nil},
		{"boards", http.MethodGet, "/api/v1/azure-devops/boards?workspace_id=ws-1&project=project-1&team=team-1", nil},
		{"board snapshot", http.MethodGet, "/api/v1/azure-devops/boards/board-1?workspace_id=ws-1&project=project-1&team=team-1", nil},
		{"repositories", http.MethodGet, "/api/v1/azure-devops/repositories?workspace_id=ws-1&project=project-1", nil},
		{"branches", http.MethodGet, "/api/v1/azure-devops/branches?workspace_id=ws-1&project=project-1&repository=repo-1", nil},
		{"identity", http.MethodGet, "/api/v1/azure-devops/identity?workspace_id=ws-1", nil},
		{"work item", http.MethodGet, "/api/v1/azure-devops/work-items/101?workspace_id=ws-1&project=project-1", nil},
		{"work item comments", http.MethodGet, "/api/v1/azure-devops/work-items/101/comments?workspace_id=ws-1&project=project-1", nil},
		{"pull request", http.MethodGet, "/api/v1/azure-devops/pull-requests/project-1/repo-1/42?workspace_id=ws-1", nil},
		{"pull request feedback", http.MethodGet, "/api/v1/azure-devops/pull-requests/project-1/repo-1/42/feedback?workspace_id=ws-1", nil},
		{"pull requests", http.MethodGet, "/api/v1/azure-devops/pull-requests?workspace_id=ws-1&project=project-1&repository=repo-1", nil},
		{
			"work item search", http.MethodPost, "/api/v1/azure-devops/work-items/search?workspace_id=ws-1",
			map[string]any{"project": "project-1", "wiql": "SELECT [System.Id] FROM WorkItems"},
		},
		{
			"work item assignment", http.MethodPatch, "/api/v1/azure-devops/work-items/101?workspace_id=ws-1&project=project-1",
			map[string]any{"revision": 7, "assigneeAction": unassignAction},
		},
		{
			"board work item", http.MethodPatch,
			"/api/v1/azure-devops/boards/board-1/work-items/101?workspace_id=ws-1&project=project-1&team=team-1",
			map[string]any{"revision": 7, "columnDone": true},
		},
	}
	for _, route := range routes {
		t.Run(route.name, func(t *testing.T) {
			router, service := newRouteFixture(t, &failingAzureClient{err: upstream})
			seedRouteConfig(t, service)
			response := performRequest(t, router, route.method, route.path, route.body)
			if response.Code != http.StatusBadGateway {
				t.Fatalf("status = %d, want 502: %s", response.Code, response.Body.String())
			}
			if body := decodeJSON[map[string]string](t, response); body["error"] != upstream.Error() {
				t.Fatalf("error body = %q, want %q", body["error"], upstream.Error())
			}
		})
	}
}

// failingAzureClient implements every optional capability the routes probe for
// and fails all of them, so the tests above exercise the handler error branch
// rather than a "capability unavailable" short circuit.
type failingAzureClient struct {
	invalidClient
	err error
}

func (c *failingAzureClient) ListTeams(context.Context, string) ([]Team, error) { return nil, c.err }
func (c *failingAzureClient) ListBoards(context.Context, string, string) ([]BoardReference, error) {
	return nil, c.err
}

func (c *failingAzureClient) GetBoardSnapshot(context.Context, string, string, string) (*BoardSnapshot, error) {
	return nil, c.err
}

func (c *failingAzureClient) UpdateBoardWorkItem(context.Context, string, string, string, int, BoardWorkItemUpdateRequest) (*BoardWorkItem, error) {
	return nil, c.err
}

func (c *failingAzureClient) UpdateWorkItem(context.Context, string, int, WorkItemAssignmentRequest) (*WorkItem, error) {
	return nil, c.err
}

func (c *failingAzureClient) ListBranches(context.Context, string, string) ([]Branch, error) {
	return nil, c.err
}

func (c *failingAzureClient) GetWorkItemDetail(context.Context, string, int) (*WorkItemDetail, error) {
	return nil, c.err
}

func (c *failingAzureClient) ListWorkItemComments(context.Context, string, int, string) (*WorkItemCommentPage, error) {
	return nil, c.err
}

func (c *failingAzureClient) GetCurrentIdentity(context.Context) (*Identity, error) {
	return nil, c.err
}

func (c *failingAzureClient) TestAuth(context.Context) (*TestConnectionResult, error) {
	return nil, c.err
}

func (c *failingAzureClient) ListProjects(context.Context) ([]Project, error) { return nil, c.err }
func (c *failingAzureClient) ListRepositories(context.Context, string) ([]Repository, error) {
	return nil, c.err
}

func (c *failingAzureClient) QueryWIQL(context.Context, string, string, int) (*WorkItemSearchResult, error) {
	return nil, c.err
}

func (c *failingAzureClient) ListPullRequests(context.Context, PullRequestFilter) (*PullRequestPage, error) {
	return nil, c.err
}

func (c *failingAzureClient) GetPullRequest(context.Context, string, string, int) (*PullRequest, error) {
	return nil, c.err
}
