package routing

import (
	"context"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/office/models"
)

// PreviewItem mirrors the dashboard's AgentRoutePreview shape but lives
// in the routing package so the concrete Provider can produce it without
// importing dashboard. Callers map this to whatever DTO they expose.
//
// PrimaryProviderID/PrimaryModel reflect the configured *intent* — the
// first entry in the effective provider order — even when that provider
// is currently degraded or missing a tier mapping. CurrentProviderID/
// CurrentModel reflect what the next launch would actually pick (first
// eligible candidate); when not degraded these equal the primary.
// When every provider is skipped, both Current fields are empty.
type PreviewItem struct {
	AgentID           string
	AgentName         string
	TierSource        string
	EffectiveTier     string
	PrimaryProviderID string
	PrimaryModel      string
	CurrentProviderID string
	CurrentModel      string
	FallbackChain     []PreviewProviderModel
	Missing           []string
	Degraded          bool
}

// PreviewProviderModel is one (provider, model, tier) triple used in a
// PreviewItem.FallbackChain.
type PreviewProviderModel struct {
	ProviderID string
	Model      string
	Tier       string
}

// ProviderRepo is the narrow interface the routing Provider needs over
// the office sqlite repo. Defined here so callers wire the concrete
// repo via an adapter or directly (the office sqlite Repository
// already implements every method).
type ProviderRepo interface {
	GetWorkspaceRouting(ctx context.Context, workspaceID string) (*WorkspaceConfig, error)
	UpsertWorkspaceRouting(ctx context.Context, workspaceID string, cfg *WorkspaceConfig) error
	ListProviderHealth(ctx context.Context, workspaceID string) ([]models.ProviderHealth, error)
	ListAgentInstances(ctx context.Context, workspaceID string) ([]*models.AgentInstance, error)
	GetAgentInstance(ctx context.Context, id string) (*models.AgentInstance, error)
	// ClearAllParkedRoutingForWorkspace clears every parked run's
	// routing_blocked_status / earliest_retry_at / scheduled_retry_at
	// columns for the workspace and re-queues them. Called when the
	// workspace flips routing from enabled to disabled so the parked
	// runs unstick on the next scheduler tick.
	ClearAllParkedRoutingForWorkspace(ctx context.Context, workspaceID string) error
}

// RetryRunner runs a provider retry. Pulled out so the Provider depends
// on a tiny seam rather than the full SchedulerService.
type RetryRunner interface {
	RetryProvider(ctx context.Context, workspaceID, providerID string) error
}

// Provider implements the dashboard's RoutingProvider seam against the
// office repo + agent registry + scheduler retry. Kept in the routing
// package so the dashboard package stays repo-agnostic and the
// concrete preview logic lives next to the resolver it calls into.
type Provider struct {
	repo     ProviderRepo
	registry *registry.Registry
	resolver *Resolver
	retry    RetryRunner
}

// NewProvider builds a Provider over the supplied dependencies. registry
// may be nil — in that case KnownProviders falls back to the static v1
// allow-list. retry may be nil — the Retry method returns an error in
// that case so the HTTP layer can surface a 503.
func NewProvider(
	repo ProviderRepo,
	reg *registry.Registry,
	resolver *Resolver,
	retry RetryRunner,
) *Provider {
	return &Provider{repo: repo, registry: reg, resolver: resolver, retry: retry}
}

// GetConfig returns the workspace routing config + known-provider list.
// Defaults the config to the empty disabled shape when no row exists.
func (p *Provider) GetConfig(
	ctx context.Context, workspaceID string,
) (*WorkspaceConfig, []ProviderID, error) {
	cfg, err := p.repo.GetWorkspaceRouting(ctx, workspaceID)
	if err != nil {
		return nil, nil, err
	}
	if cfg == nil {
		cfg = &WorkspaceConfig{
			DefaultTier:      TierBalanced,
			ProviderOrder:    []ProviderID{},
			ProviderProfiles: map[ProviderID]ProviderProfile{},
		}
	}
	return cfg, KnownProviders(p.registry), nil
}

// UpdateConfig validates cfg via ValidateWorkspaceConfig and writes it
// when the validator passes. Validation runs in strict mode whenever
// cfg.Enabled is true.
//
// Side effect: parked runs are cleared whenever the active routing
// surface materially changes. Two cases:
//   - enabled→disabled: parked runs would otherwise sit forever waiting
//     on a provider that no longer matters.
//   - enabled→enabled with a material change (provider order, default
//     tier, or provider profiles): a user fixing a missing tier mapping
//     should unblock blocked_provider_action_required runs immediately;
//     without this, those parks persist until the user manually retries.
//
// "Material" is defined coarsely — any change to ProviderOrder,
// DefaultTier, or ProviderProfiles triggers a clear. False positives
// (clearing when the change couldn't affect any block reason) are
// harmless because runs simply re-dispatch and re-park with the
// latest verdict.
func (p *Provider) UpdateConfig(
	ctx context.Context, workspaceID string, cfg WorkspaceConfig,
) error {
	known := KnownProviders(p.registry)
	if err := ValidateWorkspaceConfig(cfg, known); err != nil {
		return err
	}
	prev, _ := p.repo.GetWorkspaceRouting(ctx, workspaceID)
	if err := p.repo.UpsertWorkspaceRouting(ctx, workspaceID, &cfg); err != nil {
		return err
	}
	if shouldClearParked(prev, cfg) {
		if err := p.repo.ClearAllParkedRoutingForWorkspace(ctx, workspaceID); err != nil {
			return fmt.Errorf("routing: clear parked runs: %w", err)
		}
	}
	return nil
}

// shouldClearParked reports whether the config transition warrants
// clearing parked runs. Disabled→disabled is a no-op.
func shouldClearParked(prev *WorkspaceConfig, next WorkspaceConfig) bool {
	if prev == nil || !prev.Enabled {
		return false
	}
	if !next.Enabled {
		return true
	}
	return !routingConfigEqual(*prev, next)
}

// routingConfigEqual reports whether two enabled configs are
// behaviorally identical for routing decisions. Compares provider
// order, default tier, and per-provider profile maps. The Enabled
// field is intentionally not compared — callers check that.
func routingConfigEqual(a, b WorkspaceConfig) bool {
	if a.DefaultTier != b.DefaultTier {
		return false
	}
	if !providerOrderEqual(a.ProviderOrder, b.ProviderOrder) {
		return false
	}
	return providerProfilesEqual(a.ProviderProfiles, b.ProviderProfiles)
}

func providerOrderEqual(a, b []ProviderID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func providerProfilesEqual(a, b map[ProviderID]ProviderProfile) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		vb, ok := b[k]
		if !ok || !providerProfileEqual(va, vb) {
			return false
		}
	}
	return true
}

func providerProfileEqual(a, b ProviderProfile) bool {
	if a.TierMap != b.TierMap || a.Mode != b.Mode {
		return false
	}
	if !stringSliceEqual(a.Flags, b.Flags) {
		return false
	}
	return stringMapEqual(a.Env, b.Env)
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func stringMapEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		if vb, ok := b[k]; !ok || va != vb {
			return false
		}
	}
	return true
}

// Retry dispatches a provider retry through the scheduler. Returns the
// new retry_at when one is known immediately; nil otherwise (a "retrying"
// status means the next dispatch will pick the provider up).
func (p *Provider) Retry(
	ctx context.Context, workspaceID, providerID string,
) (string, *time.Time, error) {
	if p.retry == nil {
		return "", nil, fmt.Errorf("routing: retry runner not wired")
	}
	if providerID == "" {
		return "", nil, fmt.Errorf("routing: provider_id required")
	}
	if err := p.retry.RetryProvider(ctx, workspaceID, providerID); err != nil {
		return "", nil, err
	}
	return "retrying", nil, nil
}

// Health returns the non-healthy provider rows for the workspace.
func (p *Provider) Health(
	ctx context.Context, workspaceID string,
) ([]models.ProviderHealth, error) {
	return p.repo.ListProviderHealth(ctx, workspaceID)
}

// Preview produces one PreviewItem per Office agent in the workspace by
// running Resolver.Resolve for each. The workspace config is read once
// at the top of the call so the preview is internally consistent even
// if the row is updated mid-iteration.
func (p *Provider) Preview(
	ctx context.Context, workspaceID string,
) ([]PreviewItem, error) {
	cfg, err := p.repo.GetWorkspaceRouting(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	agents, err := p.repo.ListAgentInstances(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]PreviewItem, 0, len(agents))
	for _, agent := range agents {
		if agent == nil {
			continue
		}
		item, err := p.previewForAgent(ctx, workspaceID, *agent, cfg)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

// AgentOverrides returns the routing override blob persisted on the
// named agent's settings JSON. The dashboard's GET /agents/:id/route
// endpoint surfaces this so the routing UI hydrates from the response
// on first paint. Returns the zero blob (no overrides) when the agent
// has no routing key in its settings; parse errors are surfaced so
// the HTTP layer can log them rather than silently masking a malformed
// row.
func (p *Provider) AgentOverrides(
	ctx context.Context, agentID string,
) (AgentOverrides, error) {
	agent, err := p.repo.GetAgentInstance(ctx, agentID)
	if err != nil || agent == nil {
		return AgentOverrides{}, err
	}
	return ReadAgentOverrides(agent.Settings)
}

// PreviewAgent returns the per-agent preview for a single agent id,
// or (nil, nil) when the id does not resolve to an office agent.
func (p *Provider) PreviewAgent(
	ctx context.Context, agentID string,
) (*PreviewItem, error) {
	agent, err := p.repo.GetAgentInstance(ctx, agentID)
	if err != nil || agent == nil {
		return nil, err
	}
	cfg, err := p.repo.GetWorkspaceRouting(ctx, agent.WorkspaceID)
	if err != nil {
		return nil, err
	}
	item, err := p.previewForAgent(ctx, agent.WorkspaceID, *agent, cfg)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// previewForAgent builds the per-agent row. When routing is disabled
// the preview still shows the workspace default tier so the UI can
// render a row instead of a blank cell.
func (p *Provider) previewForAgent(
	ctx context.Context, workspaceID string,
	agent models.AgentInstance, cfg *WorkspaceConfig,
) (PreviewItem, error) {
	tierSource := tierSourceForAgent(agent, cfg)
	res, err := p.resolver.Resolve(ctx, workspaceID, agent, ResolveOptions{})
	if err != nil {
		return PreviewItem{}, fmt.Errorf("routing: resolve %s: %w", agent.ID, err)
	}
	primaryProvider, primaryModel := primaryProviderModel(res, cfg)
	return PreviewItem{
		AgentID:           agent.ID,
		AgentName:         agent.Name,
		TierSource:        tierSource,
		EffectiveTier:     string(effectivePreviewTier(res, cfg)),
		PrimaryProviderID: primaryProvider,
		PrimaryModel:      primaryModel,
		CurrentProviderID: firstCandidateProvider(res),
		CurrentModel:      firstCandidateModel(res),
		FallbackChain:     fallbackChain(res),
		Missing:           missingHints(res),
		Degraded:          hasDegradedSkip(res),
	}, nil
}

// primaryProviderModel returns the configured-intent primary route: the
// first entry in the effective provider order, even when that provider
// is currently degraded or missing a tier mapping. The model comes from
// the workspace ProviderProfiles tier map for the requested tier; empty
// when the mapping isn't set.
func primaryProviderModel(res *Resolution, cfg *WorkspaceConfig) (string, string) {
	if res == nil || cfg == nil || len(res.ProviderOrder) == 0 {
		return "", ""
	}
	first := res.ProviderOrder[0]
	prof, ok := cfg.ProviderProfiles[first]
	if !ok {
		return string(first), ""
	}
	return string(first), prof.TierMap.Model(res.RequestedTier)
}

// tierSourceForAgent returns "override" when the agent's settings flip
// TierSource explicitly; "inherit" otherwise.
func tierSourceForAgent(agent models.AgentInstance, _ *WorkspaceConfig) string {
	ov, err := ReadAgentOverrides(agent.Settings)
	if err != nil {
		return "inherit"
	}
	if ov.TierSource == TierSourceOverride && ov.Tier != "" {
		return "override"
	}
	return "inherit"
}

// effectivePreviewTier returns the tier the resolver consumed, falling
// back to the workspace default when the resolver short-circuited
// because routing is disabled.
func effectivePreviewTier(res *Resolution, cfg *WorkspaceConfig) Tier {
	if res != nil && res.RequestedTier != "" {
		return res.RequestedTier
	}
	if cfg != nil {
		return cfg.DefaultTier
	}
	return TierBalanced
}

// firstCandidateProvider returns the first non-skipped candidate's
// provider id, or "" when every provider was skipped.
func firstCandidateProvider(res *Resolution) string {
	if res == nil || len(res.Candidates) == 0 {
		return ""
	}
	return string(res.Candidates[0].ProviderID)
}

// firstCandidateModel returns the first non-skipped candidate's model.
func firstCandidateModel(res *Resolution) string {
	if res == nil || len(res.Candidates) == 0 {
		return ""
	}
	return res.Candidates[0].Model
}

// fallbackChain returns every successful candidate after the primary so
// the UI can render the "primary → fallback1 → fallback2" trail.
func fallbackChain(res *Resolution) []PreviewProviderModel {
	if res == nil || len(res.Candidates) <= 1 {
		return []PreviewProviderModel{}
	}
	out := make([]PreviewProviderModel, 0, len(res.Candidates)-1)
	for _, c := range res.Candidates[1:] {
		out = append(out, PreviewProviderModel{
			ProviderID: string(c.ProviderID),
			Model:      c.Model,
			Tier:       string(c.Tier),
		})
	}
	return out
}

// missingHints translates skip-reasons into human-friendly bullets the
// UI can surface as "needs attention" badges.
func missingHints(res *Resolution) []string {
	if res == nil {
		return []string{}
	}
	out := make([]string, 0, len(res.SkippedDegraded))
	for _, sk := range res.SkippedDegraded {
		hint := skipReasonHint(sk)
		if hint != "" {
			out = append(out, hint)
		}
	}
	return out
}

// skipReasonHint returns a human-readable hint for one skipped
// candidate, or "" when the skip is not worth surfacing.
func skipReasonHint(sk SkippedCandidate) string {
	switch sk.Reason {
	case SkipReasonMissingModelMapping:
		return fmt.Sprintf("%s: missing model mapping for this tier",
			string(sk.ProviderID))
	case SkipReasonUserAction:
		return fmt.Sprintf("%s: needs user action (%s)",
			string(sk.ProviderID), sk.ErrorCode)
	}
	return ""
}

// hasDegradedSkip reports whether any skip in res is a degraded provider
// awaiting auto-retry. Surfaced as the row-level "degraded" flag.
func hasDegradedSkip(res *Resolution) bool {
	if res == nil {
		return false
	}
	for _, sk := range res.SkippedDegraded {
		if sk.Reason == SkipReasonDegraded {
			return true
		}
	}
	return false
}
