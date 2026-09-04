package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agent/runtime/routingpolicy"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/store"
)

func TestResolveRouteActionRejectsManualRetryAfterRecoveryClaim(t *testing.T) {
	ctx := context.Background()
	resolver, engine, generation := newRetryingRouteActionResolver(t)
	_, err := resolver.ResolveRouteAction(
		ctx, "session-retry", "dynamic-profile", "concrete-profile", generation, "retry",
	)
	if !errors.Is(err, dynamic.ErrRecoveryPending) {
		t.Fatalf("ResolveRouteAction error = %v, want recovery pending", err)
	}
	state, ok := engine.State("session-retry")
	if !ok || state.Generation != generation || state.Status != "retrying" {
		t.Fatalf("route state = %#v, ok=%v, want unchanged claimed state", state, ok)
	}
}

func TestResolveRouteActionRejectsSkipAfterRecoveryClaim(t *testing.T) {
	resolver, _, generation := newRetryingRouteActionResolver(t)
	_, err := resolver.ResolveRouteAction(
		context.Background(), "session-retry", "dynamic-profile", "concrete-profile", generation, "skip",
	)
	if !errors.Is(err, dynamic.ErrRecoveryPending) {
		t.Fatalf("ResolveRouteAction error = %v, want recovery pending", err)
	}
}

func TestMarkRouteActionRequiredUnblocksRetryAndSkipAfterFailedLaunch(t *testing.T) {
	ctx := context.Background()
	resolver, engine, generation := newRetryingRouteActionResolver(t)

	// Simulate the launch-failure handler's sync (dynamic_policy_recovery.go
	// and dynamic_routing.go both call this after a resumed launch fails).
	if err := resolver.MarkRouteActionRequired(ctx, "session-retry", generation); err != nil {
		t.Fatalf("MarkRouteActionRequired: %v", err)
	}
	state, ok := engine.State("session-retry")
	if !ok || state.Status != "action_required" {
		t.Fatalf("route state after sync = %#v, ok=%v, want action_required", state, ok)
	}

	if _, err := resolver.ResolveRouteAction(
		ctx, "session-retry", "dynamic-profile", "concrete-profile", generation, "retry",
	); err != nil {
		t.Fatalf("ResolveRouteAction retry after sync = %v, want success", err)
	}
}

func newRetryingRouteActionResolver(t *testing.T) (*ProfileExecutionResolver, *dynamic.Engine, int64) {
	t.Helper()
	ctx := context.Background()
	engine := dynamic.NewEngine()
	document := routingpolicy.DefaultDocument()
	document.Transient.Retry = routingpolicy.RetryPolicy{
		Enabled: true, MaxRetries: 1, InitialIntervalSeconds: 60,
	}
	profile := dynamic.Profile{ID: "dynamic-profile", Version: 1, Candidates: []dynamic.Candidate{
		{ID: "concrete-profile", Enabled: true, Policies: document},
	}}
	initial, err := engine.Select("session-retry", profile, 0, "")
	if err != nil {
		t.Fatalf("initial Select: %v", err)
	}
	if _, err := engine.ApplyFailure(
		"session-retry", profile, initial.Generation, initial.ExecutionProfileID,
		&routingerr.Error{Code: routingerr.CodeRateLimited, Class: routingerr.ClassTransient, FallbackAllowed: true},
	); !errors.Is(err, dynamic.ErrRecoveryPending) {
		t.Fatalf("ApplyFailure error = %v, want recovery pending", err)
	}
	if _, err := engine.ResumePendingNow(ctx, "session-retry", initial.Generation); err != nil {
		t.Fatalf("ResumePendingNow: %v", err)
	}

	profiles := &dynamicResolverTestProfiles{
		logical:  &agentsettingsmodels.AgentProfile{ID: "dynamic-profile", AgentID: agents.DynamicAgentID, Enabled: true},
		concrete: &agentsettingsmodels.AgentProfile{ID: "concrete-profile", AgentID: "concrete", Enabled: true},
		dynamic:  &agentsettingsmodels.DynamicAgentProfile{ProfileID: "dynamic-profile", Version: 1},
		routes: []agentsettingsmodels.DynamicAgentRoute{{
			DynamicProfileID: "dynamic-profile", ExecutionProfileID: "concrete-profile", Enabled: true,
		}},
	}
	return NewProfileExecutionResolver(profiles, engine, true), engine, initial.Generation
}

type dynamicResolverTestProfiles struct {
	store.Repository
	store.DynamicProfileRepository
	logical  *agentsettingsmodels.AgentProfile
	concrete *agentsettingsmodels.AgentProfile
	dynamic  *agentsettingsmodels.DynamicAgentProfile
	routes   []agentsettingsmodels.DynamicAgentRoute
}

func (p *dynamicResolverTestProfiles) GetAgentProfile(_ context.Context, id string) (*agentsettingsmodels.AgentProfile, error) {
	switch id {
	case p.logical.ID:
		return p.logical, nil
	case p.concrete.ID:
		return p.concrete, nil
	default:
		return nil, errors.New("profile not found")
	}
}

func (p *dynamicResolverTestProfiles) GetAgent(_ context.Context, id string) (*agentsettingsmodels.Agent, error) {
	if id == agents.DynamicAgentID {
		return &agentsettingsmodels.Agent{ID: id, Name: agents.DynamicAgentID}, nil
	}
	return &agentsettingsmodels.Agent{ID: id, Name: "concrete"}, nil
}

func (p *dynamicResolverTestProfiles) GetDynamicAgentProfile(
	_ context.Context, profileID string,
) (*agentsettingsmodels.DynamicAgentProfile, []agentsettingsmodels.DynamicAgentRoute, error) {
	if profileID != p.dynamic.ProfileID {
		return nil, nil, errors.New("dynamic profile not found")
	}
	return p.dynamic, p.routes, nil
}
