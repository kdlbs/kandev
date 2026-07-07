package slack

import (
	"context"
	"errors"
	"fmt"
)

// ErrSameWorkspace is returned when a copy targets the same workspace it reads
// from — a no-op the caller should surface as a client error rather than
// silently succeeding.
var ErrSameWorkspace = errors.New("slack: source and target workspaces are the same")

// ErrNothingToCopy is returned when the source workspace has no Slack config to
// copy.
var ErrNothingToCopy = errors.New("slack: source workspace has no configuration to copy")

// CopyConfigToWorkspace copies the Slack provider config and credentials
// (token + cookie) from sourceWorkspaceID to targetWorkspaceID. Watchers and
// automations are intentionally out of scope — only the connection settings and
// secrets are duplicated. The target's health probe re-runs so runtime fields
// (team/user id) repopulate from the copied credentials.
func (s *Service) CopyConfigToWorkspace(ctx context.Context, sourceWorkspaceID, targetWorkspaceID string) (*SlackConfig, error) {
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
		return nil, fmt.Errorf("read source slack config: %w", err)
	}
	if cfg == nil {
		return nil, ErrNothingToCopy
	}
	token, cookie, err := s.revealSecrets(ctx, sourceWorkspaceID)
	if err != nil {
		return nil, err
	}
	// Empty Token/Cookie mean "preserve existing" in SetConfigForWorkspace, so a
	// source without stored secrets would silently leave the target on its old
	// credentials. Treat a missing source secret as nothing to copy instead.
	if token == "" || cookie == "" {
		return nil, ErrNothingToCopy
	}
	req := &SetConfigRequest{
		AuthMethod:          cfg.AuthMethod,
		CommandPrefix:       cfg.CommandPrefix,
		UtilityAgentID:      cfg.UtilityAgentID,
		PollIntervalSeconds: cfg.PollIntervalSeconds,
		Token:               token,
		Cookie:              cookie,
	}
	return s.SetConfigForWorkspace(ctx, targetWorkspaceID, req)
}
