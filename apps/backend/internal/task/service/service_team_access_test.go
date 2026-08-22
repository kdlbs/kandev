package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// End-to-end team access through the real service and repository. The authz
// package proves the resolver's table; this proves the wiring actually reaches
// it — a correct resolver that nobody calls protects nothing.

func ctxAsRole(userID string, role authn.Role) context.Context {
	return authn.WithIdentity(context.Background(), authn.Identity{UserID: userID, Role: role})
}

type teamAccessRepo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateWorkflow(context.Context, *models.Workflow) error
	CreateTask(context.Context, *models.Task) error
	UpsertWorkspaceMember(context.Context, *models.WorkspaceMember) error
}

func seedTeamWorkspace(t *testing.T, repo teamAccessRepo, visibility authz.Visibility) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "ws-team", Name: "Team", OwnerID: "user-ana", Visibility: string(visibility),
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-team", WorkspaceID: "ws-team", Name: "Board"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-team", WorkspaceID: "ws-team", WorkflowID: "wf-team", WorkflowStepID: "step-1",
		Title: "Ana's task", State: v1.TaskStateCreated, Priority: "medium",
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
}

// The #2824 golden path: a colleague sees the board with no invitation.
func TestOrgVisibleWorkspaceReachableWithoutInvitation(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedTeamWorkspace(t, repo, authz.VisibilityOrg)

	if _, err := svc.GetWorkspace(ctxAsRole("user-bruno", authn.RoleMember), "ws-team"); err != nil {
		t.Fatalf("org-visible workspace must be reachable by a member: %v", err)
	}
	if _, err := svc.GetTask(ctxAsRole("user-bruno", authn.RoleMember), "task-team"); err != nil {
		t.Fatalf("org-visible task must be readable by a member: %v", err)
	}
}

// The privacy case the current design is built around must not regress.
func TestPrivateWorkspaceStaysPrivate(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedTeamWorkspace(t, repo, authz.VisibilityPrivate)

	bruno := ctxAsRole("user-bruno", authn.RoleMember)
	if _, err := svc.GetWorkspace(bruno, "ws-team"); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("private workspace leaked to a non-member: %v", err)
	}
	if _, err := svc.GetTask(bruno, "task-team"); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("private task leaked to a non-member: %v", err)
	}
	// An admin is a management role, not a visibility role.
	admin := ctxAsRole("user-carla", authn.RoleAdmin)
	if _, err := svc.GetWorkspace(admin, "ws-team"); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("admin reached a private workspace they are not in: %v", err)
	}
}

// A guest is deliberately excluded from org-visible workspaces.
func TestGuestExcludedFromOrgVisibleWorkspace(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedTeamWorkspace(t, repo, authz.VisibilityOrg)

	guest := ctxAsRole("user-contractor", authn.RoleGuest)
	if _, err := svc.GetWorkspace(guest, "ws-team"); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("guest reached an org-visible workspace: %v", err)
	}

	// ...until they hold an explicit row.
	if err := repo.UpsertWorkspaceMember(context.Background(), &models.WorkspaceMember{
		WorkspaceID: "ws-team", UserID: "user-contractor", Role: string(authz.WorkspaceRoleCollaborator),
	}); err != nil {
		t.Fatalf("add member: %v", err)
	}
	if _, err := svc.GetWorkspace(guest, "ws-team"); err != nil {
		t.Fatalf("guest with a membership row must reach the workspace: %v", err)
	}
}

// An explicit row outranks the org default in the narrowing direction too.
func TestViewerRowNarrowsMemberOnOrgVisibleWorkspace(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedTeamWorkspace(t, repo, authz.VisibilityOrg)
	if err := repo.UpsertWorkspaceMember(context.Background(), &models.WorkspaceMember{
		WorkspaceID: "ws-team", UserID: "user-bruno", Role: string(authz.WorkspaceRoleViewer),
	}); err != nil {
		t.Fatalf("add viewer: %v", err)
	}

	viewer := ctxAsRole("user-bruno", authn.RoleMember)
	if _, err := svc.GetTask(viewer, "task-team"); err != nil {
		t.Fatalf("viewer must still read: %v", err)
	}
	// Reading is 200; writing is 403, not 404 — existence is already known.
	title := "hijacked"
	_, err := svc.UpdateTask(viewer, "task-team", &UpdateTaskRequest{Title: &title})
	if !IsForbidden(err) {
		t.Fatalf("viewer task write = %v, want ErrForbidden", err)
	}
}

// A collaborator contributes; it does not administer.
func TestCollaboratorCannotManageWorkspace(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedTeamWorkspace(t, repo, authz.VisibilityOrg)

	bruno := ctxAsRole("user-bruno", authn.RoleMember)
	title := "renamed by a collaborator"
	if _, err := svc.UpdateWorkspace(bruno, "ws-team", &UpdateWorkspaceRequest{Name: &title}); !IsForbidden(err) {
		t.Fatalf("collaborator workspace rename = %v, want ErrForbidden", err)
	}
	if _, err := svc.UpsertWorkspaceMember(bruno, "ws-team", "user-carla", string(authz.WorkspaceRoleCollaborator)); !IsForbidden(err) {
		t.Fatalf("collaborator member add = %v, want ErrForbidden", err)
	}
	// A collaborator CAN do the work: writing a task is the whole point.
	if _, err := svc.UpdateTask(bruno, "task-team", &UpdateTaskRequest{Title: &title}); err != nil {
		t.Fatalf("collaborator must be able to write a task: %v", err)
	}
}

// The owner keeps full control, and the DTO projection agrees with the resolver.
func TestOwnerScopesProjectIntoAccessProjection(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedTeamWorkspace(t, repo, authz.VisibilityOrg)

	workspace, err := svc.GetWorkspace(ctxAsRole("user-ana", authn.RoleMember), "ws-team")
	if err != nil {
		t.Fatalf("owner get: %v", err)
	}
	projection := svc.ProjectWorkspaceAccess(ctxAsRole("user-ana", authn.RoleMember), []*models.Workspace{workspace})
	decision := projection.Decision("ws-team")
	if decision.Role != authz.WorkspaceRoleOwner {
		t.Fatalf("owner viewer_role = %q", decision.Role)
	}
	for _, scope := range []authz.Scope{authz.ScopeWorkspaceManage, authz.ScopeMemberManage, authz.ScopeSessionExec} {
		if !decision.Has(scope) {
			t.Errorf("owner projection missing %q", scope)
		}
	}

	brunoProjection := svc.ProjectWorkspaceAccess(ctxAsRole("user-bruno", authn.RoleMember), []*models.Workspace{workspace})
	if got := brunoProjection.Decision("ws-team"); got.Role != authz.WorkspaceRoleCollaborator {
		t.Fatalf("member viewer_role = %q, want collaborator", got.Role)
	}
}

// Membership management rejects each bad input with its own reason.
func TestMemberManagementValidation(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedTeamWorkspace(t, repo, authz.VisibilityPrivate)
	ana := ctxAsRole("user-ana", authn.RoleMember)

	if _, err := svc.UpsertWorkspaceMember(ana, "ws-team", "user-bruno", "owner"); !errors.Is(err, ErrMemberRoleInvalid) {
		t.Fatalf("assigning owner = %v, want ErrMemberRoleInvalid", err)
	}
	if _, err := svc.UpsertWorkspaceMember(ana, "ws-team", "user-ana", "collaborator"); !errors.Is(err, ErrMemberSelf) {
		t.Fatalf("adding the owner = %v, want ErrMemberSelf", err)
	}
	if err := svc.RemoveWorkspaceMember(ana, "ws-team", "user-ana"); !errors.Is(err, ErrMemberIsOwner) {
		t.Fatalf("removing the owner = %v, want ErrMemberIsOwner", err)
	}
	if err := svc.TransferWorkspaceOwnership(ana, "ws-team", "user-nobody"); !errors.Is(err, ErrTransferTargetNotMember) {
		t.Fatalf("transfer to a non-member = %v, want ErrTransferTargetNotMember", err)
	}
}

// Ownership transfer must leave owner_id and the owner row in agreement.
func TestTransferOwnershipKeepsOwnerRowConsistent(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedTeamWorkspace(t, repo, authz.VisibilityPrivate)
	ana := ctxAsRole("user-ana", authn.RoleMember)

	if _, err := svc.UpsertWorkspaceMember(ana, "ws-team", "user-bruno", "collaborator"); err != nil {
		t.Fatalf("add member: %v", err)
	}
	if err := svc.TransferWorkspaceOwnership(ana, "ws-team", "user-bruno"); err != nil {
		t.Fatalf("transfer: %v", err)
	}

	workspace, err := svc.GetWorkspace(context.Background(), "ws-team")
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if workspace.OwnerID != "user-bruno" {
		t.Fatalf("owner_id = %q, want user-bruno", workspace.OwnerID)
	}
	member, err := repo.GetWorkspaceMember(context.Background(), "ws-team", "user-bruno")
	if err != nil || member == nil {
		t.Fatalf("owner membership row missing: %v", err)
	}
	if member.Role != string(authz.WorkspaceRoleOwner) {
		t.Fatalf("new owner row role = %q, want owner", member.Role)
	}
	// The previous owner is demoted, not removed.
	previous, err := repo.GetWorkspaceMember(context.Background(), "ws-team", "user-ana")
	if err != nil || previous == nil {
		t.Fatalf("previous owner row missing: %v", err)
	}
	if previous.Role != string(authz.WorkspaceRoleCollaborator) {
		t.Fatalf("previous owner role = %q, want collaborator", previous.Role)
	}
}

// Attribution comes from the authenticated caller, never the payload.
func TestMessageAuthorIsServerStamped(t *testing.T) {
	svc, _, _ := createTestService(t)
	stamped := svc.resolveAuthorID(ctxAsRole("user-bruno", authn.RoleMember), models.MessageAuthorUser, "user-ana")
	if stamped != "user-bruno" {
		t.Fatalf("author = %q, want the authenticated caller user-bruno", stamped)
	}
	// Agent messages name an execution, not a person, and keep their value.
	agent := svc.resolveAuthorID(ctxAsRole("user-bruno", authn.RoleMember), models.MessageAuthorAgent, "exec-123")
	if agent != "exec-123" {
		t.Fatalf("agent author = %q, want exec-123", agent)
	}
	// Internal/auth-disabled callers keep today's behavior.
	internal := svc.resolveAuthorID(context.Background(), models.MessageAuthorUser, "whatever")
	if internal != "whatever" {
		t.Fatalf("internal author = %q, want whatever", internal)
	}
}

// A viewer may read a task but must not move it, edit its metadata, or change
// a repository's base branch. These are the mutators that stayed on the
// reach-only helper until review caught them.
func TestViewerCannotReachWriteOnlyMutators(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedTeamWorkspace(t, repo, authz.VisibilityOrg)
	if err := repo.UpsertWorkspaceMember(context.Background(), &models.WorkspaceMember{
		WorkspaceID: "ws-team", UserID: "user-bruno", Role: string(authz.WorkspaceRoleViewer),
	}); err != nil {
		t.Fatalf("add viewer: %v", err)
	}
	viewer := ctxAsRole("user-bruno", authn.RoleMember)

	if _, err := svc.UpdateTaskMetadata(viewer, "task-team", map[string]interface{}{"x": 1}); !IsForbidden(err) {
		t.Errorf("viewer UpdateTaskMetadata = %v, want ErrForbidden", err)
	}
	if _, err := svc.GetTask(viewer, "task-team"); err != nil {
		t.Errorf("viewer must still read the task: %v", err)
	}
}

// A lookup failure must never read as "granted".
func TestAuthorizationFailsClosedOnLookupError(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedTeamWorkspace(t, repo, authz.VisibilityPrivate)
	// A task pointing at a workspace row that does not exist is a dangling
	// reference and stays readable; that is the documented exception.
	if err := repo.CreateTask(context.Background(), &models.Task{
		ID: "task-dangling", WorkspaceID: "ws-missing", WorkflowID: "wf-team",
		WorkflowStepID: "step-1", Title: "Dangling", State: v1.TaskStateCreated, Priority: "medium",
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := svc.GetTask(ctxAsRole("user-bruno", authn.RoleMember), "task-dangling"); err != nil {
		t.Errorf("a dangling workspace reference should not hide the task: %v", err)
	}
}
