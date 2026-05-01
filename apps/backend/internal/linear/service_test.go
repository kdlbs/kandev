package linear

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

// fakeSecretStore is an in-memory SecretStore for tests.
type fakeSecretStore struct {
	mu      sync.Mutex
	secrets map[string]string
}

func newFakeSecretStore() *fakeSecretStore {
	return &fakeSecretStore{secrets: map[string]string{}}
}

func (f *fakeSecretStore) Reveal(_ context.Context, id string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.secrets[id]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}

func (f *fakeSecretStore) Set(_ context.Context, id, _, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.secrets[id] = value
	return nil
}

func (f *fakeSecretStore) Delete(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.secrets, id)
	return nil
}

func (f *fakeSecretStore) Exists(_ context.Context, id string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.secrets[id]
	return ok, nil
}

// fakeClient is an in-memory Client for verifying service plumbing.
type fakeClient struct {
	testAuthFn     func() (*TestConnectionResult, error)
	getIssueFn     func(id string) (*LinearIssue, error)
	setStateFn     func(issueID, stateID string) error
	listTeamsFn    func() ([]LinearTeam, error)
	listStatesFn   func(teamKey string) ([]LinearWorkflowState, error)
	transitionLog  []string
	searchIssuesFn func(filter SearchFilter, pageToken string, max int) (*SearchResult, error)
}

func (c *fakeClient) TestAuth(_ context.Context) (*TestConnectionResult, error) {
	if c.testAuthFn != nil {
		return c.testAuthFn()
	}
	return &TestConnectionResult{OK: true}, nil
}

func (c *fakeClient) GetIssue(_ context.Context, id string) (*LinearIssue, error) {
	if c.getIssueFn != nil {
		return c.getIssueFn(id)
	}
	return &LinearIssue{Identifier: id, ID: id}, nil
}

func (c *fakeClient) ListStates(_ context.Context, teamKey string) ([]LinearWorkflowState, error) {
	if c.listStatesFn != nil {
		return c.listStatesFn(teamKey)
	}
	return nil, nil
}

func (c *fakeClient) SetIssueState(_ context.Context, issueID, stateID string) error {
	c.transitionLog = append(c.transitionLog, issueID+":"+stateID)
	if c.setStateFn != nil {
		return c.setStateFn(issueID, stateID)
	}
	return nil
}

func (c *fakeClient) ListTeams(_ context.Context) ([]LinearTeam, error) {
	if c.listTeamsFn != nil {
		return c.listTeamsFn()
	}
	return nil, nil
}

func (c *fakeClient) SearchIssues(_ context.Context, filter SearchFilter, pageToken string, max int) (*SearchResult, error) {
	if c.searchIssuesFn != nil {
		return c.searchIssuesFn(filter, pageToken, max)
	}
	return &SearchResult{}, nil
}

type svcFixture struct {
	svc        *Service
	store      *Store
	secrets    *fakeSecretStore
	client     *fakeClient
	factoryHit atomic.Int32
	probed     chan string
}

func newSvcFixture(t *testing.T) *svcFixture {
	t.Helper()
	f := &svcFixture{
		store:   newTestStore(t),
		secrets: newFakeSecretStore(),
		client:  &fakeClient{},
		probed:  make(chan string, 8),
	}
	f.svc = NewService(f.store, f.secrets, func(_ *LinearConfig, _ string) Client {
		f.factoryHit.Add(1)
		return f.client
	}, logger.Default())
	f.svc.SetProbeHook(func(workspaceID string) {
		select {
		case f.probed <- workspaceID:
		default:
		}
	})
	return f
}

// waitForAuthProbe blocks until the async probe has fired.
func waitForAuthProbe(t *testing.T, f *svcFixture, workspaceID string) *LinearConfig {
	t.Helper()
	for {
		select {
		case got := <-f.probed:
			if got != workspaceID {
				continue
			}
			cfg, err := f.svc.GetConfig(context.Background(), workspaceID)
			if err != nil {
				t.Fatalf("get config after probe: %v", err)
			}
			return cfg
		case <-time.After(2 * time.Second):
			t.Fatalf("async probe hook did not fire for %q within 2s", workspaceID)
			return nil
		}
	}
}

func TestService_SetConfig_UpsertsAndStoresSecret(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()

	cfg, err := f.svc.SetConfig(ctx, &SetConfigRequest{
		WorkspaceID:    "ws-1",
		AuthMethod:     AuthMethodAPIKey,
		DefaultTeamKey: "ENG",
		Secret:         "lin_api_xyz",
	})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if cfg.DefaultTeamKey != "ENG" {
		t.Errorf("team key not stored: %q", cfg.DefaultTeamKey)
	}
	if !cfg.HasSecret {
		t.Error("expected HasSecret=true")
	}
	if got, _ := f.secrets.Reveal(ctx, SecretKeyForWorkspace("ws-1")); got != "lin_api_xyz" {
		t.Errorf("secret stored = %q", got)
	}
}

func TestService_SetConfig_ProbesAuthImmediately(t *testing.T) {
	f := newSvcFixture(t)
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		return &TestConnectionResult{OK: true, DisplayName: "Alice", OrgSlug: "acme"}, nil
	}
	if _, err := f.svc.SetConfig(context.Background(), &SetConfigRequest{
		WorkspaceID: "ws-1", AuthMethod: AuthMethodAPIKey, Secret: "tok",
	}); err != nil {
		t.Fatalf("set: %v", err)
	}
	cfg := waitForAuthProbe(t, f, "ws-1")
	if !cfg.LastOk {
		t.Errorf("expected LastOk=true after async probe, got %+v", cfg)
	}
	if cfg.OrgSlug != "acme" {
		t.Errorf("expected org_slug captured, got %q", cfg.OrgSlug)
	}
}

func TestService_SetConfig_PersistsProbeFailure(t *testing.T) {
	f := newSvcFixture(t)
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		return &TestConnectionResult{OK: false, Error: "401 unauthorized"}, nil
	}
	if _, err := f.svc.SetConfig(context.Background(), &SetConfigRequest{
		WorkspaceID: "ws-1", AuthMethod: AuthMethodAPIKey, Secret: "bad",
	}); err != nil {
		t.Fatalf("set: %v", err)
	}
	cfg := waitForAuthProbe(t, f, "ws-1")
	if cfg.LastOk {
		t.Error("expected LastOk=false after failed probe")
	}
	if cfg.LastError != "401 unauthorized" {
		t.Errorf("expected probe error preserved, got %q", cfg.LastError)
	}
}

func TestService_SetConfig_EmptySecret_KeepsExisting(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	if _, err := f.svc.SetConfig(ctx, &SetConfigRequest{
		WorkspaceID: "ws-1", AuthMethod: AuthMethodAPIKey, Secret: "first",
	}); err != nil {
		t.Fatalf("initial: %v", err)
	}
	if _, err := f.svc.SetConfig(ctx, &SetConfigRequest{
		WorkspaceID: "ws-1", AuthMethod: AuthMethodAPIKey, DefaultTeamKey: "MOB",
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got, _ := f.secrets.Reveal(ctx, SecretKeyForWorkspace("ws-1")); got != "first" {
		t.Errorf("secret should be preserved, got %q", got)
	}
}

func TestService_SetConfig_InvalidatesClientCache(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	_, _ = f.svc.SetConfig(ctx, &SetConfigRequest{
		WorkspaceID: "ws-1", AuthMethod: AuthMethodAPIKey, Secret: "t",
	})
	waitForAuthProbe(t, f, "ws-1")
	if _, err := f.svc.GetIssue(ctx, "ws-1", "A-1"); err != nil {
		t.Fatalf("get1: %v", err)
	}
	hits := f.factoryHit.Load()
	if _, err := f.svc.GetIssue(ctx, "ws-1", "A-2"); err != nil {
		t.Fatalf("get2: %v", err)
	}
	if got := f.factoryHit.Load(); got != hits {
		t.Errorf("factory should be cached, hits %d→%d", hits, got)
	}
	_, _ = f.svc.SetConfig(ctx, &SetConfigRequest{
		WorkspaceID: "ws-1", AuthMethod: AuthMethodAPIKey, Secret: "t2",
	})
	waitForAuthProbe(t, f, "ws-1")
	if _, err := f.svc.GetIssue(ctx, "ws-1", "A-3"); err != nil {
		t.Fatalf("get3: %v", err)
	}
	if got := f.factoryHit.Load(); got <= hits {
		t.Errorf("factory should rebuild after config change, hits=%d", got)
	}
}

func TestService_Validation(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	cases := []struct {
		name string
		req  SetConfigRequest
	}{
		{"missing ws", SetConfigRequest{AuthMethod: AuthMethodAPIKey}},
		{"bad auth", SetConfigRequest{WorkspaceID: "w", AuthMethod: "bogus"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := f.svc.SetConfig(ctx, &tc.req); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestService_Validation_DefaultsAuthMethod(t *testing.T) {
	f := newSvcFixture(t)
	// Empty AuthMethod is filled with AuthMethodAPIKey rather than rejected.
	if _, err := f.svc.SetConfig(context.Background(), &SetConfigRequest{
		WorkspaceID: "ws-1", Secret: "tok",
	}); err != nil {
		t.Fatalf("expected default auth method, got error: %v", err)
	}
}

func TestService_TestConnection_InlineSecret(t *testing.T) {
	f := newSvcFixture(t)
	called := false
	f.client.testAuthFn = func() (*TestConnectionResult, error) {
		called = true
		return &TestConnectionResult{OK: true, DisplayName: "Alice"}, nil
	}
	res, err := f.svc.TestConnection(context.Background(), &SetConfigRequest{
		WorkspaceID: "ws-1", AuthMethod: AuthMethodAPIKey, Secret: "inline",
	})
	if err != nil || !called {
		t.Fatalf("called=%v err=%v", called, err)
	}
	if !res.OK || res.DisplayName != "Alice" {
		t.Errorf("result: %+v", res)
	}
}

func TestService_TestConnection_NoStoredSecret_ReturnsFailure(t *testing.T) {
	f := newSvcFixture(t)
	res, err := f.svc.TestConnection(context.Background(), &SetConfigRequest{
		WorkspaceID: "ws-nope", AuthMethod: AuthMethodAPIKey,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.OK {
		t.Error("expected OK=false when no secret stored")
	}
}

func TestService_GetIssue_NotConfigured(t *testing.T) {
	f := newSvcFixture(t)
	_, err := f.svc.GetIssue(context.Background(), "missing", "ENG-1")
	if !errors.Is(err, ErrNotConfigured) {
		t.Errorf("expected ErrNotConfigured, got %v", err)
	}
}

func TestService_DeleteConfig_RemovesSecretAndCache(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	_, _ = f.svc.SetConfig(ctx, &SetConfigRequest{
		WorkspaceID: "ws-1", AuthMethod: AuthMethodAPIKey, Secret: "t",
	})
	waitForAuthProbe(t, f, "ws-1")
	if err := f.svc.DeleteConfig(ctx, "ws-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if exists, _ := f.secrets.Exists(ctx, SecretKeyForWorkspace("ws-1")); exists {
		t.Error("expected secret removed")
	}
	cfg, _ := f.svc.GetConfig(ctx, "ws-1")
	if cfg != nil {
		t.Errorf("expected config gone, got %+v", cfg)
	}
}

func TestService_SetIssueState_Forwards(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	_, _ = f.svc.SetConfig(ctx, &SetConfigRequest{
		WorkspaceID: "ws-1", AuthMethod: AuthMethodAPIKey, Secret: "t",
	})
	waitForAuthProbe(t, f, "ws-1")
	if err := f.svc.SetIssueState(ctx, "ws-1", "ENG-1", "state-id"); err != nil {
		t.Fatalf("set state: %v", err)
	}
	if len(f.client.transitionLog) != 1 || f.client.transitionLog[0] != "ENG-1:state-id" {
		t.Errorf("transition log: %v", f.client.transitionLog)
	}
}
