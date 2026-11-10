package hostutility

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/auth/authn"
)

func TestManagedRuntimeProbeOwnerScopeSeparatesActors(t *testing.T) {
	manager := &Manager{}
	actorA := authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-a"})
	actorB := authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-b"})
	scopeA := manager.managedRuntimeProbeOwnerScope(actorA, "")
	scopeB := manager.managedRuntimeProbeOwnerScope(actorB, "")
	if scopeA == scopeB {
		t.Fatalf("distinct actors shared repair scope %q", scopeA)
	}
	if scopeA != managedruntime.RepairOwnerScopeAgentWide("user-a") {
		t.Fatalf("actor scope = %q", scopeA)
	}
	profile := manager.managedRuntimeProbeOwnerScope(actorA, "user-a:profile-1")
	if profile != managedruntime.RepairOwnerScopeProfile("user-a:profile-1") {
		t.Fatalf("profile scope = %q, want the authorized profile scope", profile)
	}
	internal := manager.managedRuntimeProbeOwnerScope(context.Background(), "")
	if internal != managedruntime.RepairOwnerScopeAgentWide(managedRuntimeSystemOwnerUserID) {
		t.Fatalf("internal scope = %q, want system", internal)
	}
}

// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.18
func TestManagedRuntimeProbeOwnerScopeModeParity(t *testing.T) {
	manager := &Manager{}
	manager.SetManagedRuntimeSingleUserOwner(func() (string, bool) { return managedRuntimeSystemOwnerUserID, true })
	startup := manager.managedRuntimeProbeOwnerScope(context.Background(), "")
	interactive := manager.managedRuntimeProbeOwnerScope(
		authn.WithIdentity(context.Background(), authn.Identity{UserID: managedRuntimeSystemOwnerUserID, Synthetic: true}), "",
	)
	if startup != interactive || startup != managedruntime.RepairOwnerScopeAgentWide(managedRuntimeSystemOwnerUserID) {
		t.Fatalf("disabled-mode startup/interactive scopes = (%q, %q), want the same single-user owner", startup, interactive)
	}

	manager.SetManagedRuntimeSingleUserOwner(func() (string, bool) { return managedRuntimeSystemOwnerUserID, false })
	enabledInternal := manager.managedRuntimeProbeOwnerScope(context.Background(), "")
	enabledUser := manager.managedRuntimeProbeOwnerScope(
		authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-a"}), "",
	)
	if enabledInternal != managedruntime.RepairOwnerScopeAgentWide(managedRuntimeSystemOwnerUserID) ||
		enabledUser != managedruntime.RepairOwnerScopeAgentWide("user-a") ||
		enabledInternal == enabledUser {
		t.Fatalf("enabled-mode scopes = (%q, %q), want system/user separation", enabledInternal, enabledUser)
	}
}
