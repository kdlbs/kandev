package controller

import (
	"slices"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

type orderedTestAgent struct {
	*testAgent
	order int
}

func (a *orderedTestAgent) DisplayOrder() int { return a.order }

func TestSortAgentsByDisplayOrder_PutsUnknownAgentsLast(t *testing.T) {
	ctrl := newTestController(map[string]agents.Agent{
		"early": &orderedTestAgent{testAgent: &testAgent{id: "early"}, order: 1},
		"late":  &orderedTestAgent{testAgent: &testAgent{id: "late"}, order: 99},
	})
	payload := []dto.AgentDTO{
		{Name: "unknown-first"},
		{Name: "late"},
		{Name: "early"},
		{Name: "unknown-second"},
	}

	ctrl.sortAgentsByDisplayOrder(payload)

	got := make([]string, 0, len(payload))
	for _, agent := range payload {
		got = append(got, agent.Name)
	}
	want := []string{"early", "late", "unknown-first", "unknown-second"}
	if !slices.Equal(got, want) {
		t.Fatalf("agent order = %v, want %v", got, want)
	}
}
