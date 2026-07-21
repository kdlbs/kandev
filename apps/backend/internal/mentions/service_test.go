package mentions

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

type fakeProvider struct {
	descriptor ProviderDescriptor
	search     func(context.Context, SearchRequest) ([]Candidate, error)
	authorize  func(context.Context, ReferenceAuthorizationRequest) error
}

type searchOnlyProvider struct {
	descriptor ProviderDescriptor
}

func (p searchOnlyProvider) Descriptor() ProviderDescriptor {
	return p.descriptor
}

func (p searchOnlyProvider) Search(context.Context, SearchRequest) ([]Candidate, error) {
	return nil, nil
}

func (p fakeProvider) Descriptor() ProviderDescriptor {
	return p.descriptor
}

func (p fakeProvider) Search(ctx context.Context, request SearchRequest) ([]Candidate, error) {
	return p.search(ctx, request)
}

func (p fakeProvider) AuthorizeReference(ctx context.Context, request ReferenceAuthorizationRequest) error {
	if p.authorize == nil {
		return nil
	}
	return p.authorize(ctx, request)
}

func TestServiceSearch_RegistryOwnsNamespacedProviderIdentity(t *testing.T) {
	registry := NewRegistry()
	err := registry.Register(fakeProvider{
		descriptor: ProviderDescriptor{
			Source:      "plugin:acme:incidents",
			Provider:    "plugin:acme:tracker",
			Kind:        "incident",
			DisplayName: "Acme incidents",
			KindLabel:   "Incident",
		},
		search: func(_ context.Context, request SearchRequest) ([]Candidate, error) {
			if request.Query != "auth" {
				t.Fatalf("query = %q, want trimmed query auth", request.Query)
			}
			if request.Limit != DefaultLimit {
				t.Fatalf("limit = %d, want default %d", request.Limit, DefaultLimit)
			}
			return []Candidate{{
				ID:    "incident-7",
				Key:   "INC-7",
				Title: "Authentication outage",
				URL:   "https://incidents.example.test/7",
				Scope: "tenant-a",
			}}, nil
		},
	})
	if err != nil {
		t.Fatalf("register provider: %v", err)
	}

	response, err := NewService(registry).Search(context.Background(), SearchRequest{
		WorkspaceID: "workspace-1",
		Query:       "  auth  ",
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(response.Groups) != 1 || len(response.Groups[0].Results) != 1 {
		t.Fatalf("groups = %#v, want one group with one result", response.Groups)
	}
	group := response.Groups[0]
	result := group.Results[0]
	if group.Source != "plugin:acme:incidents" || result.Provider != "plugin:acme:tracker" || result.Kind != "incident" {
		t.Fatalf("registry identity = source %q provider %q kind %q", group.Source, result.Provider, result.Kind)
	}
	const wantRef = "mention:v1:plugin%3Aacme%3Atracker:incident:tenant-a:incident-7"
	if result.Ref != wantRef {
		t.Fatalf("ref = %q, want %q", result.Ref, wantRef)
	}
}

func TestRegistryRegister_RejectsUnsafeAndDuplicateSources(t *testing.T) {
	provider := func(source string) fakeProvider {
		return fakeProvider{
			descriptor: ProviderDescriptor{Source: source, Provider: "acme", Kind: "issue"},
			search: func(context.Context, SearchRequest) ([]Candidate, error) {
				return nil, nil
			},
		}
	}

	registry := NewRegistry()
	if err := registry.Register(provider("GitHub Issues")); err == nil {
		t.Fatal("register unsafe source: err = nil, want validation error")
	}
	if err := registry.Register(provider("plugin:acme:issues")); err != nil {
		t.Fatalf("register valid source: %v", err)
	}
	if err := registry.Register(provider("plugin:acme:issues")); err == nil {
		t.Fatal("register duplicate source: err = nil, want duplicate error")
	}
}

func TestRegistryAuthorizesReferenceThroughRegisteredProvider(t *testing.T) {
	registry := NewRegistry()
	called := false
	provider := fakeProvider{
		descriptor: ProviderDescriptor{Source: "acme_issues", Provider: "acme", Kind: "issue"},
		search: func(context.Context, SearchRequest) ([]Candidate, error) {
			return nil, nil
		},
		authorize: func(_ context.Context, request ReferenceAuthorizationRequest) error {
			called = true
			if request.WorkspaceID != "workspace-1" || request.Purpose != ReferencePurposeSubmission ||
				request.Reference.ID != "issue-7" {
				t.Fatalf("authorization request = %#v", request)
			}
			return nil
		},
	}
	if err := registry.Register(provider); err != nil {
		t.Fatalf("register: %v", err)
	}
	ref := apiv1.EntityReference{Provider: "acme", Kind: "issue", ID: "issue-7"}
	if err := registry.AuthorizeReference(context.Background(), ReferenceAuthorizationRequest{
		WorkspaceID: "workspace-1",
		Purpose:     ReferencePurposeSubmission,
		Reference:   ref,
	}); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if !called {
		t.Fatal("registered provider authorizer was not called")
	}

	if err := registry.Register(fakeProvider{
		descriptor: ProviderDescriptor{Source: "acme_issues_duplicate", Provider: "acme", Kind: "issue"},
		search:     func(context.Context, SearchRequest) ([]Candidate, error) { return nil, nil },
	}); err == nil {
		t.Fatal("duplicate provider/kind registration succeeded")
	}
	unknown := ref
	unknown.Provider = "unknown"
	if err := registry.AuthorizeReference(context.Background(), ReferenceAuthorizationRequest{
		WorkspaceID: "workspace-1",
		Purpose:     ReferencePurposeSubmission,
		Reference:   unknown,
	}); !errors.Is(err, ErrReferenceProviderUnavailable) {
		t.Fatalf("unknown provider error = %v, want unavailable", err)
	}
}

func TestRegistryRejectsProviderWithoutAuthorizer(t *testing.T) {
	registry := NewRegistry()
	err := registry.Register(searchOnlyProvider{descriptor: ProviderDescriptor{
		Source: "plain_issues", Provider: "plain", Kind: "issue",
	}})
	if !errors.Is(err, ErrMissingAuthorizer) {
		t.Fatalf("register error = %v, want missing authorizer", err)
	}
}

func TestServiceSearchDropsCandidateRejectedByProviderAuthorizer(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(fakeProvider{
		descriptor: ProviderDescriptor{Source: "acme_issues", Provider: "acme", Kind: "issue"},
		search: func(context.Context, SearchRequest) ([]Candidate, error) {
			return []Candidate{
				{ID: "allowed", Title: "Allowed", URL: "https://acme.test/allowed", Scope: "tenant"},
				{ID: "blocked", Title: "Blocked", URL: "https://evil.test/blocked", Scope: "tenant"},
			}, nil
		},
		authorize: func(_ context.Context, request ReferenceAuthorizationRequest) error {
			if request.Purpose != ReferencePurposeSearch {
				t.Fatalf("purpose = %q, want search", request.Purpose)
			}
			if request.Reference.ID == "blocked" {
				return errors.New("origin mismatch")
			}
			return nil
		},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	response, err := NewService(registry).Search(context.Background(), SearchRequest{
		WorkspaceID: "workspace-1",
		Query:       "issue",
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(response.Groups[0].Results) != 1 || response.Groups[0].Results[0].ID != "allowed" {
		t.Fatalf("results = %#v, want only provider-authorized candidate", response.Groups[0].Results)
	}
}

func TestServiceSearch_RejectsInvalidWorkspaceAndQuery(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(fakeProvider{
		descriptor: ProviderDescriptor{Source: "test", Provider: "test", Kind: "issue"},
		search: func(context.Context, SearchRequest) ([]Candidate, error) {
			t.Fatal("provider called for invalid request")
			return nil, nil
		},
	}); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	service := NewService(registry)

	tests := []SearchRequest{
		{WorkspaceID: "", Query: "auth"},
		{WorkspaceID: "workspace\x00admin", Query: "auth"},
		{WorkspaceID: "workspace-1", Query: "  "},
		{WorkspaceID: "workspace-1", Query: "auth", ExcludeTaskID: "task\x00other"},
		{WorkspaceID: "workspace-1", Query: "auth\x00admin"},
		{WorkspaceID: "workspace-1", Query: strings.Repeat("界", 201)},
	}
	for _, request := range tests {
		if _, err := service.Search(context.Background(), request); err == nil {
			t.Fatalf("Search(%+v): err = nil, want validation error", request)
		}
	}
}

func TestServiceSearch_ClampsPerSourceLimitAndOutput(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(fakeProvider{
		descriptor: ProviderDescriptor{Source: "issues", Provider: "acme", Kind: "issue"},
		search: func(_ context.Context, request SearchRequest) ([]Candidate, error) {
			if request.Limit != MaxLimit {
				t.Fatalf("provider limit = %d, want %d", request.Limit, MaxLimit)
			}
			candidates := make([]Candidate, 0, MaxLimit+2)
			for i := 0; i < MaxLimit+2; i++ {
				candidates = append(candidates, Candidate{
					ID:    fmt.Sprintf("issue-%d", i),
					Title: fmt.Sprintf("Issue %d", i),
					URL:   fmt.Sprintf("https://issues.example.test/%d", i),
					Scope: "tenant-a",
				})
			}
			return candidates, nil
		},
	}); err != nil {
		t.Fatalf("register provider: %v", err)
	}

	response, err := NewService(registry).Search(context.Background(), SearchRequest{
		WorkspaceID: "workspace-1",
		Query:       "auth",
		Limit:       99,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if got := len(response.Groups[0].Results); got != MaxLimit {
		t.Fatalf("result count = %d, want capped %d", got, MaxLimit)
	}
}

func TestServiceSearch_DropsUnsafeCandidatesAndDeduplicatesRefs(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(fakeProvider{
		descriptor: ProviderDescriptor{Source: "tasks", Provider: "kandev", Kind: "task"},
		search: func(context.Context, SearchRequest) ([]Candidate, error) {
			return []Candidate{
				{ID: "", Title: "Missing ID", URL: "/t/missing", Scope: "workspace-1"},
				{ID: "bad-scope", Title: "Missing scope", URL: "/t/bad-scope"},
				{ID: "bad-title", URL: "/t/bad-title", Scope: "workspace-1"},
				{ID: "bad-url", Title: "Bad URL", URL: "javascript:alert(1)", Scope: "workspace-1"},
				{ID: "task-1", Key: " KAN-1\n", Title: "  Login\n\t failure  ", URL: "/t/task-1", Scope: " workspace-1 "},
				{ID: "task-1", Key: "KAN-1", Title: "Duplicate", URL: "/t/task-1", Scope: "workspace-1"},
			}, nil
		},
	}); err != nil {
		t.Fatalf("register provider: %v", err)
	}

	response, err := NewService(registry).Search(context.Background(), SearchRequest{
		WorkspaceID: "workspace-1",
		Query:       "login",
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	results := response.Groups[0].Results
	if len(results) != 1 {
		t.Fatalf("results = %#v, want one safe unique result", results)
	}
	if results[0].Title != "Login failure" || results[0].Key != "KAN-1" || results[0].Scope != "workspace-1" {
		t.Fatalf("sanitized result = %#v", results[0])
	}
}

func TestServiceSearch_RestrictsInternalRoutesToKandevTasks(t *testing.T) {
	registry := NewRegistry()
	providers := []fakeProvider{
		{
			descriptor: ProviderDescriptor{Source: "plugin_tasks", Provider: "plugin:acme:tracker", Kind: "task"},
			search: func(context.Context, SearchRequest) ([]Candidate, error) {
				return []Candidate{{ID: "spoofed", Title: "Spoofed task", URL: "/t/real-task", Scope: "tenant"}}, nil
			},
		},
		{
			descriptor: ProviderDescriptor{Source: "kandev_tasks", Provider: "kandev", Kind: "task"},
			search: func(context.Context, SearchRequest) ([]Candidate, error) {
				return []Candidate{{ID: "task-1", Title: "Task", URL: "/t/task-1", Scope: "workspace-1"}}, nil
			},
		},
	}
	for _, provider := range providers {
		if err := registry.Register(provider); err != nil {
			t.Fatalf("register %s: %v", provider.descriptor.Source, err)
		}
	}

	response, err := NewService(registry).Search(context.Background(), SearchRequest{
		WorkspaceID: "workspace-1",
		Query:       "task",
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	groups := make(map[string]apiv1.MentionGroup, len(response.Groups))
	for _, group := range response.Groups {
		groups[group.Source] = group
	}
	if len(groups["plugin_tasks"].Results) != 0 {
		t.Fatalf("plugin internal-route results = %#v, want none", groups["plugin_tasks"].Results)
	}
	if len(groups["kandev_tasks"].Results) != 1 || groups["kandev_tasks"].Results[0].URL != "/t/task-1" {
		t.Fatalf("Kandev task results = %#v, want allowed internal route", groups["kandev_tasks"].Results)
	}
}

func TestServiceSearch_ReturnsDeterministicPartialStatuses(t *testing.T) {
	registry := NewRegistry()
	providers := []fakeProvider{
		{
			descriptor: ProviderDescriptor{Source: "success", Provider: "acme", Kind: "issue", Order: 30},
			search: func(context.Context, SearchRequest) ([]Candidate, error) {
				return []Candidate{{ID: "1", Title: "Found", URL: "https://acme.test/1", Scope: "tenant"}}, nil
			},
		},
		{
			descriptor: ProviderDescriptor{Source: "raw-error", Provider: "broken", Kind: "issue", Order: 20},
			search: func(context.Context, SearchRequest) ([]Candidate, error) {
				return nil, errors.New("secret upstream body")
			},
		},
		{
			descriptor: ProviderDescriptor{Source: "auth", Provider: "private", Kind: "ticket", Order: 10},
			search: func(context.Context, SearchRequest) ([]Candidate, error) {
				return nil, NewProviderError(StatusUnauthorized, errors.New("token expired: secret-token"))
			},
		},
	}
	for _, provider := range providers {
		if err := registry.Register(provider); err != nil {
			t.Fatalf("register %s: %v", provider.descriptor.Source, err)
		}
	}

	response, err := NewService(registry).Search(context.Background(), SearchRequest{
		WorkspaceID: "workspace-1",
		Query:       "auth",
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if got := []string{response.Groups[0].Source, response.Groups[1].Source, response.Groups[2].Source}; fmt.Sprint(got) != "[auth raw-error success]" {
		t.Fatalf("group order = %v", got)
	}
	if response.Groups[0].Status != StatusUnauthorized || response.Groups[1].Status != StatusUpstreamError ||
		response.Groups[2].Status != StatusOK {
		t.Fatalf("statuses = %q, %q, %q", response.Groups[0].Status, response.Groups[1].Status, response.Groups[2].Status)
	}
	if response.Groups[0].DisplayName != "private" || response.Groups[0].KindLabel != "Work item" {
		t.Fatalf("generic labels = %#v", response.Groups[0])
	}
}

func TestServiceSearch_ProviderDeadlineBecomesPartialTimeout(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(fakeProvider{
		descriptor: ProviderDescriptor{Source: "slow", Provider: "slow", Kind: "issue"},
		search: func(ctx context.Context, _ SearchRequest) ([]Candidate, error) {
			if _, ok := ctx.Deadline(); !ok {
				return nil, errors.New("provider context missing deadline")
			}
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}); err != nil {
		t.Fatalf("register provider: %v", err)
	}

	response, err := NewService(registry, WithProviderTimeout(10*time.Millisecond)).Search(
		context.Background(),
		SearchRequest{WorkspaceID: "workspace-1", Query: "auth"},
	)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if response.Groups[0].Status != StatusTimeout {
		t.Fatalf("status = %q, want %q", response.Groups[0].Status, StatusTimeout)
	}
}

func TestClassifyProviderErrorTreatsCancellationAsTimeout(t *testing.T) {
	if got := classifyProviderError(context.Canceled); got != StatusTimeout {
		t.Fatalf("status = %q, want %q", got, StatusTimeout)
	}
}

func TestServiceSearch_EnforcesTimeoutWhenProviderIgnoresContext(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	registry := NewRegistry()
	if err := registry.Register(fakeProvider{
		descriptor: ProviderDescriptor{Source: "stuck", Provider: "stuck", Kind: "issue"},
		search: func(context.Context, SearchRequest) ([]Candidate, error) {
			<-release
			return nil, nil
		},
	}); err != nil {
		t.Fatalf("register provider: %v", err)
	}

	type outcome struct {
		response *apiv1.MentionSearchResponse
		err      error
	}
	done := make(chan outcome, 1)
	go func() {
		response, err := NewService(registry, WithProviderTimeout(10*time.Millisecond)).Search(
			context.Background(),
			SearchRequest{WorkspaceID: "workspace-1", Query: "auth"},
		)
		done <- outcome{response: response, err: err}
	}()

	select {
	case result := <-done:
		if result.err != nil {
			t.Fatalf("search: %v", result.err)
		}
		if result.response.Groups[0].Status != StatusTimeout {
			t.Fatalf("status = %q, want %q", result.response.Groups[0].Status, StatusTimeout)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("search remained blocked after provider timeout")
	}
}

func TestServiceSearch_BoundsAbandonedProviderCallsPerSource(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	started := make(chan struct{}, 8)
	registry := NewRegistry()
	for index := 0; index < 4; index++ {
		providerID := fmt.Sprintf("stuck-%d", index)
		if err := registry.Register(fakeProvider{
			descriptor: ProviderDescriptor{Source: providerID, Provider: providerID, Kind: "issue"},
			search: func(context.Context, SearchRequest) ([]Candidate, error) {
				started <- struct{}{}
				<-release
				return nil, nil
			},
		}); err != nil {
			t.Fatalf("register provider: %v", err)
		}
	}
	service := NewService(
		registry,
		WithMaxConcurrency(1),
		WithProviderTimeout(10*time.Millisecond),
	)
	for attempt := 0; attempt < 4; attempt++ {
		response, err := service.Search(
			context.Background(),
			SearchRequest{WorkspaceID: "workspace-1", Query: "auth"},
		)
		if err != nil {
			t.Fatalf("search %d: %v", attempt, err)
		}
		for _, group := range response.Groups {
			if group.Status != StatusTimeout {
				t.Fatalf("search %d group status = %q", attempt, group.Status)
			}
		}
	}
	if got := len(started); got != 4 {
		t.Fatalf("started provider calls = %d, want one quarantined call per source", got)
	}
}

func TestServiceSearch_QuarantinesRepeatedStuckProviderWithoutHidingHealthyResults(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	stuckStarted := make(chan struct{}, 4)
	registry := NewRegistry()
	if err := registry.Register(fakeProvider{
		descriptor: ProviderDescriptor{Source: "stuck", Provider: "stuck", Kind: "issue"},
		search: func(context.Context, SearchRequest) ([]Candidate, error) {
			stuckStarted <- struct{}{}
			<-release
			return nil, nil
		},
	}); err != nil {
		t.Fatalf("register stuck provider: %v", err)
	}
	if err := registry.Register(fakeProvider{
		descriptor: ProviderDescriptor{Source: "healthy", Provider: "healthy", Kind: "issue"},
		search: func(context.Context, SearchRequest) ([]Candidate, error) {
			return []Candidate{{
				ID:    "healthy-1",
				Title: "Healthy result",
				URL:   "https://healthy.example.test/1",
				Scope: "workspace-1",
			}}, nil
		},
	}); err != nil {
		t.Fatalf("register healthy provider: %v", err)
	}

	service := NewService(
		registry,
		WithMaxConcurrency(1),
		WithProviderTimeout(10*time.Millisecond),
	)
	for attempt := 0; attempt < 4; attempt++ {
		response, err := service.Search(
			context.Background(),
			SearchRequest{WorkspaceID: "workspace-1", Query: "auth"},
		)
		if err != nil {
			t.Fatalf("search %d: %v", attempt, err)
		}
		groups := make(map[string]apiv1.MentionGroup, len(response.Groups))
		for _, group := range response.Groups {
			groups[group.Source] = group
		}
		if groups["stuck"].Status != StatusTimeout {
			t.Fatalf("search %d stuck status = %q, want %q", attempt, groups["stuck"].Status, StatusTimeout)
		}
		if groups["healthy"].Status != StatusOK || len(groups["healthy"].Results) != 1 {
			t.Fatalf("search %d healthy group = %#v, want successful result", attempt, groups["healthy"])
		}
	}
	if got := len(stuckStarted); got != 1 {
		t.Fatalf("started stuck provider calls = %d, want quarantined at 1", got)
	}
}

func TestServiceSearch_BoundsConcurrentFanoutAndKeepsDescriptorOrder(t *testing.T) {
	started := make(chan string, 3)
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })

	registry := NewRegistry()
	for index, source := range []string{"c", "a", "b"} {
		source := source
		if err := registry.Register(fakeProvider{
			descriptor: ProviderDescriptor{Source: source, Provider: source, Kind: "issue", Order: 30 - index*10},
			search: func(context.Context, SearchRequest) ([]Candidate, error) {
				started <- source
				<-release
				return nil, nil
			},
		}); err != nil {
			t.Fatalf("register %s: %v", source, err)
		}
	}

	type searchOutcome struct {
		responseSources []string
		err             error
	}
	done := make(chan searchOutcome, 1)
	go func() {
		response, err := NewService(
			registry,
			WithMaxConcurrency(2),
			WithProviderTimeout(time.Second),
		).Search(context.Background(), SearchRequest{WorkspaceID: "workspace-1", Query: "auth"})
		outcome := searchOutcome{err: err}
		if response != nil {
			for _, group := range response.Groups {
				outcome.responseSources = append(outcome.responseSources, group.Source)
			}
		}
		done <- outcome
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(500 * time.Millisecond):
			t.Fatal("fewer than two providers started concurrently")
		}
	}
	select {
	case source := <-started:
		t.Fatalf("provider %q exceeded concurrency bound before release", source)
	case <-time.After(50 * time.Millisecond):
	}

	releaseOnce.Do(func() { close(release) })
	select {
	case outcome := <-done:
		if outcome.err != nil {
			t.Fatalf("search: %v", outcome.err)
		}
		if got := fmt.Sprint(outcome.responseSources); got != "[b a c]" {
			t.Fatalf("group order = %s, want [b a c]", got)
		}
	case <-time.After(time.Second):
		t.Fatal("search did not finish after providers released")
	}
}
