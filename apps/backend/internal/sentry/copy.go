package sentry

import (
	"context"
	"errors"
	"fmt"
)

// ErrSameWorkspace is returned when a copy targets the same workspace it reads
// from.
var ErrSameWorkspace = errors.New("sentry: source and target workspaces are the same")

// ErrNothingToCopy is returned when the source workspace has no Sentry config.
var ErrNothingToCopy = errors.New("sentry: source workspace has no configuration to copy")

// CopyConfigToWorkspace copies the Sentry provider config and credential (auth
// token) from sourceWorkspaceID to targetWorkspaceID. Watchers are
// intentionally out of scope — only the connection settings and secret are
// duplicated.
func (s *Service) CopyConfigToWorkspace(ctx context.Context, sourceWorkspaceID, targetWorkspaceID string) (*SentryConfig, error) {
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
		return nil, fmt.Errorf("read source sentry config: %w", err)
	}
	if cfg == nil {
		return nil, ErrNothingToCopy
	}
	secret, err := s.revealSecret(ctx, sourceWorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("read source sentry secret: %w", err)
	}
	req := &SetConfigRequest{
		AuthMethod: cfg.AuthMethod,
		URL:        cfg.URL,
		Secret:     secret,
	}
	return s.SetConfigForWorkspace(ctx, targetWorkspaceID, req)
}
