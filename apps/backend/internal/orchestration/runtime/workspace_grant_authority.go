package runtime

import (
	"context"
	"fmt"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestration/models"
	"slices"
)

func (s *Service) workspaceGrantReceiver(ctx context.Context, b *models.AssistantBinding) (*models.WorkspaceGrantReceiver, error) {
	a, err := s.Personas.GetAgentInstance(ctx, b.OrchestratorID)
	if err != nil {
		return nil, err
	}
	profile, executor, err := s.executionSelection(ctx, a)
	if err != nil {
		return nil, err
	}
	revision, err := s.contextProfileRevision(ctx, b.WorkspaceID, profile)
	if err != nil {
		return nil, err
	}
	p, err := s.Personas.Profiles.GetAgentProfile(ctx, profile)
	if err != nil {
		return nil, err
	}
	if s.Authority == nil {
		return nil, fmt.Errorf("assistant authority unavailable")
	}
	ctx = authn.WithIdentity(ctx, authn.Identity{UserID: b.OwnerUserID, Role: authn.RoleMember})
	authority, err := s.Authority.ResolveAssistantAuthority(ctx, *b, profile, executor)
	if err != nil {
		return nil, err
	}
	return &models.WorkspaceGrantReceiver{ProfileID: profile, ProfileName: p.Name, ProfileRevision: revision, AuthorityRevision: authority.Revision}, nil
}

func (s *Service) workspaceGrantReason(ctx context.Context, b *models.AssistantBinding, g *models.WorkspaceGrant) string {
	if g.RevokedAt != nil {
		return "grant_revoked"
	}
	if g.OwnerUserID != b.OwnerUserID || g.BindingID != b.ID || g.BindingVersion != b.Version {
		return "binding_changed"
	}
	receiver, err := s.workspaceGrantReceiver(ctx, b)
	if err != nil {
		return "receiving_profile_unavailable"
	}
	if receiver.ProfileID != g.ReceiverProfileID || receiver.ProfileRevision != g.ReceiverProfileRevision || receiver.AuthorityRevision != g.AuthorityRevision {
		return "receiving_profile_changed"
	}
	return ""
}

func (s *Service) currentWorkspaceGrant(ctx context.Context, b *models.AssistantBinding, workspace string, revision int64, operation, export string) (*models.WorkspaceGrant, error) {
	g, err := s.workspaceGrantSnapshot(ctx, b, workspace, revision)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(g.Scope.Operations, operation) || (export != "" && !slices.Contains(g.Scope.ContextExports, export)) {
		return nil, fmt.Errorf("workspace scope does not authorize this operation or context export")
	}
	reader, ok := s.Manager.(WorkspaceGrantReader)
	if !ok {
		return nil, fmt.Errorf("native workspace access unavailable")
	}
	ctx = authn.WithIdentity(ctx, authn.Identity{UserID: b.OwnerUserID, Role: authn.RoleMember})
	if _, err = reader.WorkspaceGrantAccess(ctx, workspace, operation == workspaceCoordinate); err != nil {
		return nil, err
	}
	return s.workspaceGrantSnapshot(ctx, b, workspace, revision)
}

func (s *Service) workspaceGrantSnapshot(ctx context.Context, b *models.AssistantBinding, workspace string, revision int64) (*models.WorkspaceGrant, error) {
	g, err := s.Repo.WorkspaceGrant(ctx, b.ID, workspace)
	if err != nil || g.Revision != revision || s.workspaceGrantReason(ctx, b, g) != "" {
		return nil, models.ErrConflict
	}
	current, err := s.Repo.AssistantBindingByID(ctx, b.ID)
	if err != nil || current.Version != b.Version || current.OwnerUserID != b.OwnerUserID {
		return nil, models.ErrConflict
	}
	return g, nil
}
