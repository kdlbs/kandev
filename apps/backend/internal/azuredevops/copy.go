package azuredevops

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrSameWorkspace = errors.New("azure devops: source and target workspaces are the same")
	ErrNothingToCopy = errors.New("azure devops: source workspace has no configuration to copy")
)

// CopyConfigToWorkspace copies connection settings and the PAT, but not health
// state, from one workspace to another.
func (s *Service) CopyConfigToWorkspace(
	ctx context.Context,
	sourceWorkspaceID string,
	targetWorkspaceID string,
) (*Config, error) {
	if err := validateWorkspaceID(sourceWorkspaceID); err != nil {
		return nil, err
	}
	if err := validateWorkspaceID(targetWorkspaceID); err != nil {
		return nil, err
	}
	if sourceWorkspaceID == targetWorkspaceID {
		return nil, ErrSameWorkspace
	}
	source, err := s.store.GetConfig(ctx, sourceWorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("read source azure devops config: %w", err)
	}
	if source == nil {
		return nil, ErrNothingToCopy
	}
	pat, err := s.revealPAT(ctx, sourceWorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("read source azure devops credential: %w", err)
	}
	_, err = s.SetConfigForWorkspace(ctx, targetWorkspaceID, &SetConfigRequest{
		OrganizationURL:    source.OrganizationURL,
		DefaultProjectID:   source.DefaultProjectID,
		DefaultProjectName: source.DefaultProjectName,
		AuthMethod:         source.AuthMethod,
		PAT:                pat,
	})
	if err != nil {
		return nil, err
	}
	if err := s.store.ResetAuthHealth(ctx, targetWorkspaceID); err != nil {
		return nil, fmt.Errorf("reset target azure devops health: %w", err)
	}
	return s.GetConfigForWorkspace(ctx, targetWorkspaceID)
}
