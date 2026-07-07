package linear

import (
	"context"
	"errors"
	"fmt"
)

// ErrSameWorkspace is returned when a copy targets the same workspace it reads
// from.
var ErrSameWorkspace = errors.New("linear: source and target workspaces are the same")

// ErrNothingToCopy is returned when the source workspace has no Linear config.
var ErrNothingToCopy = errors.New("linear: source workspace has no configuration to copy")

// CopyConfigToWorkspace copies the Linear provider config and credential (API
// key) from sourceWorkspaceID to targetWorkspaceID. Watchers are intentionally
// out of scope — only the connection settings and secret are duplicated.
func (s *Service) CopyConfigToWorkspace(ctx context.Context, sourceWorkspaceID, targetWorkspaceID string) (*LinearConfig, error) {
	sourceWorkspaceID, err := s.normalizeWorkspaceID(sourceWorkspaceID)
	if err != nil {
		return nil, err
	}
	targetWorkspaceID, err = s.normalizeWorkspaceID(targetWorkspaceID)
	if err != nil {
		return nil, err
	}
	if sourceWorkspaceID == targetWorkspaceID {
		return nil, ErrSameWorkspace
	}
	cfg, err := s.store.GetConfigForWorkspace(ctx, sourceWorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("read source linear config: %w", err)
	}
	if cfg == nil {
		return nil, ErrNothingToCopy
	}
	secret, err := s.revealSecret(ctx, sourceWorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("read source linear secret: %w", err)
	}
	// An empty Secret means "keep existing" in SetConfigForWorkspace, so copying
	// from a workspace without a stored API key would leave the target on its old
	// credential paired with the copied settings. Treat that as nothing to copy.
	if secret == "" {
		return nil, ErrNothingToCopy
	}
	req := &SetConfigRequest{
		AuthMethod:     cfg.AuthMethod,
		DefaultTeamKey: cfg.DefaultTeamKey,
		Secret:         secret,
	}
	return s.SetConfigForWorkspace(ctx, targetWorkspaceID, req)
}
