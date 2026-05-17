package routing

import (
	"context"
	"fmt"
	"time"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/office/models"
)

// Health state strings recognised by the resolver. Kept as local constants
// (mirroring repository/sqlite.HealthState*) so the routing package has no
// import dependency on the repo. Values must stay in sync.
const (
	healthStateHealthy            = "healthy"
	healthStateDegraded           = "degraded"
	healthStateUserActionRequired = "user_action_required"
)

// Health scope strings used by the scoped lookup. Mirror the repo's
// HealthScope* constants (see repository/sqlite/provider_health.go).
const (
	healthScopeProvider = "provider"
	healthScopeTier     = "tier"
	healthScopeModel    = "model"
)

// BlockReason status values. Exported so the dispatcher (Phase 4) and the
// HTTP layer (Phase 5) can persist / surface the same strings.
const (
	StatusWaitingForCapacity    = "waiting_for_provider_capacity"
	StatusBlockedActionRequired = "blocked_provider_action_required"
)

// Skip reason values recorded on SkippedCandidate.Reason. Short string
// constants so the HTTP layer and tests can compare without re-spelling.
const (
	SkipReasonDegraded            = "skipped_degraded"
	SkipReasonUserAction          = "skipped_user_action"
	SkipReasonMissingModelMapping = "missing_model_mapping"
)

// autoRetryableCodes is the allow-list of error codes that mark a
// degraded provider as "the scheduler will retry on its own." Anything
// outside this set is user-actionable. Referenced again by Phase 4
// (block-reason aggregation, wake-up scheduling).
var autoRetryableCodes = map[string]struct{}{
	"rate_limited":           {},
	"quota_limited":          {},
	"provider_unavailable":   {},
	"unknown_provider_error": {},
}

// IsAutoRetryableCode reports whether code is in the auto-retry allow-list.
func IsAutoRetryableCode(code string) bool {
	_, ok := autoRetryableCodes[code]
	return ok
}

// Repo is the narrow interface the resolver needs over the sqlite repo.
// Kept here so tests can pass a fake without standing up SQLite.
type Repo interface {
	GetWorkspaceRouting(ctx context.Context, workspaceID string) (*WorkspaceConfig, error)
	ListProviderHealth(ctx context.Context, workspaceID string) ([]models.ProviderHealth, error)
}

// Resolver turns workspace routing config + agent overrides + provider
// health into an ordered Candidate list. Pure: no I/O beyond Repo reads.
type Resolver struct {
	repo  Repo
	clock func() time.Time
}

// NewResolver builds a Resolver. When clock is nil, time.Now is used.
func NewResolver(repo Repo, clock func() time.Time) *Resolver {
	if clock == nil {
		clock = time.Now
	}
	return &Resolver{repo: repo, clock: clock}
}

// Candidate is one launchable provider/model/tier triple, with the
// provider-scoped CLI knobs (mode, flags, env) copied verbatim from the
// workspace ProviderProfile. Self-describing so the launch path does not
// have to reach back into routing config.
type Candidate struct {
	ProviderID ProviderID
	Model      string
	Tier       Tier
	Mode       string
	Flags      []string
	Env        map[string]string
}

// SkippedCandidate records why a provider in the effective order was
// not appended to Candidates. Surfaced in the BlockReason and in run
// telemetry (Phase 4).
type SkippedCandidate struct {
	ProviderID ProviderID
	Reason     string
	ErrorCode  string
	State      string
	RetryAt    time.Time
	AutoRetry  bool
	Scope      string
	ScopeValue string
	RawExcerpt string
}

// BlockReason aggregates why Candidates is empty. EarliestRetry is zero
// when nothing is auto-retryable.
type BlockReason struct {
	Status        string
	EarliestRetry time.Time
	Skipped       []SkippedCandidate
}

// Resolution is the resolver's output. When Enabled is false, callers
// fall through to the existing concrete-profile launch path and ignore
// the other fields.
type Resolution struct {
	Enabled         bool
	RequestedTier   Tier
	ProviderOrder   []ProviderID
	Candidates      []Candidate
	SkippedDegraded []SkippedCandidate
	BlockReason     BlockReason
}

// ResolveOptions tweak the resolution for one call. The dispatcher
// passes already-tried providers via ExcludeProviders so post-start
// fallback never re-picks a provider that has already failed this run.
type ResolveOptions struct {
	ExcludeProviders []ProviderID
	// Reason is the run's wake reason (heartbeat / routine_trigger /
	// budget_alert / …). When the effective TierPerReason map carries
	// a tier for this reason, that tier wins over the agent tier
	// override and the workspace default.
	Reason string
}

// healthIndex is the per-workspace health rows pre-grouped by provider
// for cheap scope lookups inside the candidate loop.
type healthIndex map[ProviderID]map[string]map[string]models.ProviderHealth

// Resolve runs the resolution algorithm described in
// docs/specs/office-provider-routing/plan.md §Phase 2.
func (r *Resolver) Resolve(
	ctx context.Context,
	workspaceID string,
	agent settingsmodels.AgentProfile,
	opts ResolveOptions,
) (*Resolution, error) {
	cfg, err := r.repo.GetWorkspaceRouting(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("routing: load workspace config: %w", err)
	}
	if cfg == nil || !cfg.Enabled {
		return &Resolution{Enabled: false}, nil
	}
	ov, err := ReadAgentOverrides(agent.Settings)
	if err != nil {
		return nil, fmt.Errorf("routing: load agent overrides: %w", err)
	}
	tier := effectiveTier(cfg, ov, opts.Reason)
	order := effectiveOrder(cfg, ov)
	if len(order) == 0 {
		return nil, ErrEmptyOrder
	}
	idx, err := r.loadHealthIndex(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	res := &Resolution{Enabled: true, RequestedTier: tier, ProviderOrder: order}
	excluded := providerExcludeSet(opts.ExcludeProviders)
	now := r.clock()
	for _, pid := range order {
		if _, skip := excluded[pid]; skip {
			continue
		}
		r.evaluateProvider(res, cfg, idx, pid, tier, now)
	}
	if len(res.Candidates) == 0 {
		res.BlockReason = aggregateBlock(res.SkippedDegraded)
	}
	return res, nil
}

// evaluateProvider runs the per-provider half of the resolve loop:
// missing-mapping check, scoped health lookup, then appends either a
// Candidate or a SkippedCandidate to res.
func (r *Resolver) evaluateProvider(
	res *Resolution, cfg *WorkspaceConfig, idx healthIndex,
	pid ProviderID, tier Tier, now time.Time,
) {
	prof, ok := cfg.ProviderProfiles[pid]
	model := prof.TierMap.Model(tier)
	if !ok || model == "" {
		res.SkippedDegraded = append(res.SkippedDegraded, SkippedCandidate{
			ProviderID: pid,
			Reason:     SkipReasonMissingModelMapping,
		})
		return
	}
	if hit, found := lookupHealth(idx, pid, tier, model); found {
		if sc, skip := classifyHealthHit(pid, hit, now); skip {
			res.SkippedDegraded = append(res.SkippedDegraded, sc)
			return
		}
	}
	res.Candidates = append(res.Candidates, Candidate{
		ProviderID: pid,
		Model:      model,
		Tier:       tier,
		Mode:       prof.Mode,
		Flags:      prof.Flags,
		Env:        prof.Env,
	})
}

// loadHealthIndex pulls every non-healthy row for the workspace and
// groups them by (provider, scope, scopeValue) for the resolve loop.
// ListProviderHealth already filters out healthy rows at SQL level.
func (r *Resolver) loadHealthIndex(
	ctx context.Context, workspaceID string,
) (healthIndex, error) {
	rows, err := r.repo.ListProviderHealth(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("routing: load provider health: %w", err)
	}
	idx := healthIndex{}
	for _, row := range rows {
		byScope, ok := idx[ProviderID(row.ProviderID)]
		if !ok {
			byScope = map[string]map[string]models.ProviderHealth{}
			idx[ProviderID(row.ProviderID)] = byScope
		}
		byValue, ok := byScope[string(row.Scope)]
		if !ok {
			byValue = map[string]models.ProviderHealth{}
			byScope[string(row.Scope)] = byValue
		}
		byValue[row.ScopeValue] = row
	}
	return idx, nil
}

// lookupHealth walks the scope priority (provider > tier > model) and
// returns the first row found. The caller decides whether the row
// causes a skip — a row with state=healthy is treated as "ignore" so
// the iteration continues to lower-priority scopes.
func lookupHealth(
	idx healthIndex, pid ProviderID, tier Tier, model string,
) (models.ProviderHealth, bool) {
	byScope, ok := idx[pid]
	if !ok {
		return models.ProviderHealth{}, false
	}
	probes := []struct{ scope, value string }{
		{healthScopeProvider, ""},
		{healthScopeTier, string(tier)},
		{healthScopeModel, model},
	}
	for _, p := range probes {
		row, ok := byScope[p.scope][p.value]
		if !ok {
			continue
		}
		if string(row.State) == healthStateHealthy {
			continue
		}
		return row, true
	}
	return models.ProviderHealth{}, false
}

// classifyHealthHit turns a non-healthy health row into a SkippedCandidate
// when applicable. Returns skip=false for degraded rows whose retry_at
// has passed — those are eligible (the next launch acts as the probe,
// see plan §Phase 4.5).
func classifyHealthHit(
	pid ProviderID, row models.ProviderHealth, now time.Time,
) (SkippedCandidate, bool) {
	sc := SkippedCandidate{
		ProviderID: pid,
		ErrorCode:  row.ErrorCode,
		State:      string(row.State),
		Scope:      string(row.Scope),
		ScopeValue: row.ScopeValue,
		RawExcerpt: row.RawExcerpt,
		AutoRetry:  IsAutoRetryableCode(row.ErrorCode),
	}
	switch string(row.State) {
	case healthStateDegraded:
		if row.RetryAt == nil || !row.RetryAt.After(now) {
			return SkippedCandidate{}, false
		}
		sc.Reason = SkipReasonDegraded
		sc.RetryAt = *row.RetryAt
		return sc, true
	case healthStateUserActionRequired:
		sc.Reason = SkipReasonUserAction
		sc.AutoRetry = false
		return sc, true
	}
	return SkippedCandidate{}, false
}

// aggregateBlock decides the BlockReason status from the skip list:
// any auto-retryable in the list → waiting_for_provider_capacity;
// otherwise blocked_provider_action_required. EarliestRetry is the
// minimum RetryAt across degraded entries (zero when none).
func aggregateBlock(skipped []SkippedCandidate) BlockReason {
	br := BlockReason{Skipped: skipped}
	anyAutoRetry := false
	var earliest time.Time
	for _, s := range skipped {
		if s.AutoRetry {
			anyAutoRetry = true
		}
		if !s.RetryAt.IsZero() && (earliest.IsZero() || s.RetryAt.Before(earliest)) {
			earliest = s.RetryAt
		}
	}
	if anyAutoRetry {
		br.Status = StatusWaitingForCapacity
		br.EarliestRetry = earliest
		return br
	}
	br.Status = StatusBlockedActionRequired
	return br
}

// effectiveTier resolves the tier for one run.
// Order: 1) wake-reason policy (agent override > workspace policy),
// 2) agent tier override, 3) workspace default. The reason argument
// may be empty when the caller has no run context — in that case the
// wake-reason step is skipped.
func effectiveTier(cfg *WorkspaceConfig, ov AgentOverrides, reason string) Tier {
	if reason != "" {
		if t := wakeReasonTier(cfg, ov, reason); t != "" {
			return t
		}
	}
	if ov.TierSource == TierSourceOverride && ov.Tier != "" {
		return ov.Tier
	}
	return cfg.DefaultTier
}

// wakeReasonTier returns the tier the wake-reason policy assigns for
// reason, or "" when no policy applies. Agent override map wins over
// workspace map; missing keys fall through to "".
func wakeReasonTier(cfg *WorkspaceConfig, ov AgentOverrides, reason string) Tier {
	if ov.TierPerReasonSource == TierPerReasonSourceOverride {
		if t, ok := ov.TierPerReason[reason]; ok && t != "" {
			return t
		}
		return ""
	}
	if t, ok := cfg.TierPerReason[reason]; ok && t != "" {
		return t
	}
	return ""
}

// effectiveOrder returns the override order when
// AgentOverrides.ProviderOrderSource is "override"; otherwise a copy
// of the workspace order. Returns a copy so callers can mutate freely.
func effectiveOrder(cfg *WorkspaceConfig, ov AgentOverrides) []ProviderID {
	src := cfg.ProviderOrder
	if ov.ProviderOrderSource == ProviderOrderSourceOverride {
		src = ov.ProviderOrder
	}
	out := make([]ProviderID, len(src))
	copy(out, src)
	return out
}

// providerExcludeSet turns a slice into a lookup set. Empty input
// returns nil so the caller's `_, ok := nil[k]` check still works.
func providerExcludeSet(in []ProviderID) map[ProviderID]struct{} {
	if len(in) == 0 {
		return nil
	}
	out := make(map[ProviderID]struct{}, len(in))
	for _, p := range in {
		out[p] = struct{}{}
	}
	return out
}
