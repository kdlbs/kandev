package routing

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

// fakeProviderRepo satisfies ProviderRepo without sqlite.
type fakeProviderRepo struct {
	cfg              *WorkspaceConfig
	upserts          int
	health           []models.ProviderHealth
	agents           []*models.AgentInstance
	byID             map[string]*models.AgentInstance
	clearedParkedFor []string // workspace ids passed to ClearAllParked
	clearedParkedErr error
}

func (f *fakeProviderRepo) GetWorkspaceRouting(_ context.Context, _ string) (*WorkspaceConfig, error) {
	return f.cfg, nil
}

func (f *fakeProviderRepo) UpsertWorkspaceRouting(_ context.Context, _ string, cfg *WorkspaceConfig) error {
	f.upserts++
	f.cfg = cfg
	return nil
}

func (f *fakeProviderRepo) ListProviderHealth(_ context.Context, _ string) ([]models.ProviderHealth, error) {
	return f.health, nil
}

func (f *fakeProviderRepo) ListAgentInstances(_ context.Context, _ string) ([]*models.AgentInstance, error) {
	return f.agents, nil
}

func (f *fakeProviderRepo) GetAgentInstance(_ context.Context, id string) (*models.AgentInstance, error) {
	return f.byID[id], nil
}

func (f *fakeProviderRepo) ClearAllParkedRoutingForWorkspace(_ context.Context, workspaceID string) error {
	f.clearedParkedFor = append(f.clearedParkedFor, workspaceID)
	return f.clearedParkedErr
}

// fakeRetry stubs the RetryRunner seam.
type fakeRetry struct{ calls int }

func (f *fakeRetry) RetryProvider(_ context.Context, _, _ string) error {
	f.calls++
	return nil
}

func newProviderTest(cfg *WorkspaceConfig, agents []*models.AgentInstance) (*Provider, *fakeProviderRepo) {
	repo := &fakeProviderRepo{
		cfg:    cfg,
		agents: agents,
		byID:   map[string]*models.AgentInstance{},
	}
	for _, a := range agents {
		repo.byID[a.ID] = a
	}
	resolver := NewResolver(&providerResolverAdapter{repo: repo}, nil)
	return NewProvider(repo, nil, resolver, &fakeRetry{}), repo
}

// providerResolverAdapter adapts ProviderRepo to the Resolver's narrower
// Repo interface for the test (avoid needing two stubs).
type providerResolverAdapter struct{ repo *fakeProviderRepo }

func (a *providerResolverAdapter) GetWorkspaceRouting(ctx context.Context, workspaceID string) (*WorkspaceConfig, error) {
	return a.repo.GetWorkspaceRouting(ctx, workspaceID)
}

func (a *providerResolverAdapter) ListProviderHealth(ctx context.Context, workspaceID string) ([]models.ProviderHealth, error) {
	return a.repo.ListProviderHealth(ctx, workspaceID)
}

func TestProvider_PreviewComposesCandidatesAndMissing(t *testing.T) {
	cfg := &WorkspaceConfig{
		Enabled:       true,
		DefaultTier:   TierBalanced,
		ProviderOrder: []ProviderID{"claude-acp", "codex-acp"},
		ProviderProfiles: map[ProviderID]ProviderProfile{
			"claude-acp": {TierMap: TierMap{Balanced: "sonnet"}},
			// codex-acp deliberately has no profile -> missing mapping
		},
	}
	agents := []*models.AgentInstance{{ID: "a1", Name: "Alice", WorkspaceID: "ws-1"}}
	p, _ := newProviderTest(cfg, agents)

	items, err := p.Preview(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	it := items[0]
	if it.PrimaryProviderID != "claude-acp" || it.PrimaryModel != "sonnet" {
		t.Fatalf("primary = (%s,%s)", it.PrimaryProviderID, it.PrimaryModel)
	}
	if len(it.Missing) != 1 {
		t.Fatalf("expected one missing-mapping hint, got %v", it.Missing)
	}
}

func TestProvider_PreviewPrimaryReflectsIntent(t *testing.T) {
	// Single provider, healthy: primary == current.
	cfg := &WorkspaceConfig{
		Enabled:       true,
		DefaultTier:   TierBalanced,
		ProviderOrder: []ProviderID{"claude-acp"},
		ProviderProfiles: map[ProviderID]ProviderProfile{
			"claude-acp": {TierMap: TierMap{Balanced: "sonnet"}},
		},
	}
	agents := []*models.AgentInstance{{ID: "a1", Name: "Alice", WorkspaceID: "ws-1"}}
	p, _ := newProviderTest(cfg, agents)
	items, _ := p.Preview(context.Background(), "ws-1")
	if items[0].PrimaryProviderID != "claude-acp" || items[0].PrimaryModel != "sonnet" {
		t.Errorf("primary = (%s,%s)", items[0].PrimaryProviderID, items[0].PrimaryModel)
	}
	if items[0].CurrentProviderID != "claude-acp" || items[0].CurrentModel != "sonnet" {
		t.Errorf("current = (%s,%s)", items[0].CurrentProviderID, items[0].CurrentModel)
	}
}

func TestProvider_PreviewCurrentDiffersWhenPrimaryDegraded(t *testing.T) {
	cfg := &WorkspaceConfig{
		Enabled:       true,
		DefaultTier:   TierBalanced,
		ProviderOrder: []ProviderID{"claude-acp", "codex-acp"},
		ProviderProfiles: map[ProviderID]ProviderProfile{
			"claude-acp": {TierMap: TierMap{Balanced: "sonnet"}},
			"codex-acp":  {TierMap: TierMap{Balanced: "gpt-5"}},
		},
	}
	agents := []*models.AgentInstance{{ID: "a1", Name: "Alice", WorkspaceID: "ws-1"}}
	p, repo := newProviderTest(cfg, agents)
	future := newTestTime()
	repo.health = []models.ProviderHealth{
		{
			WorkspaceID: "ws-1",
			ProviderID:  "claude-acp",
			Scope:       "provider",
			ScopeValue:  "",
			State:       "degraded",
			ErrorCode:   "rate_limited",
			RetryAt:     &future,
		},
	}
	items, _ := p.Preview(context.Background(), "ws-1")
	it := items[0]
	if it.PrimaryProviderID != "claude-acp" || it.PrimaryModel != "sonnet" {
		t.Errorf("primary = (%s,%s), want (claude-acp,sonnet) — intent preserved",
			it.PrimaryProviderID, it.PrimaryModel)
	}
	if it.CurrentProviderID != "codex-acp" || it.CurrentModel != "gpt-5" {
		t.Errorf("current = (%s,%s), want (codex-acp,gpt-5) — fell back",
			it.CurrentProviderID, it.CurrentModel)
	}
	if !it.Degraded {
		t.Error("expected Degraded=true when primary is skipped")
	}
}

func TestProvider_PreviewCurrentEmptyWhenAllSkipped(t *testing.T) {
	cfg := &WorkspaceConfig{
		Enabled:       true,
		DefaultTier:   TierBalanced,
		ProviderOrder: []ProviderID{"claude-acp"},
		ProviderProfiles: map[ProviderID]ProviderProfile{
			// no tier mapping → skipped as missing_model_mapping
			"claude-acp": {TierMap: TierMap{}},
		},
	}
	agents := []*models.AgentInstance{{ID: "a1", Name: "Alice", WorkspaceID: "ws-1"}}
	p, _ := newProviderTest(cfg, agents)
	items, _ := p.Preview(context.Background(), "ws-1")
	it := items[0]
	if it.PrimaryProviderID != "claude-acp" {
		t.Errorf("primary provider lost; got %q", it.PrimaryProviderID)
	}
	if it.PrimaryModel != "" {
		t.Errorf("primary model = %q, want empty (mapping missing)", it.PrimaryModel)
	}
	if it.CurrentProviderID != "" || it.CurrentModel != "" {
		t.Errorf("current = (%s,%s), want empty when all skipped",
			it.CurrentProviderID, it.CurrentModel)
	}
}

func newTestTime() time.Time {
	return time.Now().UTC().Add(10 * time.Minute)
}

func TestProvider_PreviewAgentRoundTrips(t *testing.T) {
	cfg := &WorkspaceConfig{
		Enabled:       true,
		DefaultTier:   TierBalanced,
		ProviderOrder: []ProviderID{"claude-acp"},
		ProviderProfiles: map[ProviderID]ProviderProfile{
			"claude-acp": {TierMap: TierMap{Balanced: "sonnet"}},
		},
	}
	agents := []*models.AgentInstance{{ID: "a1", Name: "Alice", WorkspaceID: "ws-1"}}
	p, _ := newProviderTest(cfg, agents)

	got, err := p.PreviewAgent(context.Background(), "a1")
	if err != nil {
		t.Fatalf("preview agent: %v", err)
	}
	if got == nil || got.AgentID != "a1" {
		t.Fatalf("got %+v", got)
	}
}

func TestProvider_UpdateRejectsInvalid(t *testing.T) {
	cfg := &WorkspaceConfig{}
	p, _ := newProviderTest(cfg, nil)
	bad := WorkspaceConfig{
		Enabled:       true,
		DefaultTier:   TierBalanced,
		ProviderOrder: []ProviderID{},
	}
	err := p.UpdateConfig(context.Background(), "ws-1", bad)
	if err == nil {
		t.Fatalf("expected validation error, got nil")
	}
}

func TestProvider_UpdatePersistsValid(t *testing.T) {
	cfg := &WorkspaceConfig{}
	p, repo := newProviderTest(cfg, nil)
	good := WorkspaceConfig{
		Enabled:     false,
		DefaultTier: TierBalanced,
	}
	if err := p.UpdateConfig(context.Background(), "ws-1", good); err != nil {
		t.Fatalf("update: %v", err)
	}
	if repo.upserts != 1 {
		t.Fatalf("expected upsert, got %d", repo.upserts)
	}
}

func TestProvider_UpdateClearsParkedOnDisable(t *testing.T) {
	prev := &WorkspaceConfig{
		Enabled:       true,
		DefaultTier:   TierBalanced,
		ProviderOrder: []ProviderID{"claude-acp"},
		ProviderProfiles: map[ProviderID]ProviderProfile{
			"claude-acp": {TierMap: TierMap{Balanced: "sonnet"}},
		},
	}
	p, repo := newProviderTest(prev, nil)
	next := WorkspaceConfig{Enabled: false, DefaultTier: TierBalanced}
	if err := p.UpdateConfig(context.Background(), "ws-1", next); err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(repo.clearedParkedFor) != 1 || repo.clearedParkedFor[0] != "ws-1" {
		t.Fatalf("expected ClearAllParkedRoutingForWorkspace(ws-1), got %v",
			repo.clearedParkedFor)
	}
}

func TestProvider_UpdateClearsParkedOnMaterialChange(t *testing.T) {
	prev := &WorkspaceConfig{
		Enabled:       true,
		DefaultTier:   TierBalanced,
		ProviderOrder: []ProviderID{"claude-acp"},
		ProviderProfiles: map[ProviderID]ProviderProfile{
			"claude-acp": {TierMap: TierMap{Balanced: "sonnet"}},
		},
	}
	p, repo := newProviderTest(prev, nil)
	// Material change: default tier flips, tier map updated. A run
	// parked under blocked_provider_action_required for a missing
	// frontier mapping should now unblock.
	next := WorkspaceConfig{
		Enabled:       true,
		DefaultTier:   TierFrontier,
		ProviderOrder: []ProviderID{"claude-acp"},
		ProviderProfiles: map[ProviderID]ProviderProfile{
			"claude-acp": {TierMap: TierMap{Frontier: "opus"}},
		},
	}
	if err := p.UpdateConfig(context.Background(), "ws-1", next); err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(repo.clearedParkedFor) != 1 || repo.clearedParkedFor[0] != "ws-1" {
		t.Fatalf("expected ClearAllParkedRoutingForWorkspace(ws-1), got %v",
			repo.clearedParkedFor)
	}
}

func TestProvider_UpdateDoesNotClearWhenConfigUnchanged(t *testing.T) {
	prev := &WorkspaceConfig{
		Enabled:       true,
		DefaultTier:   TierBalanced,
		ProviderOrder: []ProviderID{"claude-acp"},
		ProviderProfiles: map[ProviderID]ProviderProfile{
			"claude-acp": {TierMap: TierMap{Balanced: "sonnet"}},
		},
	}
	p, repo := newProviderTest(prev, nil)
	next := WorkspaceConfig{
		Enabled:       true,
		DefaultTier:   TierBalanced,
		ProviderOrder: []ProviderID{"claude-acp"},
		ProviderProfiles: map[ProviderID]ProviderProfile{
			"claude-acp": {TierMap: TierMap{Balanced: "sonnet"}},
		},
	}
	if err := p.UpdateConfig(context.Background(), "ws-1", next); err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(repo.clearedParkedFor) != 0 {
		t.Fatalf("expected no clear on identical config, got %v",
			repo.clearedParkedFor)
	}
}

func TestProvider_UpdateDoesNotClearWhenStartingDisabled(t *testing.T) {
	prev := &WorkspaceConfig{Enabled: false, DefaultTier: TierBalanced}
	p, repo := newProviderTest(prev, nil)
	next := WorkspaceConfig{Enabled: false, DefaultTier: TierBalanced}
	if err := p.UpdateConfig(context.Background(), "ws-1", next); err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(repo.clearedParkedFor) != 0 {
		t.Fatalf("expected no clear on disabled→disabled, got %v",
			repo.clearedParkedFor)
	}
}

func TestProvider_RetryRoutesThrough(t *testing.T) {
	cfg := &WorkspaceConfig{}
	p, _ := newProviderTest(cfg, nil)
	status, _, err := p.Retry(context.Background(), "ws-1", "claude-acp")
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if status != "retrying" {
		t.Fatalf("status = %q", status)
	}
}
