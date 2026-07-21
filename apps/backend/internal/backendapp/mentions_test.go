package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/mentions"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

type fakeMentionScopeResolver struct {
	workspaces map[string]*models.Workspace
	tasks      map[string]*models.Task
	err        error
}

func (r *fakeMentionScopeResolver) GetWorkspace(_ context.Context, id string) (*models.Workspace, error) {
	if r.err != nil {
		return nil, r.err
	}
	workspace, ok := r.workspaces[id]
	if !ok {
		return nil, repository.ErrWorkspaceNotFound
	}
	return workspace, nil
}

func (r *fakeMentionScopeResolver) GetTask(_ context.Context, id string) (*models.Task, error) {
	if r.err != nil {
		return nil, r.err
	}
	task, ok := r.tasks[id]
	if !ok {
		return nil, repository.ErrTaskNotFound
	}
	return task, nil
}

func (r *fakeMentionScopeResolver) ResolveWorkspace(_ context.Context, sessionID, _ string) (string, error) {
	if sessionID == "session-1" {
		return "workspace-1", nil
	}
	return "", errors.New("unknown conversation")
}

type fakeComposedMentionProvider struct {
	descriptor mentions.ProviderDescriptor
	candidates []mentions.Candidate
	err        error
	calls      int
	workspace  string
	waitCancel bool
	canceled   bool
	started    chan struct{}
	cancelSeen chan struct{}
}

func (p *fakeComposedMentionProvider) Descriptor() mentions.ProviderDescriptor {
	return p.descriptor
}

func (p *fakeComposedMentionProvider) Search(
	ctx context.Context,
	request mentions.SearchRequest,
) ([]mentions.Candidate, error) {
	p.calls++
	p.workspace = request.WorkspaceID
	if p.started != nil {
		close(p.started)
	}
	if p.waitCancel {
		<-ctx.Done()
		p.canceled = errors.Is(ctx.Err(), context.Canceled)
		if p.cancelSeen != nil {
			close(p.cancelSeen)
		}
		return nil, ctx.Err()
	}
	return p.candidates, p.err
}

func (p *fakeComposedMentionProvider) AuthorizeReference(
	context.Context,
	mentions.ReferenceAuthorizationRequest,
) error {
	return nil
}

func TestMentionHTTPCompositionValidatesWorkspaceBeforeMixedProviderFanout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	success := &fakeComposedMentionProvider{
		descriptor: mentions.ProviderDescriptor{
			Source: "plugin:test:issues", Provider: "plugin:test:tracker", Kind: "issue",
			DisplayName: "Test tracker", KindLabel: "Issue", Order: 90,
		},
		candidates: []mentions.Candidate{
			{ID: "1", Key: "T-1", Title: "First", URL: "https://tracker.example/items/1", Scope: "workspace-1"},
			{ID: "2", Key: "T-2", Title: "Second", URL: "https://tracker.example/items/2", Scope: "workspace-1"},
			{ID: "3", Key: "T-3", Title: "Third", URL: "https://tracker.example/items/3", Scope: "workspace-1"},
		},
	}
	failure := &fakeComposedMentionProvider{
		descriptor: mentions.ProviderDescriptor{
			Source: "plugin:test:pulls", Provider: "plugin:test:tracker", Kind: "pull_request",
			DisplayName: "Test tracker", KindLabel: "Pull request", Order: 91,
		},
		err: mentions.NewProviderError(mentions.StatusUnauthorized, errors.New("secret upstream detail")),
	}
	timeout := &fakeComposedMentionProvider{
		descriptor: mentions.ProviderDescriptor{
			Source: "plugin:test:alerts", Provider: "plugin:test:alerts", Kind: "issue",
			DisplayName: "Test alerts", KindLabel: "Issue", Order: 92,
		},
		err: context.DeadlineExceeded,
	}
	resolver := &fakeMentionScopeResolver{workspaces: map[string]*models.Workspace{
		"workspace-1": {ID: "workspace-1", Name: "One"},
	}}
	components, err := newMentionComponents(resolver, resolver, failure, timeout, success)
	if err != nil {
		t.Fatalf("compose mentions: %v", err)
	}
	router := gin.New()
	registerMentionRoutes(router, components)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(
		http.MethodGet,
		"/api/v1/workspaces/workspace-1/mentions/search?q=auth&limit=2",
		nil,
	))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body apiv1.MentionSearchResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Groups) != 3 || len(body.Groups[0].Results) != 2 ||
		body.Groups[1].Status != mentions.StatusUnauthorized ||
		body.Groups[2].Status != mentions.StatusTimeout {
		t.Fatalf("groups = %#v", body.Groups)
	}
	if success.workspace != "workspace-1" || failure.workspace != "workspace-1" {
		t.Fatalf("provider workspaces = %q, %q", success.workspace, failure.workspace)
	}

	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, httptest.NewRequest(
		http.MethodGet,
		"/api/v1/workspaces/workspace-2/mentions/search?q=auth",
		nil,
	))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing workspace status = %d, body = %s", missing.Code, missing.Body.String())
	}
	if success.calls != 1 || failure.calls != 1 || timeout.calls != 1 {
		t.Fatalf(
			"providers called after missing workspace: success=%d failure=%d timeout=%d",
			success.calls, failure.calls, timeout.calls,
		)
	}

	foreignExclude := httptest.NewRecorder()
	router.ServeHTTP(foreignExclude, httptest.NewRequest(
		http.MethodGet,
		"/api/v1/workspaces/workspace-1/mentions/search?q=auth&exclude_task_id=foreign-task",
		nil,
	))
	if foreignExclude.Code != http.StatusBadRequest {
		t.Fatalf("foreign exclude status = %d, body = %s", foreignExclude.Code, foreignExclude.Body.String())
	}
	if success.calls != 1 || failure.calls != 1 || timeout.calls != 1 {
		t.Fatal("providers called after foreign exclude task")
	}

	resolver.err = errors.New("database unavailable")
	infrastructureFailure := httptest.NewRecorder()
	router.ServeHTTP(infrastructureFailure, httptest.NewRequest(
		http.MethodGet,
		"/api/v1/workspaces/workspace-1/mentions/search?q=auth",
		nil,
	))
	if infrastructureFailure.Code != http.StatusInternalServerError {
		t.Fatalf(
			"workspace failure status = %d, body = %s",
			infrastructureFailure.Code,
			infrastructureFailure.Body.String(),
		)
	}
	if success.calls != 1 || failure.calls != 1 || timeout.calls != 1 {
		t.Fatal("providers called after workspace infrastructure failure")
	}
}

func TestMentionHTTPCompositionPropagatesRequestCancellation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := &fakeComposedMentionProvider{
		descriptor: mentions.ProviderDescriptor{
			Source: "plugin:test:cancel", Provider: "plugin:test:cancel", Kind: "issue",
			DisplayName: "Cancellation test", KindLabel: "Issue", Order: 90,
		},
		waitCancel: true,
		started:    make(chan struct{}),
		cancelSeen: make(chan struct{}),
	}
	resolver := &fakeMentionScopeResolver{workspaces: map[string]*models.Workspace{
		"workspace-1": {ID: "workspace-1", Name: "One"},
	}}
	components, err := newMentionComponents(resolver, resolver, provider)
	if err != nil {
		t.Fatalf("compose mentions: %v", err)
	}
	router := gin.New()
	registerMentionRoutes(router, components)

	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/workspaces/workspace-1/mentions/search?q=cancel",
		nil,
	).WithContext(ctx)
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		router.ServeHTTP(response, request)
		close(done)
	}()
	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("provider did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("request did not stop after cancellation")
	}
	select {
	case <-provider.cancelSeen:
	case <-time.After(time.Second):
		t.Fatal("provider did not observe request cancellation")
	}

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if !provider.canceled {
		t.Fatal("provider did not observe request cancellation")
	}
}

func TestBuiltinMentionProvidersRemainDescriptorDrivenWhenIntegrationsAreNil(t *testing.T) {
	providers := builtinMentionProviders(nil, &Services{}, nil)
	want := []struct {
		source string
		order  int
	}{
		{"kandev_tasks", 0},
		{"jira_issues", 20},
		{"linear_issues", 30},
		{"github_issues", 40},
		{"github_pull_requests", 41},
		{"gitlab_issues", 50},
		{"gitlab_merge_requests", 51},
		{"azure_work_items", 60},
		{"azure_pull_requests", 61},
		{"sentry_issues", 70},
	}
	if len(providers) != len(want) {
		t.Fatalf("provider count = %d, want %d", len(providers), len(want))
	}
	for index, expected := range want {
		descriptor := providers[index].Descriptor()
		if descriptor.Source != expected.source || descriptor.Order != expected.order {
			t.Fatalf("provider %d descriptor = %#v, want source=%q order=%d", index, descriptor, expected.source, expected.order)
		}
	}
}
