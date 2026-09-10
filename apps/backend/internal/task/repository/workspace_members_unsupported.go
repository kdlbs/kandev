package repository

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
)

// UnsupportedWorkspaceMembers supplies membership no-ops for fakes that only
// exercise other parts of WorkspaceRepository.
//
// Every method reports "no membership", which resolves to owner-and-visibility
// access — the narrower answer. A fake that embeds this can never accidentally
// widen access in a test and make a scoping regression look green.
type UnsupportedWorkspaceMembers struct{}

func (UnsupportedWorkspaceMembers) ListWorkspaceMembers(context.Context, string) ([]*models.WorkspaceMember, error) {
	return nil, nil
}

func (UnsupportedWorkspaceMembers) GetWorkspaceMember(context.Context, string, string) (*models.WorkspaceMember, error) {
	return nil, nil
}

func (UnsupportedWorkspaceMembers) ListWorkspaceIDsForMember(context.Context, string) (map[string]string, error) {
	return map[string]string{}, nil
}

func (UnsupportedWorkspaceMembers) UpsertWorkspaceMember(context.Context, *models.WorkspaceMember) error {
	return nil
}

func (UnsupportedWorkspaceMembers) DeleteWorkspaceMember(context.Context, string, string) error {
	return nil
}

func (UnsupportedWorkspaceMembers) DeleteWorkspaceMembersByWorkspace(context.Context, string) error {
	return nil
}

func (UnsupportedWorkspaceMembers) CountWorkspaceMembers(context.Context) (map[string]int, error) {
	return map[string]int{}, nil
}

func (UnsupportedWorkspaceMembers) TransferWorkspaceOwnership(context.Context, string, string, string) error {
	return nil
}
