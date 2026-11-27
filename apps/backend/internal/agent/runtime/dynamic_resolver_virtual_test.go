package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
)

// TestAgentNameForProfile_RejectsVirtualFamily pins the W-045 inlet guard: a
// route decision pointing at a virtual-family profile must fail closed during
// resolution instead of flowing downstream as agent name "dynamic", which a
// direct launch would turn into an empty agent command.
func TestAgentNameForProfile_RejectsVirtualFamily(t *testing.T) {
	ctx := context.Background()
	profiles := &dynamicResolverTestProfiles{}
	resolver := NewProfileExecutionResolver(profiles, dynamic.NewEngine(), true)

	virtual := &agentsettingsmodels.AgentProfile{
		ID: "candidate-virtual", AgentID: agents.DynamicAgentID, Enabled: true,
	}
	_, err := resolver.agentNameForProfile(ctx, virtual)
	if !errors.Is(err, ErrVirtualProfile) {
		t.Fatalf("agentNameForProfile(virtual) error = %v, want ErrVirtualProfile", err)
	}
	if strings.Contains(err.Error(), "cannot be empty") {
		t.Fatalf("agentNameForProfile(virtual) error = %v, must not leak the empty-command symptom", err)
	}

	concrete := &agentsettingsmodels.AgentProfile{
		ID: "candidate-concrete", AgentID: "concrete-agent", Enabled: true,
	}
	name, err := resolver.agentNameForProfile(ctx, concrete)
	if err != nil {
		t.Fatalf("agentNameForProfile(concrete): %v", err)
	}
	if name != "concrete" {
		t.Fatalf("agentNameForProfile(concrete) = %q, want %q", name, "concrete")
	}
}
