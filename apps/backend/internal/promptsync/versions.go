package promptsync

import (
	"context"

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
// the local version in one write. Choosing the repository version uses the
// repository version stored by the last sync; the next sync refreshes it. The
// save itself never runs a sync, so one Save that changes several prompts
// cannot move a prompt that a later save in the same Save is based on.
func (s *Service) SaveVersions(ctx context.Context, promptID string, save promptservice.VersionSave) (*promptmodels.Prompt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.lockedAuthorizedConfig(ctx); err != nil {
		return nil, err
	}
	return s.prompts.SavePromptVersions(ctx, promptID, save)
}
