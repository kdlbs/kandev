package controller

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
)

type fakeHostUsageLister struct {
	entries   []agentusage.HostAgentUsage
	lastFresh bool
	calls     int
}

func (f *fakeHostUsageLister) List(_ context.Context, fresh bool) []agentusage.HostAgentUsage {
	f.calls++
	f.lastFresh = fresh
	return f.entries
}

func TestControllerSubscriptionUsage_NoLister(t *testing.T) {
	ctrl := newTestController(map[string]agents.Agent{})

	resp := ctrl.SubscriptionUsage(context.Background(), false)

	if resp == nil || resp.Agents == nil {
		t.Fatalf("expected non-nil response with empty agents, got %+v", resp)
	}
	if len(resp.Agents) != 0 {
		t.Fatalf("expected empty listing without a lister, got %+v", resp.Agents)
	}
}

func TestControllerSubscriptionUsage_MapsEntries(t *testing.T) {
	ctrl := newTestController(map[string]agents.Agent{
		"claude-acp": &testAgent{id: "claude-acp", name: "claude-acp", displayName: "Claude", enabled: true},
	})
	now := time.Now()
	usage := &agentusage.ProviderUsage{
		Provider:  "anthropic",
		Plan:      "max",
		Windows:   []agentusage.UtilizationWindow{{Label: "5-hour", UtilizationPct: 42, ResetAt: now}},
		FetchedAt: now,
	}
	lister := &fakeHostUsageLister{entries: []agentusage.HostAgentUsage{
		{AgentID: "claude-acp", Usage: usage},
		{AgentID: "unknown-acp", Error: "failed to fetch usage from provider"},
	}}
	ctrl.SetHostUsageLister(lister)

	resp := ctrl.SubscriptionUsage(context.Background(), false)

	if len(resp.Agents) != 2 {
		t.Fatalf("agents = %+v, want 2 entries", resp.Agents)
	}
	// Registry display-name lookup for known agents.
	if resp.Agents[0].AgentID != "claude-acp" || resp.Agents[0].DisplayName != "Claude" {
		t.Errorf("entry[0] = %+v", resp.Agents[0])
	}
	if resp.Agents[0].Usage != usage || resp.Agents[0].Error != "" {
		t.Errorf("entry[0] usage/error = %+v", resp.Agents[0])
	}
	// Unknown agents fall back to the agent ID and forward the error.
	if resp.Agents[1].DisplayName != "unknown-acp" || resp.Agents[1].Error == "" {
		t.Errorf("entry[1] = %+v", resp.Agents[1])
	}
	if resp.Agents[1].Usage != nil {
		t.Errorf("entry[1] usage = %+v, want nil", resp.Agents[1].Usage)
	}
}

func TestControllerSubscriptionUsage_PropagatesFresh(t *testing.T) {
	ctrl := newTestController(map[string]agents.Agent{})
	lister := &fakeHostUsageLister{}
	ctrl.SetHostUsageLister(lister)

	_ = ctrl.SubscriptionUsage(context.Background(), true)
	if !lister.lastFresh {
		t.Error("fresh=true was not propagated to the lister")
	}
	_ = ctrl.SubscriptionUsage(context.Background(), false)
	if lister.lastFresh {
		t.Error("fresh=false was not propagated to the lister")
	}
	if lister.calls != 2 {
		t.Errorf("lister calls = %d, want 2", lister.calls)
	}
}
