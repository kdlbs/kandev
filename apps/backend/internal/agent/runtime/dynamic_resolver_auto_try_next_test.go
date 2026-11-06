package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
)

func newAutoTryNextResolver(t *testing.T) (*ProfileExecutionResolver, int64) {
	t.Helper()
	engine := dynamic.NewEngine()
	profiles := &dynamicResolverTestProfiles{
		logical:  &agentsettingsmodels.AgentProfile{ID: "dynamic-profile", AgentID: agents.DynamicAgentID, Enabled: true},
		concrete: &agentsettingsmodels.AgentProfile{ID: "concrete-a", AgentID: "concrete", Enabled: true},
		bound:    &agentsettingsmodels.AgentProfile{ID: "concrete-b", AgentID: "concrete", Enabled: true},
		dynamic:  &agentsettingsmodels.DynamicAgentProfile{ProfileID: "dynamic-profile", Version: 1},
		routes: []agentsettingsmodels.DynamicAgentRoute{
			{DynamicProfileID: "dynamic-profile", ExecutionProfileID: "concrete-a", Enabled: true},
			{DynamicProfileID: "dynamic-profile", ExecutionProfileID: "concrete-b", Enabled: true},
		},
	}
	initial, err := engine.Select("session-auto", dynamic.Profile{
		ID: "dynamic-profile", Version: 1,
		Candidates: []dynamic.Candidate{{ID: "concrete-a", Enabled: true}, {ID: "concrete-b", Enabled: true}},
	}, 0, "")
	if err != nil {
		t.Fatalf("initial Select: %v", err)
	}
	return NewProfileExecutionResolver(profiles, engine, true), initial.Generation
}

func TestResolveRouteActionAutoTryNextSelectsNextCandidateWithItsReason(t *testing.T) {
	resolver, generation := newAutoTryNextResolver(t)
	execution, err := resolver.ResolveRouteAction(
		context.Background(), "session-auto", "dynamic-profile", "concrete-a", generation, RouteActionAutoTryNext,
	)
	if err != nil {
		t.Fatalf("ResolveRouteAction: %v", err)
	}
	if execution.ExecutionProfileID != "concrete-b" || execution.Generation != generation+1 ||
		execution.Decision.Reason != RouteReasonAutoProviderFailover {
		t.Fatalf("execution = %#v, want concrete-b at the next generation with the automatic reason", execution)
	}
}

func TestResolveRouteActionAutoTryNextIsGenerationFenced(t *testing.T) {
	resolver, generation := newAutoTryNextResolver(t)
	_, err := resolver.ResolveRouteAction(
		context.Background(), "session-auto", "dynamic-profile", "concrete-a", generation-1, RouteActionAutoTryNext,
	)
	if !errors.Is(err, dynamic.ErrStaleGeneration) {
		t.Fatalf("ResolveRouteAction error = %v, want stale generation", err)
	}
}
