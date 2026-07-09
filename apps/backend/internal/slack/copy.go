package slack

import (
	"context"
	"errors"
	"fmt"
	"time"
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
	// Mark the target unhealthy before the write so the UI doesn't briefly flash
	// a stale "connected" state (old team/user id) from the target's previous
	// connection while the async probe validates the copied credentials. The
	// probe repopulates team/user id from the new credentials.
	if err := s.store.UpdateAuthHealthForWorkspace(ctx, targetWorkspaceID, false, "", "", "", time.Now().UTC()); err != nil {
		return nil, fmt.Errorf("reset target slack health: %w", err)
	}
	cfgOut, err := s.SetConfigForWorkspace(ctx, targetWorkspaceID, req)
	if err != nil {
		// The write failed, so the target's config is unchanged but we already
		// flipped its health row. Re-probe asynchronously (best effort) so the row
		// reflects the target's real state again rather than a spurious
		// "unhealthy", without delaying the error response by the probe timeout.
		// Detach from the request context, which may already be cancelled.
		go s.RecordAuthHealthForWorkspace(context.Background(), targetWorkspaceID)
		return nil, err
	}
	return cfgOut, nil
}
