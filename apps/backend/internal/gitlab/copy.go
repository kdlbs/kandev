package gitlab

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrSameWorkspace = errors.New("gitlab: source and target workspaces are the same")
var ErrNothingToCopy = errors.New("gitlab: source workspace has no configuration to copy")

// CopyConfigToWorkspace copies connection metadata and a stored PAT only.
func (s *Service) CopyConfigToWorkspace(ctx context.Context, sourceWorkspaceID, targetWorkspaceID string) (*GitLabConfig, error) {
	sourceWorkspaceID = strings.TrimSpace(sourceWorkspaceID)
	targetWorkspaceID = strings.TrimSpace(targetWorkspaceID)
	if sourceWorkspaceID == "" || targetWorkspaceID == "" {
		return nil, ErrWorkspaceRequired
	}
	if sourceWorkspaceID == targetWorkspaceID {
		return nil, ErrSameWorkspace
	}
	if err := s.copyConfigLocked(ctx, sourceWorkspaceID, targetWorkspaceID); err != nil {
		return nil, err
	}
	return s.GetConfigForWorkspace(ctx, targetWorkspaceID)
}

func (s *Service) copyConfigLocked(ctx context.Context, sourceWorkspaceID, targetWorkspaceID string) error {
	s.configMutationMu.Lock()
	defer s.configMutationMu.Unlock()

	s.mu.RLock()
	store := s.store
	secrets := s.workspaceSecrets
	s.mu.RUnlock()
	if store == nil {
		return errors.New("gitlab store not configured")
	}
	source, err := snapshotWorkspaceConnection(ctx, store, secrets, sourceWorkspaceID)
	if err != nil {
		return err
	}
	if source.config == nil {
		return ErrNothingToCopy
	}
	token := ""
	if source.config.AuthMethod == AuthMethodPAT {
		if !source.secret.exists || strings.TrimSpace(source.secret.value) == "" {
			return ErrNothingToCopy
		}
		token = source.secret.value
	}
	copyConfig := &GitLabConfig{Host: source.config.Host, AuthMethod: source.config.AuthMethod, Username: source.config.Username}
	if err := s.persistWorkspaceConfigLocked(ctx, store, secrets, targetWorkspaceID, copyConfig, token); err != nil {
		return fmt.Errorf("copy GitLab config: %w", err)
	}
	return nil
}
