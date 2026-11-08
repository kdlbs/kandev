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
	if got := executor.managedRuntimeOwnerUserID(synthetic, "workspace-1"); got != "synthetic" {
		t.Fatalf("synthetic owner = %q, want the single-user identity", got)
	}
	empty := &Executor{}
	if got := empty.managedRuntimeOwnerUserID(context.Background(), ""); got != "" {
		t.Fatalf("missing owner = %q, want empty", got)
	}
}
