package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
)

func TestResolveExistingAccountBlockedProfileRequiresRelease(t *testing.T) {
	for _, logicalID := range []string{"dynamic-profile", "office-profile"} {
		t.Run(logicalID, func(t *testing.T) {
			ctx := context.Background()
			profiles := &dynamicResolverTestProfiles{
				logical: &agentsettingsmodels.AgentProfile{ID: "dynamic-profile", AgentID: agents.DynamicAgentID, Enabled: true},
				office: &agentsettingsmodels.AgentProfile{
					ID: "office-profile", AgentID: agents.DynamicAgentID, Enabled: true,
					WorkspaceID: "workspace-1", ExecutionAgentProfileID: "dynamic-profile",
				},
				concrete: &agentsettingsmodels.AgentProfile{ID: "concrete-profile", AgentID: "concrete", Enabled: true},
			}
			resolver := NewProfileExecutionResolver(profiles, dynamic.NewEngine(), true)
			resolve := func() (ProfileExecution, error) {
				return resolver.ResolveExisting(ctx, "retained-session", logicalID, "concrete-profile", 7, 3, "durable_route_state")
			}
			if _, err := resolve(); err != nil {
				t.Fatalf("unheld existing route: %v", err)
			}
			if err := resolver.HoldAccountBlock("concrete-profile", routingerr.CodeAccountBlocked); err != nil {
				t.Fatalf("hold: %v", err)
			}
			if resolved, err := resolve(); !errors.Is(err, dynamic.ErrExecutionProfileHeld) || resolved.ExecutionProfileID != "" {
				t.Fatalf("held existing route = %+v, %v; want refusal", resolved, err)
			}
			if released, err := resolver.ReleaseAccountBlock(ctx, "concrete-profile"); err != nil || !released {
				t.Fatalf("release = %v, %v", released, err)
			}
			resolved, err := resolve()
			if err != nil || resolved.ExecutionProfileID != "concrete-profile" || resolved.Generation != 7 || resolved.ProfileVersion != 3 {
				t.Fatalf("released existing route = %+v, %v; want original attribution", resolved, err)
			}
		})
	}
}
