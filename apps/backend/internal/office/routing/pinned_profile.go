package routing

import (
	"context"
	"fmt"
)

// resolvePinnedProfile uses exactly one complete profile. Workspace fallback and
// tier mappings cannot move a persona onto another account.
func (r *Resolver) resolvePinnedProfile(ctx context.Context, workspaceID, id string, opts ResolveOptions) (*Resolution, error) {
	if r.profiles == nil {
		return nil, fmt.Errorf("execution profile store unavailable")
	}
	profile, err := r.profiles.GetAgentProfile(ctx, id)
	if err != nil || profile == nil {
		return nil, fmt.Errorf("execution profile not found")
	}
	provider, err := r.profiles.GetAgent(ctx, profile.AgentID)
	if err != nil || provider == nil {
		return nil, fmt.Errorf("execution provider not found")
	}
	pid := ProviderID(provider.Name)
	if _, err := r.resolveExecutionProfile(ctx, workspaceID, pid, TierBalanced, id); err != nil {
		return nil, err
	}
	res := &Resolution{Enabled: true, RequestedTier: TierBalanced, ProviderOrder: []ProviderID{pid},
		PinnedProfile: &Candidate{ExecutionProfileID: id, ProviderID: pid, Model: profile.Model, Tier: TierBalanced}}
	if _, excluded := providerExcludeSet(opts.ExcludeProviders)[pid]; excluded {
		res.BlockReason.Status = StatusBlockedActionRequired
		return res, nil
	}
	cfg := &WorkspaceConfig{ProviderProfiles: map[ProviderID]ProviderProfile{
		pid: {ExecutionProfileIDs: ExecutionProfileIDs{Balanced: id}},
	}}
	idx, err := r.loadHealthIndex(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if _, err := r.evaluateProvider(ctx, workspaceID, res, cfg, idx, pid, TierBalanced, r.clock(), false); err != nil {
		return nil, err
	}
	if len(res.Candidates) == 0 {
		res.BlockReason = aggregateBlock(res.SkippedDegraded)
	}
	return res, nil
}
