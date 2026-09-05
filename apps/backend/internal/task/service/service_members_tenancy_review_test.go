package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/models"
)

type tenancyReviewDirectory struct {
	users map[string]string
}

func (d tenancyReviewDirectory) ListDirectory(context.Context, string) ([]DirectoryUser, error) {
	return nil, nil
}

func (d tenancyReviewDirectory) LookupStatus(_ context.Context, userID string) (string, string, bool, error) {
	status, ok := d.users[userID]
	return status, string(authz.OrgRoleMember), ok, nil
}

func TestWorkspaceMembershipRejectsUserFromAnotherOrganization(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "ws-org-a", Name: "Org A", OwnerID: "owner-a", OrgID: "org-a",
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	svc.SetUserDirectory(tenancyReviewDirectory{users: map[string]string{"user-b": "active"}})
	svc.SetUserOrgResolver(func(context.Context, string) (string, error) { return "org-b", nil })
	owner := authn.WithIdentity(ctx, authn.Identity{
		UserID: "owner-a", OrgID: "org-a", Role: authn.RoleMember,
	})

	if _, err := svc.UpsertWorkspaceMember(
		owner, "ws-org-a", "user-b", string(authz.WorkspaceRoleCollaborator),
	); !errors.Is(err, ErrMemberUserNotFound) {
		t.Fatalf("cross-organization member add = %v, want ErrMemberUserNotFound", err)
	}
	member, err := repo.GetWorkspaceMember(ctx, "ws-org-a", "user-b")
	if err != nil {
		t.Fatalf("get rejected member: %v", err)
	}
	if member != nil {
		t.Fatalf("cross-organization membership persisted: %+v", member)
	}

	if err := repo.UpsertWorkspaceMember(ctx, &models.WorkspaceMember{
		WorkspaceID: "ws-org-a", UserID: "user-b", Role: string(authz.WorkspaceRoleCollaborator),
	}); err != nil {
		t.Fatalf("seed legacy cross-organization member: %v", err)
	}
	if err := svc.TransferWorkspaceOwnership(owner, "ws-org-a", "user-b"); !errors.Is(err, ErrMemberUserNotFound) {
		t.Fatalf("cross-organization ownership transfer = %v, want ErrMemberUserNotFound", err)
	}
	workspace, err := repo.GetWorkspace(ctx, "ws-org-a")
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if workspace.OwnerID != "owner-a" {
		t.Fatalf("workspace owner = %q, want owner-a", workspace.OwnerID)
	}
}
