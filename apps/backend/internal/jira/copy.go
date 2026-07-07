package jira

import (
	"context"
	"errors"
	"fmt"
)

// ErrSameWorkspace is returned when a copy targets the same workspace it reads
// from.
var ErrSameWorkspace = errors.New("jira: source and target workspaces are the same")

// ErrNothingToCopy is returned when the source workspace has no Jira config.
var ErrNothingToCopy = errors.New("jira: source workspace has no configuration to copy")

// CopyConfigToWorkspace copies the Jira provider config and credential (token)
// from sourceWorkspaceID to targetWorkspaceID. Watchers are intentionally out
// of scope — only the connection settings and secret are duplicated.
func (s *Service) CopyConfigToWorkspace(ctx context.Context, sourceWorkspaceID, targetWorkspaceID string) (*JiraConfig, error) {
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
		return nil, fmt.Errorf("read source jira config: %w", err)
	}
	if cfg == nil {
		return nil, ErrNothingToCopy
	}
	secret, err := s.revealSecret(ctx, sourceWorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("read source jira secret: %w", err)
	}
	req := &SetConfigRequest{
		SiteURL:           cfg.SiteURL,
		Email:             cfg.Email,
		AuthMethod:        cfg.AuthMethod,
		InstanceType:      cfg.InstanceType,
		DefaultProjectKey: cfg.DefaultProjectKey,
		Secret:            secret,
	}
	return s.SetConfigForWorkspace(ctx, targetWorkspaceID, req)
}
