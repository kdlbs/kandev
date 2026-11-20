package service

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/prompts/models"
)

var (
	// ErrPromptNotSynced rejects overriding a prompt that sync does not own.
	ErrPromptNotSynced = errors.New("only a synced prompt can be overridden")
	// ErrPromptNotOverride rejects reverting a prompt that is not an override.
	ErrPromptNotOverride = errors.New("only an overridden prompt can be reverted to the synced version")
)

// OverrideSyncedPrompt turns a synced prompt into a local, editable override
// with the same identity and content. Sync then leaves it alone.
func (s *Service) OverrideSyncedPrompt(ctx context.Context, id string) (*models.Prompt, error) {
	return s.moveSource(ctx, id, models.PromptSourceSync, models.PromptSourceOverride, ErrPromptNotSynced)
}

// RevertOverride returns an override to sync ownership; the next sync
// replaces its content with the repository version.
func (s *Service) RevertOverride(ctx context.Context, id string) (*models.Prompt, error) {
	return s.moveSource(ctx, id, models.PromptSourceOverride, models.PromptSourceSync, ErrPromptNotOverride)
}

func (s *Service) moveSource(ctx context.Context, id, from, to string, wrongSource error) (*models.Prompt, error) {
	changed, err := s.repo.SetPromptSource(ctx, id, from, to)
	if err != nil {
		return nil, err
	}
	if !changed {
		if _, err := s.GetPromptByID(ctx, id); err != nil {
			return nil, err
		}
		return nil, wrongSource
	}
	s.publishChanged(ctx)
	return s.GetPromptByID(ctx, id)
}
