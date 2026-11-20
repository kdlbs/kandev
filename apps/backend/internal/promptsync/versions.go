package promptsync

import (
	"context"

	"go.uber.org/zap"

	promptmodels "github.com/kandev/kandev/internal/prompts/models"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
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

// SaveVersions applies the operator's Local | Repository choice and any edit to
// the local version in one write. Choosing the repository version runs a sync
// so the latest repository version is in use; a failed sync leaves the
// prompt synced and is recorded on the configuration like any other run.
func (s *Service) SaveVersions(ctx context.Context, promptID string, save promptservice.VersionSave) (*promptmodels.Prompt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg, err := s.lockedAuthorizedConfig(ctx)
	if err != nil {
		return nil, err
	}
	prompt, err := s.prompts.SavePromptVersions(ctx, promptID, save)
	if err != nil || save.UseLocal {
		return prompt, err
	}
	if _, err := s.syncLocked(ctx, syncManual, cfg, nil); err != nil {
		s.logger.Warn("sync after choosing a prompt's repository version failed", zap.String("error", safeErrorMessage(err)))
	}
	return s.prompts.GetPromptByID(ctx, promptID)
}
