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
// with the same identity. It starts from the kept local version when there is
// one, otherwise from the synced content. Sync then leaves it alone.
func (s *Service) OverrideSyncedPrompt(ctx context.Context, id string) (*models.Prompt, error) {
	return s.transition(ctx, id, s.repo.OverrideSyncedPrompt, ErrPromptNotSynced)
}

// RevertOverride returns an override to sync ownership, keeping its content
// as the local version; the next sync writes the repository version.
func (s *Service) RevertOverride(ctx context.Context, id string) (*models.Prompt, error) {
	return s.transition(ctx, id, s.repo.RevertOverride, ErrPromptNotOverride)
}

func (s *Service) transition(
	ctx context.Context, id string, write func(context.Context, string) (bool, error), wrongSource error,
) (*models.Prompt, error) {
	changed, err := write(ctx, id)
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
