package sentry

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

type mentionSearchCall struct {
	filter SearchFilter
	cursor string
	limit  int
}

type mentionClient struct {
	mu                 sync.Mutex
	organizationHook   func(context.Context) error
	searchHook         func(context.Context) error
	organizations      []SentryOrganization
	organizationsMore  bool
	organizationsErr   error
	issuesByOrg        map[string][]SentryIssue
	searchErrByOrg     map[string]error
	organizationLimits []int
	searches           []mentionSearchCall
}

func (c *mentionClient) TestAuth(context.Context) (*TestConnectionResult, error) {
	return &TestConnectionResult{OK: true}, nil
}

func (c *mentionClient) ListOrganizations(ctx context.Context) ([]SentryOrganization, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]SentryOrganization(nil), c.organizations...), c.organizationsErr
}

func (c *mentionClient) ListOrganizationsLimited(
	ctx context.Context,
	limit int,
) ([]SentryOrganization, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if c.organizationHook != nil {
		if err := c.organizationHook(ctx); err != nil {
			return nil, false, err
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.organizationLimits = append(c.organizationLimits, limit)
	if c.organizationsErr != nil {
		return nil, false, c.organizationsErr
	}
	count := min(len(c.organizations), limit)
	return append([]SentryOrganization(nil), c.organizations[:count]...),
		c.organizationsMore || len(c.organizations) > limit,
		nil
}

func (c *mentionClient) ListProjects(context.Context) ([]SentryProject, error) {
	return nil, nil
}

func (c *mentionClient) SearchIssues(
	ctx context.Context,
	filter SearchFilter,
	cursor string,
) (*SearchResult, error) {
	return c.SearchIssuesLimited(ctx, filter, cursor, 100)
}

func (c *mentionClient) SearchIssuesLimited(
	ctx context.Context,
	filter SearchFilter,
	cursor string,
	limit int,
) (*SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.searchHook != nil {
		if err := c.searchHook(ctx); err != nil {
			return nil, err
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.searches = append(c.searches, mentionSearchCall{filter: filter, cursor: cursor, limit: limit})
	if err := c.searchErrByOrg[filter.OrgSlug]; err != nil {
		return nil, err
	}
	issues := c.issuesByOrg[filter.OrgSlug]
	return &SearchResult{Issues: append([]SentryIssue(nil), issues...), IsLast: true}, nil
}

type mentionConcurrencyProbe struct {
	mu          sync.Mutex
	started     chan struct{}
	release     chan struct{}
	releaseOnce sync.Once
	active      int
	maximum     int
	total       int
}

func newMentionConcurrencyProbe() *mentionConcurrencyProbe {
	return &mentionConcurrencyProbe{
		started: make(chan struct{}, 16),
		release: make(chan struct{}),
	}
}

func (p *mentionConcurrencyProbe) enter(ctx context.Context) error {
	p.mu.Lock()
	p.active++
	p.total++
	p.maximum = max(p.maximum, p.active)
	p.mu.Unlock()
	p.started <- struct{}{}
	defer func() {
		p.mu.Lock()
		p.active--
		p.mu.Unlock()
	}()
	select {
	case <-p.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *mentionConcurrencyProbe) unblock() {
	p.releaseOnce.Do(func() { close(p.release) })
}

func (p *mentionConcurrencyProbe) counts() (int, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.maximum, p.total
}

func waitForBlockedMentionCalls(t *testing.T, probe *mentionConcurrencyProbe, want int) {
	t.Helper()
	for index := 0; index < want; index++ {
		select {
		case <-probe.started:
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out after %d of %d concurrent calls", index, want)
		}
	}
	select {
	case <-probe.started:
		t.Fatalf("more than %d calls ran concurrently", want)
	case <-time.After(100 * time.Millisecond):
	}
}

func (c *mentionClient) GetIssue(context.Context, string) (*SentryIssue, error) {
	return nil, nil
}

func (c *mentionClient) snapshotCalls() ([]int, []mentionSearchCall) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]int(nil), c.organizationLimits...), append([]mentionSearchCall(nil), c.searches...)
}

type mentionServiceFixture struct {
	service      *Service
	store        *Store
	secrets      *fakeSecretStore
	clients      map[string]*mentionClient
	factoryMu    sync.Mutex
	factoryCalls int
}

func newMentionServiceFixture(t *testing.T) *mentionServiceFixture {
	t.Helper()
	fixture := &mentionServiceFixture{
		store:   newTestStore(t),
		secrets: newFakeSecretStore(),
		clients: make(map[string]*mentionClient),
	}
	fixture.service = NewService(fixture.store, fixture.secrets, func(cfg *SentryConfig, _ string) Client {
		fixture.factoryMu.Lock()
		defer fixture.factoryMu.Unlock()
		fixture.factoryCalls++
		return fixture.clients[cfg.ID]
	}, logger.Default())
	return fixture
}

func (f *mentionServiceFixture) seedInstance(
	t *testing.T,
	workspaceID, id, name, origin string,
	client *mentionClient,
) {
	t.Helper()
	cfg := &SentryConfig{
		ID: id, WorkspaceID: workspaceID, Name: name,
		AuthMethod: AuthMethodAuthToken, URL: origin,
	}
	if err := f.store.CreateInstance(context.Background(), cfg); err != nil {
		t.Fatalf("seed instance: %v", err)
	}
	if err := f.secrets.Set(
		context.Background(),
		secretKeyForInstance(id),
		"Sentry auth token",
		"token-"+id,
	); err != nil {
		t.Fatalf("seed secret: %v", err)
	}
	f.clients[id] = client
}

func TestServiceSearchMentionIssuesSearchesEveryInstanceAndOrganization(t *testing.T) {
	fixture := newMentionServiceFixture(t)
	clientA := &mentionClient{
		organizations: []SentryOrganization{{Slug: "acme"}, {Slug: "globex"}},
		issuesByOrg: map[string][]SentryIssue{
			"acme": {{
				ID: "42", ShortID: "APP-1", Title: "Panic in auth",
				Permalink: "https://sentry.example/organizations/acme/issues/42/",
			}},
			"globex": {{
				ID: "43", ShortID: "WEB-2", Title: "Timeout",
				Permalink: "https://sentry.example/issues/43/",
			}},
		},
	}
	clientB := &mentionClient{
		organizations: []SentryOrganization{{Slug: "internal"}},
		issuesByOrg: map[string][]SentryIssue{
			"internal": {{
				ID: "42", ShortID: "OPS-1", Title: "Self-hosted panic",
				Permalink: "http://self-hosted.example:9000/issues/42/",
			}},
		},
	}
	fixture.seedInstance(t, "workspace-1", "instance-a", "A SaaS", "https://sentry.example", clientA)
	fixture.seedInstance(t, "workspace-1", "instance-b", "B Self-hosted", "http://self-hosted.example:9000", clientB)
	fixture.seedInstance(t, "workspace-2", "instance-other", "Other workspace", "https://other.example", &mentionClient{})

	issues, err := fixture.service.SearchMentionIssues(
		context.Background(),
		"workspace-1",
		`is:resolved "panic" \ path`,
		3,
	)
	if err != nil {
		t.Fatalf("search mention issues: %v", err)
	}
	if len(issues) != 3 {
		t.Fatalf("issues = %#v, want three bounded results", issues)
	}
	if issues[0] != (MentionIssue{
		InstanceID: "instance-a", InstanceOrigin: "https://sentry.example", OrganizationSlug: "acme",
		ID: "42", ShortID: "APP-1", Title: "Panic in auth",
		URL: "https://sentry.example/organizations/acme/issues/42/",
	}) {
		t.Fatalf("first issue = %#v", issues[0])
	}
	if issues[2].ID != "42" || issues[2].InstanceID != "instance-b" ||
		issues[2].InstanceOrigin != "http://self-hosted.example:9000" {
		t.Fatalf("self-hosted identity projection = %#v", issues[2])
	}

	const wantLiteral = `"is:resolved \"panic\" \\ path"`
	for name, client := range map[string]*mentionClient{"A": clientA, "B": clientB} {
		organizationLimits, searches := client.snapshotCalls()
		if len(organizationLimits) != 1 || organizationLimits[0] != 8 {
			t.Fatalf("client %s organization limits = %#v, want one bounded discovery of 8", name, organizationLimits)
		}
		if len(searches) != len(client.organizations) {
			t.Fatalf("client %s searches = %#v, want every organization", name, searches)
		}
		for _, search := range searches {
			if search.filter.Query != wantLiteral || search.cursor != "" || search.limit != 3 {
				t.Fatalf("client %s search = %#v, want literal first-page limit 3", name, search)
			}
		}
	}
}

func TestServiceSearchMentionIssuesBoundsConcurrencyWithoutOmittingScopes(t *testing.T) {
	t.Run("instance discovery", func(t *testing.T) {
		fixture := newMentionServiceFixture(t)
		probe := newMentionConcurrencyProbe()
		t.Cleanup(probe.unblock)
		for index := 0; index < maxMentionInstances; index++ {
			client := &mentionClient{
				organizationHook: probe.enter,
				organizations:    []SentryOrganization{{Slug: "acme"}},
			}
			fixture.seedInstance(
				t,
				"workspace-1",
				fmt.Sprintf("instance-%d", index),
				fmt.Sprintf("Instance %02d", index),
				fmt.Sprintf("https://sentry-%d.example", index),
				client,
			)
		}

		done := make(chan error, 1)
		go func() {
			_, err := fixture.service.SearchMentionIssues(context.Background(), "workspace-1", "panic", 5)
			done <- err
		}()
		waitForBlockedMentionCalls(t, probe, maxMentionSearchConcurrency)
		probe.unblock()
		if err := <-done; err != nil {
			t.Fatalf("search mention issues: %v", err)
		}
		maximum, total := probe.counts()
		if maximum != maxMentionSearchConcurrency || total != maxMentionInstances {
			t.Fatalf("discovery concurrency max/total = %d/%d, want %d/%d",
				maximum, total, maxMentionSearchConcurrency, maxMentionInstances)
		}
	})

	t.Run("organization search", func(t *testing.T) {
		fixture := newMentionServiceFixture(t)
		probe := newMentionConcurrencyProbe()
		t.Cleanup(probe.unblock)
		organizations := make([]SentryOrganization, maxMentionOrganizations)
		for index := range organizations {
			organizations[index].Slug = fmt.Sprintf("org-%d", index)
		}
		client := &mentionClient{
			searchHook:    probe.enter,
			organizations: organizations,
		}
		fixture.seedInstance(t, "workspace-1", "instance-a", "A", "https://sentry.example", client)

		done := make(chan error, 1)
		go func() {
			_, err := fixture.service.SearchMentionIssues(context.Background(), "workspace-1", "panic", 5)
			done <- err
		}()
		waitForBlockedMentionCalls(t, probe, maxMentionSearchConcurrency)
		probe.unblock()
		if err := <-done; err != nil {
			t.Fatalf("search mention issues: %v", err)
		}
		maximum, total := probe.counts()
		if maximum != maxMentionSearchConcurrency || total != maxMentionOrganizations {
			t.Fatalf("search concurrency max/total = %d/%d, want %d/%d",
				maximum, total, maxMentionSearchConcurrency, maxMentionOrganizations)
		}
	})
}

func TestServiceSearchMentionIssuesRejectsScopeOverflowWithoutOmission(t *testing.T) {
	t.Run("instances", func(t *testing.T) {
		fixture := newMentionServiceFixture(t)
		for index := 0; index < 9; index++ {
			fixture.seedInstance(
				t,
				"workspace-1",
				fmt.Sprintf("instance-%d", index),
				fmt.Sprintf("Instance %02d", index),
				fmt.Sprintf("https://sentry-%d.example", index),
				&mentionClient{},
			)
		}
		_, err := fixture.service.SearchMentionIssues(context.Background(), "workspace-1", "panic", 5)
		if err == nil || !strings.Contains(err.Error(), "scope") {
			t.Fatalf("error = %v, want explicit scope overflow", err)
		}
		if fixture.factoryCalls != 0 {
			t.Fatalf("factory called %d times before instance bound rejection", fixture.factoryCalls)
		}
	})

	t.Run("organizations", func(t *testing.T) {
		fixture := newMentionServiceFixture(t)
		organizations := make([]SentryOrganization, 8)
		for index := range organizations {
			organizations[index].Slug = fmt.Sprintf("org-%d", index)
		}
		client := &mentionClient{organizations: organizations, organizationsMore: true}
		fixture.seedInstance(t, "workspace-1", "instance-a", "A", "https://sentry.example", client)

		_, err := fixture.service.SearchMentionIssues(context.Background(), "workspace-1", "panic", 5)
		if err == nil || !strings.Contains(err.Error(), "scope") {
			t.Fatalf("error = %v, want explicit organization scope overflow", err)
		}
		organizationLimits, searches := client.snapshotCalls()
		if len(organizationLimits) != 1 || organizationLimits[0] != 8 || len(searches) != 0 {
			t.Fatalf("organization limits = %#v, searches = %#v; want stop before omission", organizationLimits, searches)
		}
	})
}

func TestServiceSearchMentionIssuesRejectsBlankWorkspaceAndPreservesCancellation(t *testing.T) {
	fixture := newMentionServiceFixture(t)
	if _, err := fixture.service.SearchMentionIssues(context.Background(), " \t", "panic", 5); err == nil ||
		!strings.Contains(err.Error(), "workspace") {
		t.Fatalf("blank workspace error = %v", err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fixture.service.SearchMentionIssues(cancelled, "workspace-1", "panic", 5); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v, want context canceled", err)
	}

	probe := newMentionConcurrencyProbe()
	t.Cleanup(probe.unblock)
	fixture.seedInstance(t, "workspace-1", "instance-a", "A", "https://sentry.example", &mentionClient{
		organizationHook: probe.enter,
	})
	inFlight, cancelInFlight := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := fixture.service.SearchMentionIssues(inFlight, "workspace-1", "panic", 5)
		done <- err
	}()
	select {
	case <-probe.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for in-flight organization discovery")
	}
	cancelInFlight()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("in-flight cancellation error = %v, want context canceled", err)
	}
}

func TestServiceSearchMentionIssuesHandlesMissingConfigAndRejectsUnsafeOrigin(t *testing.T) {
	fixture := newMentionServiceFixture(t)
	if _, err := fixture.service.SearchMentionIssues(
		context.Background(), "workspace-1", "panic", 5,
	); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("missing config error = %v, want not configured", err)
	}

	fixture.seedInstance(
		t,
		"workspace-1",
		"instance-a",
		"A",
		"https://user:secret@sentry.example",
		&mentionClient{},
	)
	if _, err := fixture.service.SearchMentionIssues(
		context.Background(), "workspace-1", "panic", 5,
	); !errors.Is(err, ErrMentionInvalidOrigin) {
		t.Fatalf("unsafe origin error = %v, want invalid origin", err)
	}
	if fixture.factoryCalls != 0 {
		t.Fatalf("factory called %d times for unsafe origin", fixture.factoryCalls)
	}
}

func TestServiceMentionInstanceForWorkspaceReturnsOwnedCanonicalOrigin(t *testing.T) {
	fixture := newMentionServiceFixture(t)
	fixture.seedInstance(t, "workspace-1", "instance-a", "A", "https://sentry.example/", &mentionClient{})

	instance, err := fixture.service.MentionInstanceForWorkspace(context.Background(), "workspace-1", "instance-a")
	if err != nil {
		t.Fatalf("mention instance: %v", err)
	}
	want := &MentionInstance{ID: "instance-a", WorkspaceID: "workspace-1", Origin: "https://sentry.example"}
	if !reflect.DeepEqual(instance, want) {
		t.Fatalf("instance = %#v, want %#v", instance, want)
	}
	if _, err := fixture.service.MentionInstanceForWorkspace(context.Background(), "workspace-2", "instance-a"); !errors.Is(err, ErrInstanceNotFound) {
		t.Fatalf("cross-workspace error = %v, want instance not found", err)
	}
}
