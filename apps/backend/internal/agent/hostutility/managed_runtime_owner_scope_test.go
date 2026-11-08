package hostutility

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/auth/authn"
)

func TestManagedRuntimeProbeOwnerScopeSeparatesActors(t *testing.T) {
	actorA := authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-a"})
	actorB := authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-b"})
	scopeA := managedRuntimeProbeOwnerScope(actorA, "")
	scopeB := managedRuntimeProbeOwnerScope(actorB, "")
	if scopeA == scopeB {
		t.Fatalf("agent-wide scopes = %q and %q, want actor-separated values", scopeA, scopeB)
	}
	if scopeA != managedruntime.RepairOwnerScopeAgentWide("user-a") {
		t.Fatalf("agent-wide scope = %q, want the actor-bound form", scopeA)
	}
	profile := managedRuntimeProbeOwnerScope(actorA, "user-a:profile-1")
	if profile != managedruntime.RepairOwnerScopeProfile("user-a:profile-1") {
		t.Fatalf("profile scope = %q, want the profile-owner form", profile)
	}
	internal := managedRuntimeProbeOwnerScope(context.Background(), "")
	if internal != managedruntime.RepairOwnerScopeAgentWide("system") {
		t.Fatalf("internal scope = %q, want the system owner", internal)
	}
}
