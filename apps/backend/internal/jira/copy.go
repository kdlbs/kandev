package jira

import (
	"context"
	"errors"
	"fmt"
	"time"
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
	// An empty Secret means "keep existing" in SetConfigForWorkspace, so copying
	// from a workspace without a stored token would leave the target on its old
	// credential paired with the copied site/email. Treat that as nothing to copy.
	if secret == "" {
		return nil, ErrNothingToCopy
	}
	req := &SetConfigRequest{
		SiteURL:           cfg.SiteURL,
		Email:             cfg.Email,
		AuthMethod:        cfg.AuthMethod,
		InstanceType:      cfg.InstanceType,
		DefaultProjectKey: cfg.DefaultProjectKey,
		Secret:            secret,
	}
	// Mark the target unhealthy before the write so the UI doesn't briefly flash
	// a stale "connected" state from the target's previous credential while the
	// async probe kicked off by SetConfigForWorkspace validates the copied one.
	if err := s.store.UpdateAuthHealthForWorkspace(ctx, targetWorkspaceID, false, "", time.Now().UTC()); err != nil {
		return nil, fmt.Errorf("reset target jira health: %w", err)
	}
	return s.SetConfigForWorkspace(ctx, targetWorkspaceID, req)
}
