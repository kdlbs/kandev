package promptsync

import (
	"context"

	"go.uber.org/zap"

	promptmodels "github.com/kandev/kandev/internal/prompts/models"
)

// lockedAuthorizedConfig loads the configuration under the held service lock
// and authorizes its credential workspace.
func (s *Service) lockedAuthorizedConfig(ctx context.Context) (*Config, error) {
	cfg, err := s.store.Get(ctx)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, ErrNotConfigured
	}
	if err := s.authorize(ctx, cfg.WorkspaceID); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Override turns a synced prompt into a local, editable override that sync
// leaves alone.
func (s *Service) Override(ctx context.Context, promptID string) (*promptmodels.Prompt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.lockedAuthorizedConfig(ctx); err != nil {
		return nil, err
	}
	return s.prompts.OverrideSyncedPrompt(ctx, promptID)
}

// Revert returns an override to sync ownership and runs a sync so the
// repository version replaces the local content. A failed sync leaves the
// prompt synced and is recorded on the configuration like any other run.
func (s *Service) Revert(ctx context.Context, promptID string) (*promptmodels.Prompt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg, err := s.lockedAuthorizedConfig(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.prompts.RevertOverride(ctx, promptID); err != nil {
		return nil, err
	}
	if _, err := s.syncLocked(ctx, syncManual, cfg, nil); err != nil {
		s.logger.Warn("sync after reverting a prompt override failed", zap.String("error", safeErrorMessage(err)))
	}
	return s.prompts.GetPromptByID(ctx, promptID)
}
