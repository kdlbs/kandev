package executor

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
)

type repairOwnerRepository struct {
	*mockRepository
	ownerID string
}

func (r repairOwnerRepository) GetWorkspace(context.Context, string) (*models.Workspace, error) {
	return &models.Workspace{OwnerID: r.ownerID}, nil
}

func TestManagedRuntimeOwnerUserIDBindsActorAndWorkspaceOwner(t *testing.T) {
	executor := &Executor{repo: repairOwnerRepository{mockRepository: &mockRepository{}, ownerID: "workspace-owner"}}
	interactive := authn.WithIdentity(context.Background(), authn.Identity{UserID: "actor-a", Role: authn.RoleMember})
	if got := executor.managedRuntimeOwnerUserID(interactive, "workspace-1"); got != "actor-a" {
		t.Fatalf("interactive owner = %q, want the authenticated actor", got)
	}
	if got := executor.managedRuntimeOwnerUserID(context.Background(), "workspace-1"); got != "workspace-owner" {
		t.Fatalf("background owner = %q, want the workspace owner", got)
	}
	synthetic := authn.WithIdentity(context.Background(), authn.Identity{UserID: "synthetic", Synthetic: true})
	if got := executor.managedRuntimeOwnerUserID(synthetic, "workspace-1"); got != "workspace-owner" {
		t.Fatalf("synthetic owned-workspace owner = %q, want the durable workspace owner", got)
	}
	unowned := &Executor{}
	unowned.SetManagedRuntimeSingleUserOwner(func() (string, bool) { return "system", true })
	if got := unowned.managedRuntimeOwnerUserID(synthetic, ""); got != "system" {
		t.Fatalf("synthetic unowned-workspace owner = %q, want the disabled fallback", got)
	}
	empty := &Executor{}
	if got := empty.managedRuntimeOwnerUserID(context.Background(), ""); got != "" {
		t.Fatalf("missing owner = %q, want empty", got)
	}
	empty.SetManagedRuntimeSingleUserOwner(func() (string, bool) { return "system", true })
	if got := empty.managedRuntimeOwnerUserID(context.Background(), ""); got != "system" {
		t.Fatalf("disabled-mode background owner = %q, want system", got)
	}
	empty.SetManagedRuntimeSingleUserOwner(func() (string, bool) { return "system", false })
	if got := empty.managedRuntimeOwnerUserID(context.Background(), ""); got != "" {
		t.Fatalf("enabled-mode background owner = %q, want empty", got)
	}
}
