package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingpolicy"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/store"
)

// fixedProfileRepository is a minimal store.Repository stand-in. The embedded
// interface is left nil; only GetAgentProfile is overridden, since that is
// the only method these tests' code paths reach.
type fixedProfileRepository struct {
	store.Repository
	profile *agentsettingsmodels.AgentProfile
}

func (f *fixedProfileRepository) GetAgentProfile(_ context.Context, id string) (*agentsettingsmodels.AgentProfile, error) {
	if f.profile != nil && f.profile.ID == id {
		return f.profile, nil
	}
	return nil, errors.New("agent profile not found")
}

// fixedRouteStateLoader hands the engine one durable route state on first
// access, standing in for a state restored after a backend restart.
type fixedRouteStateLoader struct {
	state dynamic.RouteState
}

func (f *fixedRouteStateLoader) LoadRouteState(_ context.Context, sessionID string) (*dynamic.RouteState, error) {
	if sessionID != f.state.SessionID {
		return nil, nil
	}
	state := f.state
	return &state, nil
}

func persistedRetryWaitState(t *testing.T, sessionID string) dynamic.RouteState {
	t.Helper()
	deadline := time.Now().Add(-time.Minute)
	policyState := dynamic.PolicyState{Deadline: &deadline}
	raw, err := json.Marshal(policyState)
	if err != nil {
		t.Fatalf("marshal policy state: %v", err)
	}
	return dynamic.RouteState{
		SessionID:          sessionID,
		LogicalProfileID:   "dynamic-policy",
		ExecutionProfileID: "first",
		Generation:         1,
		ProfileVersion:     1,
		Status:             string(routingpolicy.DecisionRetry),
		PolicyStateJSON:    string(raw),
		UpdatedAt:          time.Now().Add(-time.Minute),
	}
}

func TestResumePendingRoute_DisabledFlagRefusesWithoutMutatingState(t *testing.T) {
	const sessionID = "policy-session"
	persisted := persistedRetryWaitState(t, sessionID)
	loader := &fixedRouteStateLoader{state: persisted}
	engine := dynamic.NewEngine(dynamic.WithStateLoader(loader))
	resolver := agentruntime.NewProfileExecutionResolver(&fixedProfileRepository{}, engine, false)

	ctx := context.Background()
	if _, err := resolver.ResumePendingRoute(ctx, sessionID, persisted.Generation); !errors.Is(err, agentruntime.ErrDynamicRoutingDisabled) {
		t.Fatalf("ResumePendingRoute with flag off = %v, want ErrDynamicRoutingDisabled", err)
	}

	state, exists, err := engine.LoadState(ctx, sessionID)
	if err != nil {
		t.Fatalf("LoadState after refused resume: %v", err)
	}
	if !exists || state.Status != string(routingpolicy.DecisionRetry) {
		t.Fatalf("route state after refused resume = %+v (exists=%v), want unchanged retry_wait", state, exists)
	}
}

func TestResumePendingRoute_EnabledFlagResumesDueRoute(t *testing.T) {
	const sessionID = "policy-session-enabled"
	persisted := persistedRetryWaitState(t, sessionID)
	loader := &fixedRouteStateLoader{state: persisted}
	engine := dynamic.NewEngine(dynamic.WithStateLoader(loader))
	profiles := &fixedProfileRepository{profile: &agentsettingsmodels.AgentProfile{ID: "first", Enabled: true}}
	resolver := agentruntime.NewProfileExecutionResolver(profiles, engine, true)

	ctx := context.Background()
	execution, err := resolver.ResumePendingRoute(ctx, sessionID, persisted.Generation)
	if err != nil {
		t.Fatalf("ResumePendingRoute with flag on: %v", err)
	}
	if execution.ExecutionProfileID != "first" || execution.Generation != persisted.Generation {
		t.Fatalf("execution = %+v, want ExecutionProfileID=first Generation=%d", execution, persisted.Generation)
	}

	state, exists, err := engine.LoadState(ctx, sessionID)
	if err != nil {
		t.Fatalf("LoadState after resume: %v", err)
	}
	if !exists || state.Status != "retrying" {
		t.Fatalf("route state after resume = %+v (exists=%v), want status retrying", state, exists)
	}
}
